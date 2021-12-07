// This is a generated file. Do not edit directly.

module k8s.io/metrics

go 1.16

require (
	github.com/gogo/protobuf v1.3.2
	github.com/stretchr/testify v1.7.0
	k8s.io/api v0.23.0-rc.0
	k8s.io/apimachinery v0.23.0-rc.0
	k8s.io/client-go v0.23.0-rc.0
	k8s.io/code-generator v0.23.0-rc.0
)

replace (
	github.com/hashicorp/golang-lru => github.com/hashicorp/golang-lru v0.5.0
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/mattn/go-colorable => github.com/mattn/go-colorable v0.0.9
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.7.0-origin.0+incompatible

	github.com/openshift/api => github.com/damemi/api v0.0.0-20211203171043-fac64b2988f0
	github.com/openshift/apiserver-library-go => github.com/damemi/apiserver-library-go v0.0.0-20211206170242-12b800d6e005
	github.com/openshift/client-go => github.com/damemi/client-go v0.0.0-20211203172409-e87791f0ec9b
	github.com/openshift/library-go => github.com/damemi/library-go v0.0.0-20211206171111-1dacda958b43
	k8s.io/api => ../api
	k8s.io/apiextensions-apiserver => ../apiextensions-apiserver
	k8s.io/apimachinery => ../apimachinery
	k8s.io/apiserver => ../apiserver
	k8s.io/cli-runtime => ../cli-runtime
	k8s.io/client-go => ../client-go
	k8s.io/cloud-provider => ../cloud-provider
	k8s.io/cluster-bootstrap => ../cluster-bootstrap
	k8s.io/code-generator => ../code-generator
	k8s.io/component-base => ../component-base
	k8s.io/component-helpers => ../component-helpers
	k8s.io/controller-manager => ../controller-manager
	k8s.io/cri-api => ../cri-api
	k8s.io/csi-translation-lib => ../csi-translation-lib
	k8s.io/kube-aggregator => ../kube-aggregator
	k8s.io/kube-controller-manager => ../kube-controller-manager
	k8s.io/kube-proxy => ../kube-proxy
	k8s.io/kube-scheduler => ../kube-scheduler
	k8s.io/kubectl => ../kubectl
	k8s.io/kubelet => ../kubelet
	k8s.io/legacy-cloud-providers => ../legacy-cloud-providers
	k8s.io/metrics => ../metrics
	k8s.io/mount-utils => ../mount-utils
	k8s.io/pod-security-admission => ../pod-security-admission
	k8s.io/sample-apiserver => ../sample-apiserver
	sigs.k8s.io/structured-merge-diff/v4 => sigs.k8s.io/structured-merge-diff/v4 v4.1.2
)
