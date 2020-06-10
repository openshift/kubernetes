// This is a generated file. Do not edit directly.

module k8s.io/metrics

go 1.13

require (
	github.com/gogo/protobuf v1.3.1
	github.com/stretchr/testify v1.4.0
	k8s.io/api v0.19.0-beta.0
	k8s.io/apimachinery v0.19.0-beta.0
	k8s.io/client-go v0.19.0-beta.0
	k8s.io/code-generator v0.19.0-beta.0
)

replace (
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	golang.org/x/net => golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190813064441-fde4db37ae7a // pinned to release-branch.go1.13
	golang.org/x/tools => golang.org/x/tools v0.0.0-20190821162956-65e3620a7ae7 // pinned to release-branch.go1.13
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/client-go => ../client-go
	k8s.io/code-generator => ../code-generator
	k8s.io/metrics => ../metrics
)
