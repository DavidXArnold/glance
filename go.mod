module github.com/davidxarnold/glance

go 1.13

require (
	github.com/go-openapi/strfmt v0.19.4 // indirect
	github.com/jedib0t/go-pretty v4.3.0+incompatible
	github.com/mattn/go-runewidth v0.0.8 // indirect
	github.com/mitchellh/go-homedir v1.1.0
	github.com/open-policy-agent/opa v0.17.2
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.6.2
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/cli-runtime v0.17.3
	k8s.io/client-go v0.17.3
	k8s.io/kubectl v0.17.3
	k8s.io/kubernetes v1.17.3
	k8s.io/metrics v0.17.3
)

replace (
	k8s.io/api v0.0.0 => k8s.io/api v0.17.3
	k8s.io/apiextensions-apiserver v0.0.0 => k8s.io/apiextensions-apiserver v0.17.3
	k8s.io/apimachinery v0.0.0 => k8s.io/apimachinery v0.17.3
	k8s.io/apiserver v0.0.0 => k8s.io/apiserver v0.17.3
	k8s.io/cli-runtime v0.0.0 => k8s.io/cli-runtime v0.17.3
	k8s.io/client-go v0.0.0 => k8s.io/client-go v0.17.3
	k8s.io/cloud-provider v0.0.0 => k8s.io/cloud-provider v0.17.3
	k8s.io/cluster-bootstrap v0.0.0 => k8s.io/cluster-bootstrap v0.17.3
	k8s.io/code-generator v0.0.0 => k8s.io/code-generator v0.17.3
	k8s.io/component-base v0.0.0 => k8s.io/component-base v0.17.3
	k8s.io/cri-api v0.0.0 => k8s.io/cri-api v0.17.3
	k8s.io/csi-translation-lib v0.0.0 => k8s.io/csi-translation-lib v0.17.3
	k8s.io/kube-aggregator v0.0.0 => k8s.io/kube-aggregator v0.17.3
	k8s.io/kube-controller-manager v0.0.0 => k8s.io/kube-controller-manager v0.17.3
	k8s.io/kube-proxy v0.0.0 => k8s.io/kube-proxy v0.17.3
	k8s.io/kube-scheduler v0.0.0 => k8s.io/kube-scheduler v0.17.3
	k8s.io/kubectl v0.0.0 => k8s.io/kubectl v0.17.3
	k8s.io/kubelet v0.0.0 => k8s.io/kubelet v0.17.3
	k8s.io/legacy-cloud-providers v0.0.0 => k8s.io/legacy-cloud-providers v0.17.3
	k8s.io/metrics v0.0.0 => k8s.io/metrics v0.17.3
	k8s.io/sample-apiserver v0.0.0 => k8s.io/sample-apiserver v0.17.3
)
