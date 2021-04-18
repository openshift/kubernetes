// This is a generated file. Do not edit directly.

module k8s.io/client-go

go 1.16

require (
	github.com/Azure/go-autorest/autorest v0.11.12
	github.com/Azure/go-autorest/autorest/adal v0.9.5
	github.com/davecgh/go-spew v1.1.1
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/gogo/protobuf v1.3.2
	github.com/golang/groupcache v0.0.0-20200121045136-8c9f03a8e57e
	github.com/golang/protobuf v1.4.3
	github.com/google/go-cmp v0.5.2
	github.com/google/gofuzz v1.1.0
	github.com/google/uuid v1.1.2
	github.com/googleapis/gnostic v0.4.1
	github.com/gregjones/httpcache v0.0.0-20180305231024-9cad4c3443a7
	github.com/imdario/mergo v0.3.7
	github.com/peterbourgon/diskv v2.0.1+incompatible
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	golang.org/x/net v0.0.0-20210224082022-3d97a244fca7
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	golang.org/x/term v0.0.0-20210220032956-6a3ed077a48d
	golang.org/x/time v0.0.0-20210220033141-f8bda1e9f3ba
	k8s.io/api v0.21.0-rc.0
	k8s.io/apimachinery v0.21.0-rc.0
	k8s.io/klog/v2 v2.8.0
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/structured-merge-diff/v4 v4.1.0
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/go-bindata/go-bindata => github.com/go-bindata/go-bindata v3.1.1+incompatible
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/mattn/go-colorable => github.com/mattn/go-colorable v0.0.9
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.7.0-origin.0+incompatible
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20210407181114-454af31d1145
	github.com/robfig/cron => github.com/robfig/cron v1.1.0
	go.uber.org/multierr => go.uber.org/multierr v1.1.0
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
	k8s.io/component-helpers => ../component-helpers
	k8s.io/controller-manager => ../controller-manager
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
	k8s.io/mount-utils => ../mount-utils
	k8s.io/sample-apiserver => ../sample-apiserver
)
