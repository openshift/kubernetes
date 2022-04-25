// This is a generated file. Do not edit directly.

module k8s.io/cli-runtime

go 1.16

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/evanphx/json-patch v4.12.0+incompatible
	github.com/google/gnostic v0.5.7-v3refs
	github.com/google/uuid v1.1.2
	github.com/liggitt/tabwriter v0.0.0-20181228230101-89fcab3d43de
	github.com/spf13/cobra v1.4.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	golang.org/x/text v0.3.7
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.24.0-rc.0
	k8s.io/apimachinery v0.24.0-rc.0
	k8s.io/client-go v0.24.0-rc.0
	k8s.io/kube-openapi v0.0.0-20220328201542-3ee0da9b0b42
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	sigs.k8s.io/kustomize/api v0.11.4
	sigs.k8s.io/kustomize/kyaml v0.13.6
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.7.0-origin.0+incompatible

	github.com/openshift/api => github.com/tkashem/api v0.0.0-20220421200455-641a165d1cca
	github.com/openshift/apiserver-library-go => github.com/tkashem/apiserver-library-go v0.0.0-20220421212124-5950ef701ec5
	github.com/openshift/client-go => github.com/tkashem/openshift-client-go v0.0.0-20220421203740-dddeb4eb20b7
	github.com/openshift/library-go => github.com/tkashem/library-go v0.0.0-20220421211142-607a089b3f0b
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
