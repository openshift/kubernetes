// This is a generated file. Do not edit directly.

module k8s.io/component-base

go 1.16

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/go-logr/logr v0.4.0
	github.com/google/go-cmp v0.5.5
	github.com/moby/term v0.0.0-20210610120745-9d4ed1856297
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/client_model v0.2.0
	github.com/prometheus/common v0.26.0
	github.com/prometheus/procfs v0.6.0
	github.com/spf13/cobra v1.1.3
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.20.0
	go.opentelemetry.io/otel v0.20.0
	go.opentelemetry.io/otel/exporters/otlp v0.20.0
	go.opentelemetry.io/otel/sdk v0.20.0
	go.opentelemetry.io/otel/trace v0.20.0
	go.uber.org/zap v1.17.0
	golang.org/x/sys v0.0.0-20210616094352-59db8d763f22
	k8s.io/apimachinery v0.22.0-rc.0
	k8s.io/client-go v0.22.0-rc.0
	k8s.io/klog/v2 v2.9.0
	k8s.io/utils v0.0.0-20210819203725-bdf08cb9a70a
)

replace (
	github.com/google/cadvisor => github.com/openshift/google-cadvisor v0.33.2-0.20210825140152-3e7875ab2744
	github.com/imdario/mergo => github.com/imdario/mergo v0.3.5
	github.com/mattn/go-colorable => github.com/mattn/go-colorable v0.0.9
	github.com/onsi/ginkgo => github.com/openshift/ginkgo v4.7.0-origin.0+incompatible
	github.com/openshift/api => github.com/openshift/api v0.0.0-20210817132244-67c28690af52
	github.com/openshift/apiserver-library-go => github.com/openshift/apiserver-library-go v0.0.0-20210823130908-fb12da91a79d
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20210730113412-1811c1b3fc0e
	github.com/openshift/library-go => github.com/openshift/library-go v0.0.0-20210826214843-cc316ba4331b
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
	k8s.io/pod-security-admission => ../pod-security-admission
	k8s.io/sample-apiserver => ../sample-apiserver
)
