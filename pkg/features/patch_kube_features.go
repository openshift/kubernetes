package features

import "k8s.io/component-base/featuregate"

var (
	// owner: @jsafrane
	// alpha: v1.21
	//
	// Disables the AWS EBS in-tree driver for the KCM only.
	KCMInTreePluginAWSUnregister featuregate.Feature = "KCM_InTreePluginAWSUnregister"

	// owner: @jsafrane
	// alpha: v1.21
	//
	// Disables the GCE PD in-tree driver for the KCM only.
	KCMInTreePluginGCEUnregister featuregate.Feature = "KCM_InTreePluginGCEUnregister"
)

func init() {
	defaultKubernetesFeatureGates[KCMInTreePluginAWSUnregister] = featuregate.FeatureSpec{Default: false, PreRelease: featuregate.Alpha}
	defaultKubernetesFeatureGates[KCMInTreePluginGCEUnregister] = featuregate.FeatureSpec{Default: false, PreRelease: featuregate.Alpha}
}
