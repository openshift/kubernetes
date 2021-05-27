// This is a generated file. Do not edit directly.

module k8s.io/sample-cli-plugin

go 1.16

require (
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	google.golang.org/appengine v1.6.5 // indirect
	k8s.io/cli-runtime v0.0.0
	k8s.io/client-go v0.21.0-rc.0
)

replace (
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.7.0-origin.0+incompatible
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/cli-runtime => ../cli-runtime
	k8s.io/client-go => ../client-go
	k8s.io/sample-cli-plugin => ../sample-cli-plugin
)
