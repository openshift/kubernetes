module k8s.io/kms/plugins/mock

go 1.24.0

godebug default=go1.24

require (
	github.com/ThalesIgnite/crypto11 v1.2.5
	k8s.io/kms v0.0.0-00010101000000-000000000000
)

require (
	github.com/miekg/pkcs11 v1.0.3-0.20190429190417-a667d056470f // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/thales-e-security/pool v0.0.2 // indirect
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250303144028-a0af3efb3deb // indirect
	google.golang.org/grpc v1.72.1 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
)

replace k8s.io/kms => ../../../../kms

replace github.com/onsi/ginkgo/v2 => github.com/openshift/onsi-ginkgo/v2 v2.6.1-0.20250416174521-4eb003743b54

replace github.com/openshift/api => github.com/jacobsee/openshift-api v0.0.0-20250729042826-6bd2705927ce

replace github.com/openshift/client-go => github.com/jacobsee/client-go v0.0.0-20250729044348-b8b6d6a299ca

replace github.com/openshift/library-go => github.com/jacobsee/library-go v0.0.0-20250730000442-e9ff9357275c

replace github.com/openshift/apiserver-library-go => github.com/jacobsee/apiserver-library-go v0.0.0-20250730003030-4cbacf92cdca
