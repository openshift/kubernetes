// This is a generated file. Do not edit directly.

module k8s.io/api

go 1.16

require (
	github.com/gogo/protobuf v1.3.2
	github.com/stretchr/testify v1.6.1
	k8s.io/apimachinery v0.21.0-rc.0
)

replace (
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.7.0-origin.0+incompatible
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
)
