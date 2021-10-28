// This is a generated file. Do not edit directly.

module k8s.io/kube-controller-manager

go 1.16

require (
	k8s.io/apimachinery v0.22.1
	k8s.io/cloud-provider v0.0.0
	k8s.io/controller-manager v0.0.0
)

replace (
	cloud.google.com/go => cloud.google.com/go v0.54.0
	github.com/cespare/xxhash/v2 => github.com/cespare/xxhash/v2 v2.1.1
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/mattn/go-colorable => github.com/mattn/go-colorable v0.0.9
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.7.0-origin.0+incompatible
	golang.org/x/net => golang.org/x/net v0.0.0-20210520170846-37e1c6afe023
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	k8s.io/api => ../api
	k8s.io/apiextensions-apiserver => ../apiextensions-apiserver
	k8s.io/apimachinery => ../apimachinery
	k8s.io/apiserver => ../apiserver
	k8s.io/client-go => ../client-go
	k8s.io/cloud-provider => ../cloud-provider
	k8s.io/code-generator => ../code-generator
	k8s.io/component-base => ../component-base
	k8s.io/controller-manager => ../controller-manager
	k8s.io/kube-aggregator => ../kube-aggregator
	k8s.io/kube-controller-manager => ../kube-controller-manager
)
