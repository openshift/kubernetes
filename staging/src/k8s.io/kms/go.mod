// This is a generated file. Do not edit directly.

module k8s.io/kms

go 1.22.0

require (
	github.com/gogo/protobuf v1.3.2
	google.golang.org/grpc v1.65.0
)

require (
	golang.org/x/net v0.29.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240924160255-9d4c2d233b61 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)

replace (
	golang.org/x/net => github.com/openshift-sustaining/net v0.35.0-sec.4
	k8s.io/kms => ../kms
)
