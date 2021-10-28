// This is a generated file. Do not edit directly.

module k8s.io/sample-controller

go 1.16

require (
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/code-generator v0.22.1
	k8s.io/klog/v2 v2.9.0
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.54.0
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.7.0-origin.0+incompatible
	golang.org/x/net => golang.org/x/net v0.0.0-20210520170846-37e1c6afe023
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/client-go => ../client-go
	k8s.io/code-generator => ../code-generator
	k8s.io/sample-controller => ../sample-controller
)
