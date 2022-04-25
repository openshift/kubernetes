// This is a generated file. Do not edit directly.

module k8s.io/sample-apiserver

go 1.16

require (
	github.com/google/gofuzz v1.1.0
	github.com/spf13/afero v1.6.0 // indirect
	github.com/spf13/cobra v1.4.0
	k8s.io/apimachinery v0.24.0-rc.0
	k8s.io/apiserver v0.24.0-rc.0
	k8s.io/client-go v0.24.0-rc.0
	k8s.io/code-generator v0.24.0-rc.0
	k8s.io/component-base v0.24.0-rc.0
	k8s.io/kube-openapi v0.0.0-20220328201542-3ee0da9b0b42
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
)

replace (
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.7.0-origin.0+incompatible

	github.com/openshift/api => github.com/tkashem/api v0.0.0-20220421200455-641a165d1cca
	github.com/openshift/client-go => github.com/tkashem/openshift-client-go v0.0.0-20220421203740-dddeb4eb20b7
	github.com/openshift/library-go => github.com/tkashem/library-go v0.0.0-20220421211142-607a089b3f0b
	k8s.io/api => ../api
	k8s.io/apiextensions-apiserver => ../apiextensions-apiserver
	k8s.io/apimachinery => ../apimachinery
	k8s.io/apiserver => ../apiserver
	k8s.io/client-go => ../client-go
	k8s.io/code-generator => ../code-generator
	k8s.io/component-base => ../component-base
	k8s.io/kube-aggregator => ../kube-aggregator
	k8s.io/sample-apiserver => ../sample-apiserver
)
