// This is a generated file. Do not edit directly.

module k8s.io/sample-cli-plugin

go 1.13

require (
	github.com/emicklei/go-restful v2.9.5+incompatible // indirect
	github.com/go-openapi/jsonreference v0.19.3 // indirect
	github.com/go-openapi/spec v0.19.3 // indirect
	github.com/mailru/easyjson v0.7.0 // indirect
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	k8s.io/cli-runtime v0.0.0
	k8s.io/client-go v0.19.0-beta.0
)

replace (
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	golang.org/x/net => golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190813064441-fde4db37ae7a // pinned to release-branch.go1.13
	golang.org/x/tools => golang.org/x/tools v0.0.0-20190821162956-65e3620a7ae7 // pinned to release-branch.go1.13
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/cli-runtime => ../cli-runtime
	k8s.io/client-go => ../client-go
	k8s.io/sample-cli-plugin => ../sample-cli-plugin
)
