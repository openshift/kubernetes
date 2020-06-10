// This is a generated file. Do not edit directly.

module k8s.io/sample-apiserver

go 1.13

require (
	github.com/coreos/go-semver v0.3.0 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/go-openapi/spec v0.19.3
	github.com/google/gofuzz v1.1.0
	github.com/spf13/cobra v1.0.0
	go.uber.org/multierr v1.1.1-0.20180122172545-ddea229ff1df // indirect
	google.golang.org/genproto v0.0.0-20200115191322-ca5a22157cba // indirect
	k8s.io/apimachinery v0.19.0-beta.0
	k8s.io/apiserver v0.19.0-beta.0
	k8s.io/client-go v0.19.0-beta.0
	k8s.io/code-generator v0.19.0-beta.0
	k8s.io/component-base v0.19.0-beta.0
	k8s.io/klog/v2 v2.0.0
	k8s.io/kube-openapi v0.0.0-20200427153329-656914f816f9
)

replace (
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	go.uber.org/multierr => go.uber.org/multierr v1.1.0
	golang.org/x/net => golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190813064441-fde4db37ae7a // pinned to release-branch.go1.13
	golang.org/x/tools => golang.org/x/tools v0.0.0-20190821162956-65e3620a7ae7 // pinned to release-branch.go1.13
	k8s.io/api => ../api
	k8s.io/apimachinery => ../apimachinery
	k8s.io/apiserver => ../apiserver
	k8s.io/client-go => ../client-go
	k8s.io/code-generator => ../code-generator
	k8s.io/component-base => ../component-base
	k8s.io/sample-apiserver => ../sample-apiserver
)
