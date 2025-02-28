package main

import (
	"flag"
	"os"
	"reflect"

	"k8s.io/kubernetes/test/e2e/framework"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
	"github.com/openshift-eng/openshift-tests-extension/pkg/extension"
	e "github.com/openshift-eng/openshift-tests-extension/pkg/extension"
	et "github.com/openshift-eng/openshift-tests-extension/pkg/extension/extensiontests"
	g "github.com/openshift-eng/openshift-tests-extension/pkg/ginkgo"
	v "github.com/openshift-eng/openshift-tests-extension/pkg/version"

	"k8s.io/client-go/pkg/version"
	utilflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	"k8s.io/kubernetes/openshift-hack/e2e/annotate/generated"
	"k8s.io/kubernetes/test/utils/image"

	// initialize framework extensions
	_ "k8s.io/kubernetes/test/e2e/framework/debug/init"
	_ "k8s.io/kubernetes/test/e2e/framework/metrics/init"
)

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()
	pflag.CommandLine.SetNormalizeFunc(utilflag.WordSepNormalizeFunc)

	// These flags are used to pull in the default values to test context - required
	// so tests run correctly, even if the underlying flags aren't used.
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)

	// Get version info from kube
	kubeVersion := version.Get()
	v.GitTreeState = kubeVersion.GitTreeState
	v.BuildDate = kubeVersion.BuildDate
	v.CommitFromGit = kubeVersion.GitCommit

	// Create our registry of openshift-tests extensions
	extensionRegistry := e.NewRegistry()
	kubeTestsExtension := e.NewExtension("openshift", "payload", "hyperkube")
	extensionRegistry.Register(kubeTestsExtension)

	// Carve up the kube tests into our openshift suites...
	kubeTestsExtension.AddSuite(e.Suite{
		Name: "kubernetes/conformance/parallel",
		Parents: []string{
			"openshift/conformance/parallel",
			"openshift/conformance/parallel/minimal",
		},
		Qualifiers: []string{`!labels.exists(l, l == "Serial") && labels.exists(l, l == "Conformance")`},
	})

	kubeTestsExtension.AddSuite(e.Suite{
		Name: "kubernetes/conformance/serial",
		Parents: []string{
			"openshift/conformance/serial",
			"openshift/conformance/serial/minimal",
		},
		Qualifiers: []string{`labels.exists(l, l == "Serial") && labels.exists(l, l == "Conformance")`},
	})

	for k, v := range image.GetOriginalImageConfigs() {
		image := convertToImage(v)
		image.Index = int(k)
		kubeTestsExtension.RegisterImage(image)
	}

	//FIXME(stbenjam): what other suites does k8s-test contribute to?

	// Build our specs from ginkgo
	specs, err := g.BuildExtensionTestSpecsFromOpenShiftGinkgoSuite()
	if err != nil {
		panic(err)
	}

	// Initialization for kube ginkgo test framework needs to run before all tests execute
	specs.AddBeforeAll(func() {
		if err := initializeTestFramework(os.Getenv("TEST_PROVIDER")); err != nil {
			panic(err)
		}
	})

	// Annotations get appended to test names, these are additions to upstream
	// tests for controlling skips, suite membership, etc.
	//
	// TODO:
	//		- Remove this annotation code, and migrate to Labels/Tags and
	//		  the environmental skip code from the enhancement once its implemented.
	//		- Make sure to account for test renames that occur because of removal of these
	//		  annotations
	specs.Walk(func(spec *et.ExtensionTestSpec) {
		if annotations, ok := generated.Annotations[spec.Name]; ok {
			spec.Name += annotations
		}
	})

	filterByPlatform(specs)

	// LoadBalancer tests in 1.31 require explicit platform-specific skips
	// https://issues.redhat.com/browse/OCPBUGS-38840
	specs.Select(et.NameContains("[Feature:LoadBalancer]")).
		Exclude(et.Or(
			et.PlatformEquals("alibabacloud"),
			et.PlatformEquals("baremetal"),
			et.PlatformEquals("ibmcloud"),
			et.PlatformEquals("kubevirt"),
			et.PlatformEquals("nutanix"),
			et.PlatformEquals("openstack"),
			et.PlatformEquals("ovirt"),
			et.PlatformEquals("vsphere"),
		))

	// LoadBalancer tests in 1.31 require explicit platform-specific skips
	// https://issues.redhat.com/browse/OCPBUGS-38840
	specs.SelectAny([]et.SelectFunction{ // Since these must use "NameContainsAll" they cannot be included in filterByPlatform
		et.NameContainsAll("[sig-network] LoadBalancers [Feature:LoadBalancer]", "UDP"),
		et.NameContainsAll("[sig-network] LoadBalancers [Feature:LoadBalancer]", "session affinity"),
	}).Exclude(et.PlatformEquals("aws"))

	filterByExternalConnectivity(specs)

	filterByTopology(specs)

	// Tests which can't be run/don't make sense to run against a cluster with all optional capabilities disabled
	specs.SelectAny([]et.SelectFunction{
		// Requires CSISnapshot capability
		et.NameContains("[Feature:VolumeSnapshotDataSource]"),
		// Requires Storage capability
		et.NameContains("[Driver: aws]"),
		et.NameContains("[Feature:StorageProvider]"),
	}).Exclude(et.NoOptionalCapabilitiesExist())

	specs.SelectAll([]et.SelectFunction{
		// ovn-kubernetes does not support named ports
		et.NameContains("NetworkPolicy"),
		et.NameContains("named port"),
	}).Exclude(et.NetworkEquals("OVNKubernetes"))

	kubeTestsExtension.AddSpecs(specs)

	// Cobra stuff
	root := &cobra.Command{
		Long: "Kubernetes tests extension for OpenShift",
	}

	root.AddCommand(
		cmd.DefaultExtensionCommands(extensionRegistry)...,
	)

	if err := func() error {
		return root.Execute()
	}(); err != nil {
		os.Exit(1)
	}
}

// convertToImages converts an image.Config to an extension.Image, which
// can easily be serialized to JSON. Since image.Config has unexported fields,
// reflection is used to read its values.
func convertToImage(obj interface{}) extension.Image {
	image := extension.Image{}
	val := reflect.ValueOf(obj)
	typ := reflect.TypeOf(obj)
	for i := 0; i < val.NumField(); i++ {
		structField := typ.Field(i)
		fieldValue := val.Field(i)
		switch structField.Name {
		case "registry":
			image.Registry = fieldValue.String()
		case "name":
			image.Name = fieldValue.String()
		case "version":
			image.Version = fieldValue.String()
		}
	}
	return image
}

// filterByPlatform is a helper function to do, simple, "NameContains" filtering on tests by platform
func filterByPlatform(specs et.ExtensionTestSpecs) {
	var platformExclusions = map[string][]string{
		"azure": {
			"Networking should provide Internet connection for containers", // Azure does not allow ICMP traffic to internet.
			// Azure CSI migration changed how we treat regions without zones.
			// See https://bugzilla.redhat.com/bugzilla/show_bug.cgi?id=2066865
			"[sig-storage] In-tree Volumes [Driver: azure-disk] [Testpattern: Dynamic PV (immediate binding)] topology should provision a volume and schedule a pod with AllowedTopologies",
			"[sig-storage] In-tree Volumes [Driver: azure-disk] [Testpattern: Dynamic PV (delayed binding)] topology should provision a volume and schedule a pod with AllowedTopologies",
		},
		"gce": {
			// Requires creation of a different compute instance in a different zone and is not compatible with volumeBindingMode of WaitForFirstConsumer which we use in 4.x
			"[sig-storage] Multi-AZ Cluster Volumes should only be allowed to provision PDs in zones where nodes exist",
			// The following tests try to ssh directly to a node. None of our nodes have external IPs
			"[k8s.io] [sig-node] crictl should be able to run crictl on the node",
			"[sig-storage] Flexvolumes should be mountable",
			"[sig-storage] Detaching volumes should not work when mount is in progress",
			// We are using ovn-kubernetes to conceal metadata
			"[sig-auth] Metadata Concealment should run a check-metadata-concealment job to completion",
			// https://bugzilla.redhat.com/show_bug.cgi?id=1740959
			"[sig-api-machinery] AdmissionWebhook should be able to deny pod and configmap creation",
			// https://bugzilla.redhat.com/show_bug.cgi?id=1745720
			"[sig-storage] CSI Volumes [Driver: pd.csi.storage.gke.io]",
			// https://bugzilla.redhat.com/show_bug.cgi?id=1749882
			"[sig-storage] CSI Volumes CSI Topology test using GCE PD driver [Serial]",
			// https://bugzilla.redhat.com/show_bug.cgi?id=1751367
			"gce-localssd-scsi-fs",
			// https://bugzilla.redhat.com/show_bug.cgi?id=1750851
			// should be serial if/when it's re-enabled
			"[HPA] Horizontal pod autoscaling (scale resource: Custom Metrics from Stackdriver)",
			"[Feature:CustomMetricsAutoscaling]",
		},
		"ibmroks": {
			// Calico is allowing the request to timeout instead of returning 'REFUSED'
			// https://bugzilla.redhat.com/show_bug.cgi?id=1825021 - ROKS: calico SDN results in a request timeout when accessing services with no endpoints
			"[sig-network] Services should be rejected when no endpoints exist",
			// Nodes in ROKS have access to secrets in the cluster to handle encryption
			// https://bugzilla.redhat.com/show_bug.cgi?id=1825013 - ROKS: worker nodes have access to secrets in the cluster
			"[sig-auth] [Feature:NodeAuthorizer] Getting a non-existent configmap should exit with the Forbidden error, not a NotFound error",
			"[sig-auth] [Feature:NodeAuthorizer] Getting a non-existent secret should exit with the Forbidden error, not a NotFound error",
			"[sig-auth] [Feature:NodeAuthorizer] Getting a secret for a workload the node has access to should succeed",
			"[sig-auth] [Feature:NodeAuthorizer] Getting an existing configmap should exit with the Forbidden error",
			"[sig-auth] [Feature:NodeAuthorizer] Getting an existing secret should exit with the Forbidden error",
			// Access to node external address is blocked from pods within a ROKS cluster by Calico
			// https://bugzilla.redhat.com/show_bug.cgi?id=1825016 - e2e: NodeAuthenticator tests use both external and internal addresses for node
			"[sig-auth] [Feature:NodeAuthenticator] The kubelet's main port 10250 should reject requests with no credentials",
			"[sig-auth] [Feature:NodeAuthenticator] The kubelet can delegate ServiceAccount tokens to the API server",
			// Mode returned by RHEL7 worker contains an extra character not expected by the test: dgtrwx vs dtrwx
			// https://bugzilla.redhat.com/show_bug.cgi?id=1825024 - e2e: Failing test - HostPath should give a volume the correct mode
			"[sig-storage] HostPath should give a volume the correct mode",
		},
	}

	for platform, exclusions := range platformExclusions {
		var selectFunctions []et.SelectFunction
		for _, exclusion := range exclusions {
			selectFunctions = append(selectFunctions, et.NameContains(exclusion))
		}

		specs.SelectAny(selectFunctions).Exclude(et.PlatformEquals(platform))
	}
}

// filterByExternalConnectivity is a helper function to do, simple, "NameContains" filtering on tests by external connectivity
func filterByExternalConnectivity(specs et.ExtensionTestSpecs) {
	var externalConnectivityExclusions = map[string][]string{
		// Tests that don't pass on disconnected, either due to requiring
		// internet access for GitHub (e.g. many of the s2i builds), or
		// because of pullthrough not supporting ICSP (https://bugzilla.redhat.com/show_bug.cgi?id=1918376)
		"Disconnected": {
			"[sig-network] Networking should provide Internet connection for containers",
		},
		// These tests are skipped when openshift-tests needs to use a proxy to reach the
		// cluster -- either because the test won't work while proxied, or because the test
		// itself is testing a functionality using it's own proxy.
		"Proxy": {
			// These tests setup their own proxy, which won't work when we need to access the
			// cluster through a proxy.
			"[sig-cli] Kubectl client Simple pod should support exec through an HTTP proxy",
			"[sig-cli] Kubectl client Simple pod should support exec through kubectl proxy",
			// Kube currently uses the x/net/websockets pkg, which doesn't work with proxies.
			// See: https://github.com/kubernetes/kubernetes/pull/103595
			"[sig-node] Pods should support retrieving logs from the container over websockets",
			"[sig-cli] Kubectl Port forwarding With a server listening on localhost should support forwarding over websockets",
			"[sig-cli] Kubectl Port forwarding With a server listening on 0.0.0.0 should support forwarding over websockets",
			"[sig-node] Pods should support remote command execution over websockets",
			// These tests are flacky and require internet access
			// See https://bugzilla.redhat.com/show_bug.cgi?id=2019375
			"[sig-network] DNS should resolve DNS of partial qualified names for services",
			"[sig-network] DNS should provide DNS for the cluster",
			// This test does not work when using in-proxy cluster, see https://bugzilla.redhat.com/show_bug.cgi?id=2084560
			"[sig-network] Networking should provide Internet connection for containers",
		},
	}

	for externalConnectivity, exclusions := range externalConnectivityExclusions {
		var selectFunctions []et.SelectFunction
		for _, exclusion := range exclusions {
			selectFunctions = append(selectFunctions, et.NameContains(exclusion))
		}

		specs.SelectAny(selectFunctions).Exclude(et.ExternalConnectivityEquals(externalConnectivity))
	}
}

// filterByTopology is a helper function to do, simple, "NameContains" filtering on tests by topology
func filterByTopology(specs et.ExtensionTestSpecs) {
	var topologyExclusions = map[string][]string{
		"SingleReplicaTopology": {
			"[sig-apps] Daemon set [Serial] should rollback without unnecessary restarts [Conformance]",
			"[sig-node] NoExecuteTaintManager Single Pod [Serial] doesn't evict pod with tolerations from tainted nodes",
			"[sig-node] NoExecuteTaintManager Single Pod [Serial] eventually evict pod with finite tolerations from tainted nodes",
			"[sig-node] NoExecuteTaintManager Single Pod [Serial] evicts pods from tainted nodes",
			"[sig-node] NoExecuteTaintManager Single Pod [Serial] removing taint cancels eviction [Disruptive] [Conformance]",
			"[sig-node] NoExecuteTaintManager Single Pod [Serial] pods evicted from tainted nodes have pod disruption condition",
			"[sig-node] NoExecuteTaintManager Multiple Pods [Serial] evicts pods with minTolerationSeconds [Disruptive] [Conformance]",
			"[sig-node] NoExecuteTaintManager Multiple Pods [Serial] only evicts pods without tolerations from tainted nodes",
			"[sig-cli] Kubectl client Kubectl taint [Serial] should remove all the taints with the same key off a node",
			"[sig-network] LoadBalancers should be able to preserve UDP traffic when server pod cycles for a LoadBalancer service on different nodes",
			"[sig-network] LoadBalancers should be able to preserve UDP traffic when server pod cycles for a LoadBalancer service on the same nodes",
			"[sig-architecture] Conformance Tests should have at least two untainted nodes",
		},
	}

	for topology, exclusions := range topologyExclusions {
		var selectFunctions []et.SelectFunction
		for _, exclusion := range exclusions {
			selectFunctions = append(selectFunctions, et.NameContains(exclusion))
		}

		specs.SelectAny(selectFunctions).Exclude(et.TopologyEquals(topology))
	}
}
