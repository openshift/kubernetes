module k8s.io/kms/plugins/mock

go 1.25.0

godebug default=go1.25

require (
	github.com/ThalesIgnite/crypto11 v1.2.5
	k8s.io/kms v0.0.0-00010101000000-000000000000
)

require (
	github.com/miekg/pkcs11 v1.0.3-0.20190429190417-a667d056470f // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/thales-e-security/pool v0.0.2 // indirect
	golang.org/x/net v0.47.0 // indirect
	golang.org/x/sys v0.38.0 // indirect
	golang.org/x/text v0.31.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250528174236-200df99c418a // indirect
	google.golang.org/grpc v1.72.2 // indirect
	google.golang.org/protobuf v1.36.8 // indirect
)

replace k8s.io/kms => ../../../../kms

replace github.com/onsi/ginkgo/v2 => github.com/openshift/onsi-ginkgo/v2 v2.6.1-0.20251120221002-696928a6a0d7

replace github.com/openshift/api => github.com/jacobsee/openshift-api v0.0.0-20260129191352-6736e67c1ff4

replace github.com/openshift/client-go => github.com/jacobsee/client-go v0.0.0-20260129193810-b0406a5c281a

replace github.com/openshift/library-go => github.com/jacobsee/library-go v0.0.0-20260129194329-86026b9dbf5e

replace github.com/openshift/apiserver-library-go => github.com/jacobsee/apiserver-library-go v0.0.0-20260129194922-f5d9320e4f1a
