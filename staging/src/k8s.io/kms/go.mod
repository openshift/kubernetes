// This is a generated file. Do not edit directly.

module k8s.io/kms

go 1.20

require (
	github.com/gogo/protobuf v1.3.2
	google.golang.org/grpc v1.54.0
	k8s.io/apimachinery v0.28.0-alpha.3
	k8s.io/client-go v0.28.0-alpha.3
	k8s.io/klog/v2 v2.100.1
)

require (
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	golang.org/x/net v0.12.0 // indirect
	golang.org/x/sys v0.10.0 // indirect
	golang.org/x/text v0.11.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230525234030-28d5490b6b19 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	k8s.io/utils v0.0.0-20230505201702-9f6742963106 // indirect
)

replace (
	github.com/onsi/ginkgo/v2 => github.com/openshift/onsi-ginkgo/v2 v2.6.1-0.20230317131656-c62d9de5a460
	github.com/openshift/api => github.com/bertinatto/api v0.0.0-20230623115844-695ccc949fee
	github.com/openshift/apiserver-library-go => github.com/bertinatto/apiserver-library-go v0.0.0-20230623125834-704f0638dabc
	github.com/openshift/client-go => github.com/bertinatto/client-go v0.0.0-20230623124427-bc27227b82fc
	github.com/openshift/library-go => github.com/bertinatto/library-go v0.0.0-20230623125208-9fbc0120376c
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
