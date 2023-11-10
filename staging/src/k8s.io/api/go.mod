// This is a generated file. Do not edit directly.

module k8s.io/api

go 1.20

require (
	github.com/gogo/protobuf v1.3.2
	github.com/stretchr/testify v1.8.4
	k8s.io/apimachinery v0.29.0-alpha.3
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.10.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/klog/v2 v2.110.1 // indirect
	k8s.io/utils v0.0.0-20230726121419-3b25d923346b // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.4.1 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	github.com/onsi/ginkgo/v2 => github.com/soltysh/ginkgo/v2 v2.1.5-0.20231031162821-c5e24be53ea7
	github.com/openshift/api => github.com/soltysh/api v0.0.0-20231110161527-403cf2b0c634
	github.com/openshift/apiserver-library-go => github.com/soltysh/apiserver-library-go v0.0.0-20231110164844-a54c851983e7
	github.com/openshift/client-go => github.com/soltysh/client-go v0.0.0-20231110162106-5c78b94630e0
	github.com/openshift/library-go => github.com/soltysh/library-go v0.0.0-20231110163950-c30e6a3ff459
	k8s.io/api => ../api
	k8s.io/apiextensions-apiserver => ../apiextensions-apiserver
	k8s.io/apimachinery => ../apimachinery
	k8s.io/apiserver => ../apiserver
	k8s.io/client-go => ../client-go
	k8s.io/code-generator => ../code-generator
	k8s.io/component-base => ../component-base
	k8s.io/component-helpers => ../component-helpers
	k8s.io/kms => ../kms
	k8s.io/kube-aggregator => ../kube-aggregator
)
