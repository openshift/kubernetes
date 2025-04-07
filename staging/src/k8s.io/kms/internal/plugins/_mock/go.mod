module k8s.io/kms/plugins/mock

go 1.24.0

godebug default=go1.24

require (
	github.com/ThalesIgnite/crypto11 v1.2.5
	k8s.io/kms v0.0.0-00010101000000-000000000000
)

require (
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/miekg/pkcs11 v1.0.3-0.20190429190417-a667d056470f // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/thales-e-security/pool v0.0.2 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241209162323-e6fa225c2576 // indirect
	google.golang.org/grpc v1.68.1 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
)

replace k8s.io/kms => ../../../../kms

replace github.com/onsi/ginkgo/v2 => github.com/openshift/onsi-ginkgo/v2 v2.6.1-0.20241205171354-8006f302fd12

replace github.com/openshift/api => github.com/bertinatto/api v0.0.0-20250401164618-1203d4e93857

replace github.com/openshift/client-go => github.com/bertinatto/client-go v0.0.0-20250401170358-6d78be609005

replace github.com/openshift/library-go => github.com/bertinatto/library-go v0.0.0-20250401170653-bfd1779cb06a

replace github.com/openshift/apiserver-library-go => github.com/bertinatto/apiserver-library-go v0.0.0-20250401171748-62d43d5fe914
