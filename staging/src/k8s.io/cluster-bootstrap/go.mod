// This is a generated file. Do not edit directly.

module k8s.io/cluster-bootstrap

go 1.16

require (
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20210220033148-5ea612d1eb83 // indirect
	gopkg.in/square/go-jose.v2 v2.2.2
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/klog/v2 v2.9.0
)

replace (
	github.com/onsi/ginkgo => github.com/openshift/onsi-ginkgo v1.14.1-0.20210831090728-b44d806b01b6
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/cluster-bootstrap => ../cluster-bootstrap
)
