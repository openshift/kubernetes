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

replace github.com/openshift/api => github.com/jubittajohn/api v0.0.0-20260420180837-5e43903e1ea6

replace github.com/openshift/client-go => github.com/jubittajohn/client-go v0.0.0-20260420191538-713b208761a8

replace github.com/openshift/library-go => github.com/jubittajohn/library-go v0.0.0-20260421171937-ae79a5397876

replace github.com/openshift/apiserver-library-go => github.com/jubittajohn/apiserver-library-go v0.0.0-20260421193111-691abe80a8d3

replace github.com/onsi/ginkgo/v2 => github.com/jubittajohn/onsi-ginkgo/v2 v2.6.1-0.20260424191255-73ef08c78ea8
