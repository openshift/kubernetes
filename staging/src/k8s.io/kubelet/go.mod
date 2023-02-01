// This is a generated file. Do not edit directly.

module k8s.io/kubelet

go 1.19

require (
	github.com/gogo/protobuf v1.3.2
<<<<<<< HEAD
	golang.org/x/net v0.0.0-20220909164309-bea034e7d591
=======
	golang.org/x/net v0.3.1-0.20221206200815-1e63c2f08a10
>>>>>>> v1.25.5
	google.golang.org/grpc v1.47.0
	k8s.io/api v0.25.0
	k8s.io/apimachinery v0.25.0
	k8s.io/component-base v0.25.0
)

require (
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.8 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/spf13/cobra v1.4.0 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
<<<<<<< HEAD
	golang.org/x/sys v0.0.0-20220728004956-3c1f35247d10 // indirect
	golang.org/x/text v0.3.7 // indirect
=======
	golang.org/x/sys v0.3.0 // indirect
	golang.org/x/text v0.5.0 // indirect
>>>>>>> v1.25.5
	google.golang.org/genproto v0.0.0-20220502173005-c8bf987b8c21 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	k8s.io/klog/v2 v2.80.1 // indirect
	k8s.io/utils v0.0.0-20220823124924-e9cbc92d1a73 // indirect
	sigs.k8s.io/json v0.0.0-20220713155537-f223a00ba0e2 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.2.0 // indirect
)

replace (
	github.com/google/gofuzz => github.com/google/gofuzz v1.1.0
	github.com/matttproud/golang_protobuf_extensions => github.com/matttproud/golang_protobuf_extensions v1.0.1
	github.com/onsi/ginkgo/v2 => github.com/openshift/onsi-ginkgo/v2 v2.0.0-20221005160638-5fa9cd70cd8c
	golang.org/x/crypto => golang.org/x/crypto v0.0.0-20220315160706-3147a52a75dd
	golang.org/x/lint => golang.org/x/lint v0.0.0-20190930215403-16217165b5de
	golang.org/x/net => golang.org/x/net v0.0.0-20220722155237-a158d28d115b
	golang.org/x/oauth2 => golang.org/x/oauth2 v0.0.0-20211104180415-d3ed0bb246c8
	golang.org/x/sys => golang.org/x/sys v0.0.0-20220722155257-8c9f86f7a55f
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/client-go => ../client-go
	k8s.io/component-base => ../component-base
	k8s.io/klog/v2 => k8s.io/klog/v2 v2.70.1
	k8s.io/kubelet => ../kubelet
	k8s.io/utils => k8s.io/utils v0.0.0-20220728103510-ee6ede2d64ed
)
