// This is a generated file. Do not edit directly.

module k8s.io/kube-proxy

go 1.16

require (
	k8s.io/apimachinery v0.22.1
	k8s.io/component-base v0.22.1
)

replace (
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/onsi/ginkgo => github.com/openshift/onsi-ginkgo v1.14.1-0.20210831090728-b44d806b01b6
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/client-go => ../client-go
	k8s.io/component-base => ../component-base
	k8s.io/kube-proxy => ../kube-proxy
)
