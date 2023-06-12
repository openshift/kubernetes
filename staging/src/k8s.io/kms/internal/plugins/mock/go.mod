module k8s.io/kms/plugins/mock

go 1.19

require (
	k8s.io/klog/v2 v2.100.1
	k8s.io/kms v0.0.0-00010101000000-000000000000
)

require (
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	golang.org/x/net v0.9.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
	golang.org/x/text v0.9.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	google.golang.org/genproto v0.0.0-20230110181048-76db0878b65f // indirect
	google.golang.org/grpc v1.54.0 // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	k8s.io/client-go v0.28.0-alpha.2 // indirect
	k8s.io/utils v0.0.0-20230505201702-9f6742963106 // indirect
)

replace (
	k8s.io/apimachinery => ../../../../apimachinery
	k8s.io/client-go => ../../../../client-go
	k8s.io/kms => ../../../../kms
)

replace github.com/openshift/api => github.com/bertinatto/api v0.0.0-20230612162802-c6bd62d2f62d

replace github.com/openshift/client-go => github.com/bertinatto/client-go v0.0.0-20230612171757-4fa46087d27b

replace github.com/openshift/library-go => github.com/bertinatto/library-go v0.0.0-20230612173447-732e2b28ed87

replace github.com/openshift/apiserver-library-go => github.com/bertinatto/apiserver-library-go v0.0.0-20230612175645-6648ff742591

replace github.com/onsi/ginkgo/v2 => github.com/openshift/onsi-ginkgo/v2 v2.6.1-0.20230317131656-c62d9de5a460
