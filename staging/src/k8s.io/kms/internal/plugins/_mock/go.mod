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
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/text v0.29.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250303144028-a0af3efb3deb // indirect
	google.golang.org/grpc v1.72.1 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
)

replace k8s.io/kms => ../../../../kms

replace github.com/onsi/ginkgo/v2 => github.com/openshift/onsi-ginkgo/v2 v2.6.1-0.20251001123353-fd5b1fb35db1

replace github.com/openshift/api => github.com/jacobsee/openshift-api v0.0.0-20250916181341-dcc007c79f9d

replace github.com/openshift/client-go => github.com/jacobsee/client-go v0.0.0-20250916184653-15813ed6e1cc

replace github.com/openshift/library-go => github.com/jacobsee/library-go v0.0.0-20250916191828-0f1b364fffdb

replace github.com/openshift/apiserver-library-go => github.com/jacobsee/apiserver-library-go v0.0.0-20250916205517-cf97543eed12
