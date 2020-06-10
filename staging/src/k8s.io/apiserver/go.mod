// This is a generated file. Do not edit directly.

module k8s.io/apiserver

go 1.13

require (
	github.com/coreos/go-oidc v2.1.0+incompatible
	github.com/coreos/go-systemd v0.0.0-20190321100706-95778dfbb74e
	github.com/coreos/pkg v0.0.0-20180928190104-399ea9e2e55f
	github.com/davecgh/go-spew v1.1.1
	github.com/emicklei/go-restful v2.9.5+incompatible
	github.com/evanphx/json-patch v4.2.0+incompatible
	github.com/go-openapi/spec v0.19.3
	github.com/gogo/protobuf v1.3.1
	github.com/google/go-cmp v0.4.0
	github.com/google/gofuzz v1.1.0
	github.com/google/uuid v1.1.1
	github.com/googleapis/gnostic v0.4.1
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/hashicorp/golang-lru v0.5.1
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822
	github.com/pkg/errors v0.9.1
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	go.etcd.io/etcd v0.5.0-alpha.5.0.20200401174654-e694b7bb0875
	go.uber.org/zap v1.10.0
	golang.org/x/crypto v0.0.0-20200220183623-bac4c82f6975
	golang.org/x/net v0.0.0-20200421231249-e086a090c8fd
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
	golang.org/x/sys v0.0.0-20200420163511-1957bb5e6d1f
	google.golang.org/grpc v1.26.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/square/go-jose.v2 v2.2.2
	gopkg.in/yaml.v2 v2.2.8
	k8s.io/api v0.19.0-beta.0
	k8s.io/apimachinery v0.19.0-beta.0
	k8s.io/client-go v0.19.0-beta.0
	k8s.io/component-base v0.19.0-beta.0
	k8s.io/klog/v2 v2.0.0
	k8s.io/kube-openapi v0.0.0-20200427153329-656914f816f9
	k8s.io/utils v0.0.0-20200414100711-2df71ebbae66
	sigs.k8s.io/apiserver-network-proxy/konnectivity-client v0.0.9-0.20200513220823-33b997865007
	sigs.k8s.io/structured-merge-diff/v3 v3.0.0
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/coreos/etcd => github.com/coreos/etcd v3.3.10+incompatible
	github.com/go-bindata/go-bindata => github.com/go-bindata/go-bindata v3.1.1+incompatible
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/openshift/api => github.com/marun/api v0.0.0-20200609033806-8e69090a7113
	github.com/openshift/apiserver-library-go => github.com/marun/apiserver-library-go v0.0.0-20200609040356-384080df6404
	github.com/openshift/client-go => github.com/marun/client-go v0.0.0-20200609034822-fb1a8b715a48
	github.com/openshift/library-go => github.com/marun/library-go v0.0.0-20200609040156-3e167ee15a14
	go.uber.org/multierr => go.uber.org/multierr v1.1.0
	golang.org/x/net => golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	golang.org/x/sys => golang.org/x/sys v0.0.0-20190813064441-fde4db37ae7a // pinned to release-branch.go1.13
	golang.org/x/tools => golang.org/x/tools v0.0.0-20190821162956-65e3620a7ae7 // pinned to release-branch.go1.13
	k8s.io/api => ../api
	k8s.io/apiextensions-apiserver => ../apiextensions-apiserver
	k8s.io/apimachinery => ../apimachinery
	k8s.io/apiserver => ../apiserver
	k8s.io/cli-runtime => ../cli-runtime
	k8s.io/client-go => ../client-go
	k8s.io/cloud-provider => ../cloud-provider
	k8s.io/cluster-bootstrap => ../cluster-bootstrap
	k8s.io/code-generator => ../code-generator
	k8s.io/component-base => ../component-base
	k8s.io/cri-api => ../cri-api
	k8s.io/csi-translation-lib => ../csi-translation-lib
	k8s.io/kube-aggregator => ../kube-aggregator
	k8s.io/kube-controller-manager => ../kube-controller-manager
	k8s.io/kube-proxy => ../kube-proxy
	k8s.io/kube-scheduler => ../kube-scheduler
	k8s.io/kubectl => ../kubectl
	k8s.io/kubelet => ../kubelet
	k8s.io/legacy-cloud-providers => ../legacy-cloud-providers
	k8s.io/metrics => ../metrics
	k8s.io/sample-apiserver => ../sample-apiserver
	vbom.ml/util => vbom.ml/util v0.0.0-20160121211510-db5cfe13f5cc
)
