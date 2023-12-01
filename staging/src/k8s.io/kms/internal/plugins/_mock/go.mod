module k8s.io/kms/plugins/mock

go 1.20

require (
	github.com/ThalesIgnite/crypto11 v1.2.5
	k8s.io/kms v0.0.0-00010101000000-000000000000
)

require (
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/miekg/pkcs11 v1.0.3-0.20190429190417-a667d056470f // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/thales-e-security/pool v0.0.2 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/grpc v1.58.3 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)

replace k8s.io/kms => ../../../../kms

replace github.com/openshift/api => github.com/sairameshv/api v0.0.0-20231130084455-c51ac830066b

replace github.com/openshift/client-go => github.com/sairameshv/client-go v0.0.0-20231130101019-60fbcc739714

replace github.com/openshift/library-go => github.com/sairameshv/library-go v0.0.0-20231130102346-3fe382d67441

replace github.com/openshift/apiserver-library-go => github.com/sairameshv/apiserver-library-go v0.0.0-20231130102837-38360a8649ff

replace github.com/onsi/ginkgo/v2 => github.com/openshift/onsi-ginkgo/v2 v2.6.1-0.20231031162821-c5e24be53ea7
