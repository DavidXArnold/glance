#!/usr/bin/env bash
# Simulate a large cluster with KWOK (https://kwok.sigs.k8s.io) in the current
# kube context, for testing glance's performance locally. KWOK fakes kubelets:
# nodes and pods are real API objects, but no containers run, so thousands of
# them fit on a laptop. metrics-server has nothing to scrape on fake nodes,
# so usage columns stay empty for them.
#
#   hack/scale-test.sh up [NODES] [PODS_PER_NODE] [NAMESPACES]   (default 1000 30 50)
#   hack/scale-test.sh down
#
# "up" installs the KWOK controller if missing. Everything it creates carries
# the label glance-scale-test=true, and "down" deletes exactly that.
set -euo pipefail

KWOK_VERSION="${KWOK_VERSION:-v0.8.0}"
LABEL="glance-scale-test=true"
cmd="${1:-}"

install_kwok() {
  if kubectl -n kube-system get deploy kwok-controller >/dev/null 2>&1; then
    return
  fi
  echo "Installing KWOK ${KWOK_VERSION}..."
  local base="https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}"
  # flowcontrol v1 needs Kubernetes 1.29+; v1beta2 works on older clusters (e.g. k3s 1.24).
  local minor
  minor=$(kubectl version -o json | sed -n 's/.*"minor": *"\([0-9]*\).*/\1/p' | tail -1)
  if [ "${minor:-99}" -lt 29 ]; then
    curl -sSLf "${base}/kwok.yaml" | sed 's#flowcontrol.apiserver.k8s.io/v1$#flowcontrol.apiserver.k8s.io/v1beta2#' |
      kubectl apply -f -
  else
    kubectl apply -f "${base}/kwok.yaml"
  fi
  kubectl apply -f "${base}/stage-fast.yaml"
  kubectl -n kube-system rollout status deploy kwok-controller --timeout=180s
}

node_yaml() {
  local i=$1 gpu=$2
  local name
  name=$(printf "kwok-node-%05d" "$i")
  cat <<EOF
---
apiVersion: v1
kind: Node
metadata:
  name: ${name}
  annotations:
    kwok.x-k8s.io/node: fake
  labels:
    glance-scale-test: "true"
    type: kwok
    kubernetes.io/hostname: ${name}
    node.kubernetes.io/instance-type: m5.8xlarge
    topology.kubernetes.io/zone: zone-$((i % 3))
EOF
  if [ "$gpu" = "true" ]; then
    cat <<EOF
    nvidia.com/gpu.product: NVIDIA-A100-SXM4-80GB
EOF
  fi
  cat <<EOF
spec:
  taints:
  - key: kwok.x-k8s.io/node
    value: fake
    effect: NoSchedule
status:
  allocatable: {cpu: "32", memory: 128Gi, pods: "110"$( [ "$gpu" = "true" ] && echo ', nvidia.com/gpu: "8"')}
  capacity: {cpu: "32", memory: 128Gi, pods: "110"$( [ "$gpu" = "true" ] && echo ', nvidia.com/gpu: "8"')}
EOF
}

deployment_yaml() {
  local ns=$1 d=$2 replicas=$3 gpu=$4
  cat <<EOF
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: app-${d}
  namespace: ${ns}
  labels: {glance-scale-test: "true"}
spec:
  replicas: ${replicas}
  selector: {matchLabels: {app: app-${d}}}
  template:
    metadata:
      labels: {app: app-${d}, glance-scale-test: "true"}
    spec:
      nodeSelector: {type: kwok}
      tolerations:
      - {key: kwok.x-k8s.io/node, operator: Exists, effect: NoSchedule}
      containers:
      - name: app
        image: registry.example/fake:latest
        env: [{name: MODE, value: scale-test}]
        resources:
          requests: {cpu: 250m, memory: 512Mi$( [ "$gpu" = "true" ] && echo ', nvidia.com/gpu: "1"')}
          limits: {cpu: "1", memory: 1Gi$( [ "$gpu" = "true" ] && echo ', nvidia.com/gpu: "1"')}
EOF
}

up() {
  local nodes=${1:-1000} per_node=${2:-30} namespaces=${3:-50}
  local pods=$((nodes * per_node))
  local gpu_nodes=$((nodes / 20)) # 5% GPU nodes
  # kubectl apply does not update node status, so re-running over existing
  # nodes would leave their capacity (e.g. GPUs) stale.
  if [ -n "$(kubectl get nodes -l "$LABEL" -o name 2>/dev/null | head -1)" ]; then
    echo "Scale-test nodes already exist; run '$0 down' first." >&2
    exit 1
  fi
  install_kwok

  echo "Creating ${nodes} nodes (${gpu_nodes} with 8 GPUs each)..."
  for ((i = 0; i < nodes; i++)); do
    node_yaml "$i" "$([ "$i" -lt "$gpu_nodes" ] && echo true || echo false)"
  done | kubectl apply -f - >/dev/null

  echo "Creating ${namespaces} namespaces..."
  for ((n = 0; n < namespaces; n++)); do
    printf -- "---\napiVersion: v1\nkind: Namespace\nmetadata:\n  name: scale-%03d\n  labels: {glance-scale-test: \"true\"}\n" "$n"
  done | kubectl apply -f - >/dev/null

  # 10 deployments per namespace share the pods; the first deployment in each
  # namespace requests one GPU per replica, filling about half the GPUs.
  local per_deploy=$((pods / (namespaces * 10)))
  local gpu_replicas=$(((gpu_nodes * 8 / 2) / namespaces))
  local expected=$((namespaces * 10 * per_deploy))
  if [ "$gpu_replicas" -gt 0 ]; then
    expected=$((namespaces * (9 * per_deploy + gpu_replicas)))
  fi
  echo "Creating $((namespaces * 10)) deployments, ${expected} pods in total..."
  for ((n = 0; n < namespaces; n++)); do
    local ns
    ns=$(printf "scale-%03d" "$n")
    for ((d = 0; d < 10; d++)); do
      if [ "$d" -eq 0 ] && [ "$gpu_replicas" -gt 0 ]; then
        deployment_yaml "$ns" "$d" "$gpu_replicas" true
      else
        deployment_yaml "$ns" "$d" "$per_deploy" false
      fi
    done
  done | kubectl apply -f - >/dev/null

  echo "Waiting for pods to be scheduled and running (Ctrl-C to stop waiting)..."
  while true; do
    local running
    running=$(kubectl get pods -A -l "$LABEL" --field-selector status.phase=Running --no-headers 2>/dev/null | wc -l | tr -d ' ')
    echo "  ${running} pods running"
    [ "$running" -ge "$expected" ] && break
    sleep 10
  done
}

down() {
  echo "Deleting scale-test namespaces, deployments and pods..."
  kubectl delete ns -l "$LABEL" --wait=false
  echo "Deleting scale-test nodes..."
  kubectl delete nodes -l "$LABEL" --wait=false
  echo "Done. The KWOK controller is left installed; remove it with:"
  echo "  kubectl delete -f https://github.com/kubernetes-sigs/kwok/releases/download/${KWOK_VERSION}/kwok.yaml"
}

case "$cmd" in
up) shift; up "$@" ;;
down) down ;;
*)
  sed -n '2,13p' "$0"
  exit 1
  ;;
esac
