module gitlab.com/davidxarnold/glance

go 1.14

require (
	cloud.google.com/go v0.38.0
	github.com/aws/aws-sdk-go-v2 v0.22.0
	github.com/gizak/termui/v3 v3.1.0
	github.com/go-echarts/go-echarts/v2 v2.2.1
	github.com/golangci/golangci-lint v1.18.0
	github.com/jedib0t/go-pretty v4.3.0+incompatible
	github.com/jedib0t/go-pretty/v6 v6.0.5
	github.com/mitchellh/go-homedir v1.1.0
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/viper v1.6.2
	google.golang.org/genproto v0.0.0-20190502173448-54afdca5d873
	k8s.io/api v0.17.4
	k8s.io/apimachinery v0.17.4
	k8s.io/cli-runtime v0.17.4
	k8s.io/client-go v0.17.4
	k8s.io/kubectl v0.17.4
	k8s.io/kubernetes v1.17.3
	k8s.io/metrics v0.17.4

)

replace (
	k8s.io/api v0.0.0 => k8s.io/api v0.17.4
	k8s.io/apiextensions-apiserver v0.0.0 => k8s.io/apiextensions-apiserver v0.17.4
	k8s.io/apimachinery v0.0.0 => k8s.io/apimachinery v0.17.4
	k8s.io/apiserver v0.0.0 => k8s.io/apiserver v0.17.4
	k8s.io/cli-runtime v0.0.0 => k8s.io/cli-runtime v0.17.4
	k8s.io/client-go v0.0.0 => k8s.io/client-go v0.17.4
	k8s.io/cloud-provider v0.0.0 => k8s.io/cloud-provider v0.17.4
	k8s.io/cluster-bootstrap v0.0.0 => k8s.io/cluster-bootstrap v0.17.4
	k8s.io/code-generator v0.0.0 => k8s.io/code-generator v0.17.4
	k8s.io/component-base v0.0.0 => k8s.io/component-base v0.17.4
	k8s.io/cri-api v0.0.0 => k8s.io/cri-api v0.17.4
	k8s.io/csi-translation-lib v0.0.0 => k8s.io/csi-translation-lib v0.17.4
	k8s.io/kube-aggregator v0.0.0 => k8s.io/kube-aggregator v0.17.4
	k8s.io/kube-controller-manager v0.0.0 => k8s.io/kube-controller-manager v0.17.4
	k8s.io/kube-proxy v0.0.0 => k8s.io/kube-proxy v0.17.4
	k8s.io/kube-scheduler v0.0.0 => k8s.io/kube-scheduler v0.17.4
	k8s.io/kubectl v0.0.0 => k8s.io/kubectl v0.17.4
	k8s.io/kubelet v0.0.0 => k8s.io/kubelet v0.17.4
	k8s.io/legacy-cloud-providers v0.0.0 => k8s.io/legacy-cloud-providers v0.17.4
	k8s.io/metrics v0.0.0 => k8s.io/metrics v0.17.4
	k8s.io/sample-apiserver v0.0.0 => k8s.io/sample-apiserver v0.17.4
)
