// This is a generated file. Do not edit directly.

module k8s.io/code-generator

go 1.16

require (
	github.com/emicklei/go-restful v2.9.5+incompatible // indirect
	github.com/go-logr/logr v1.1.0 // indirect
	github.com/go-openapi/jsonpointer v0.19.5 // indirect
	github.com/go-openapi/jsonreference v0.19.5 // indirect
	github.com/go-openapi/swag v0.19.14 // indirect
	github.com/gogo/protobuf v1.3.2
	github.com/googleapis/gnostic v0.5.5
<<<<<<< HEAD
	github.com/hpcloud/tail v1.0.0 // indirect
	github.com/json-iterator/go v1.1.11 // indirect
	github.com/onsi/ginkgo v4.7.0-origin.0+incompatible // indirect
	github.com/onsi/gomega v1.10.1 // indirect
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0 // indirect
	golang.org/x/net v0.0.0-20210520170846-37e1c6afe023 // indirect
	golang.org/x/sys v0.0.0-20210616094352-59db8d763f22 // indirect
	golang.org/x/tools v0.1.2 // indirect
	gopkg.in/fsnotify.v1 v1.4.7 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
=======
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/onsi/ginkgo v1.14.0 // indirect
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0 // indirect
	golang.org/x/net v0.0.0-20210813160813-60bc85c4be6d // indirect
	golang.org/x/sys v0.0.0-20210820121016-41cdb8703e55 // indirect
	golang.org/x/tools v0.1.6-0.20210820212750-d4cc65f0b2ff
>>>>>>> v1.23.0-alpha.3
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/gengo v0.0.0-20210813121822-485abfe95c7c
	k8s.io/klog/v2 v2.20.0
	k8s.io/kube-openapi v0.0.0-20210817084001-7fbd8d59e5b8
	sigs.k8s.io/structured-merge-diff/v4 v4.1.2 // indirect
)

replace (
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.7.0-origin.0+incompatible
	k8s.io/code-generator => ../code-generator
)
