package features

import (
	"k8s.io/apimachinery/pkg/util/version"
	"k8s.io/component-base/featuregate"
)

var RouteExternalCertificate featuregate.Feature = "RouteExternalCertificate"
var MinimumKubeletVersion featuregate.Feature = "MinimumKubeletVersion"
var StoragePerformantSecurityPolicy featuregate.Feature = "StoragePerformantSecurityPolicy"

// DRA dependent features - disabled because DRA is disabled in OpenShift by default
var DRAAdminAccessOpenShift featuregate.Feature = "DRAAdminAccess"
var DRAPrioritizedListOpenShift featuregate.Feature = "DRAPrioritizedList"
var DRASchedulerFilterTimeoutOpenShift featuregate.Feature = "DRASchedulerFilterTimeout"

// registerOpenshiftFeatures injects openshift-specific feature gates
func registerOpenshiftFeatures() {
	// Introduced in 4.16
	defaultVersionedKubernetesFeatureGates[RouteExternalCertificate] = featuregate.VersionedSpecs{
		{Version: version.MustParse("1.29"), Default: false, PreRelease: featuregate.Alpha},
	}
	// Introduced in 4.19
	defaultVersionedKubernetesFeatureGates[MinimumKubeletVersion] = featuregate.VersionedSpecs{
		{Version: version.MustParse("1.32"), Default: false, PreRelease: featuregate.Alpha},
	}
	// Introduced in 4.20
	defaultVersionedKubernetesFeatureGates[StoragePerformantSecurityPolicy] = featuregate.VersionedSpecs{
		{Version: version.MustParse("1.33"), Default: false, PreRelease: featuregate.Alpha},
	}

	// Override DRA dependent features to be disabled by default since DRA is disabled in OpenShift
	// These features are Beta in upstream K8s v1.34+ but depend on DynamicResourceAllocation
	// Introduced in K8s 1.34, disabled in OpenShift 4.21
	defaultVersionedKubernetesFeatureGates[DRAAdminAccessOpenShift] = featuregate.VersionedSpecs{
		{Version: version.MustParse("1.32"), Default: false, PreRelease: featuregate.Beta},
		{Version: version.MustParse("1.34"), Default: false, PreRelease: featuregate.Beta},
	}
	defaultVersionedKubernetesFeatureGates[DRAPrioritizedListOpenShift] = featuregate.VersionedSpecs{
		{Version: version.MustParse("1.33"), Default: false, PreRelease: featuregate.Beta},
		{Version: version.MustParse("1.34"), Default: false, PreRelease: featuregate.Beta},
	}
	defaultVersionedKubernetesFeatureGates[DRASchedulerFilterTimeoutOpenShift] = featuregate.VersionedSpecs{
		{Version: version.MustParse("1.34"), Default: false, PreRelease: featuregate.Beta},
	}
}
