module k8s.io/kms/plugins/mock

go 1.27.0

godebug default=go1.27

require (
	github.com/ThalesIgnite/crypto11 v1.2.5
	k8s.io/kms v0.0.0-00010101000000-000000000000
)

require (
	github.com/miekg/pkcs11 v1.0.3-0.20190429190417-a667d056470f // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/thales-e-security/pool v0.0.2 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/grpc v1.85.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace k8s.io/kms => ../../../../kms

replace github.com/openshift/api => github.com/jacobsee/openshift-api v0.0.0-20261110204218-fe7438bc0df3

replace github.com/openshift/client-go => github.com/jacobsee/client-go v0.0.0-20261110204527-88f20dd1ba8d

replace github.com/openshift/library-go => github.com/jacobsee/library-go v0.0.0-20261111235929-e51010ad40eb

replace github.com/openshift/apiserver-library-go => github.com/jacobsee/apiserver-library-go v0.0.0-20261112000038-64397adc7883
