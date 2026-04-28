module k8s.io/kms/plugins/mock

go 1.26.0

godebug default=go1.26

require (
	github.com/ThalesIgnite/crypto11 v1.2.5
	k8s.io/kms v0.0.0-00010101000000-000000000000
)

require (
	github.com/miekg/pkcs11 v1.0.3-0.20190429190417-a667d056470f // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/thales-e-security/pool v0.0.2 // indirect
	golang.org/x/net v0.49.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	golang.org/x/text v0.33.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260128011058-8636f8732409 // indirect
	google.golang.org/grpc v1.79.3 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
)

replace k8s.io/kms => ../../../../kms

replace github.com/openshift/api => github.com/jubittajohn/api v0.0.0-20260427143949-fa2a8fc32379

replace github.com/openshift/client-go => github.com/jubittajohn/client-go v0.0.0-20260427145758-ec18c956ffc2

replace github.com/openshift/library-go => github.com/jubittajohn/library-go v0.0.0-20260427152643-85540ba4a3d0

replace github.com/openshift/apiserver-library-go => github.com/jubittajohn/apiserver-library-go v0.0.0-20260427155350-b4c8b74c1696

replace github.com/onsi/ginkgo/v2 => github.com/openshift/onsi-ginkgo/v2 v2.6.1-0.20260424201627-4d4cc33d669d
