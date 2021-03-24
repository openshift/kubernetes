// This is a generated file. Do not edit directly.

module k8s.io/kube-proxy

go 1.15

require (
	k8s.io/apimachinery v0.20.0
	k8s.io/component-base v0.20.0
)

replace (
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.5.0-origin.1+incompatible
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/client-go => ../client-go
	k8s.io/component-base => ../component-base
	k8s.io/kube-proxy => ../kube-proxy
)
