// This is a generated file. Do not edit directly.

module k8s.io/cli-runtime

go 1.16

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/evanphx/json-patch v4.11.0+incompatible
	github.com/google/uuid v1.1.2
	github.com/googleapis/gnostic v0.5.5
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	golang.org/x/text v0.3.6
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.22.0-beta.2
	k8s.io/apimachinery v0.22.0-beta.2
	k8s.io/client-go v0.22.0-beta.2
	k8s.io/kube-openapi v0.0.0-20210421082810-95288971da7e
	sigs.k8s.io/kustomize/api v0.8.11
	sigs.k8s.io/kustomize/kyaml v0.11.0
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/mattn/go-colorable => github.com/mattn/go-colorable v0.0.9
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.7.0-origin.0+incompatible
	github.com/openshift/api => github.com/soltysh/api v0.0.0-20210719081803-9091ab00c164
	github.com/openshift/apiserver-library-go => github.com/soltysh/apiserver-library-go v0.0.0-20210719105544-c10b4ed4b61b
	github.com/openshift/client-go => github.com/soltysh/client-go v0.0.0-20210719082425-f8fde3619384
	github.com/openshift/library-go => github.com/soltysh/library-go v0.0.0-20210719104342-c952f4e07d0b
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
)
