// This is a generated file. Do not edit directly.

module k8s.io/cluster-bootstrap

go 1.16

require (
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292 // indirect
	gopkg.in/square/go-jose.v2 v2.2.2
	k8s.io/api v0.24.0
	k8s.io/apimachinery v0.24.0
	k8s.io/klog/v2 v2.60.1
)

replace (
	github.com/onsi/ginkgo => github.com/bparees/onsi-ginkgo v1.14.0-unpatch
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20211104180415-d3ed0bb246c8
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/cluster-bootstrap => ../cluster-bootstrap
)
