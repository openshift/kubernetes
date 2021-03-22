package csimigration

import (
	"k8s.io/component-base/featuregate"
	csilibplugins "k8s.io/csi-translation-lib/plugins"
	"k8s.io/kubernetes/pkg/features"
)

// NewKCMPluginManager returns a new PluginManager instance for the KCM which uses different featuregates in openshift
// to control enablement/disablement which *DO NOT MATCH* the featuregates for the rest of the cluster.
func NewKCMPluginManager(m PluginNameMapper, featureGate featuregate.FeatureGate) PluginManager {
	ret := NewPluginManager(m, featureGate)
	ret.useKCMPluginManagerFeatureGates = true
}

// IsMigrationCompleteForPlugin indicates whether CSI migration has been completed
// for a particular storage plugin. A complete migration will need to:
// 1. Enable CSIMigrationXX for the plugin
// 2. Unregister the in-tree plugin by setting the InTreePluginXXUnregister feature gate
func (pm PluginManager) kcmIsMigrationCompleteForPlugin(pluginName string) bool {
	// CSIMigration feature and plugin specific InTreePluginUnregister feature flags should
	// be enabled for plugin specific migration completion to be take effect
	if !pm.IsMigrationEnabledForPlugin(pluginName) {
		return false
	}

	// only list your special cases here
	switch pluginName {
	case csilibplugins.AWSEBSInTreePluginName:
		return pm.featureGate.Enabled(features.KCMInTreePluginAWSUnregister)
	default:
		// default to the normal path
		return pm.IsMigrationCompleteForPlugin(pluginName)
	}
}

// IsMigrationEnabledForPlugin indicates whether CSI migration has been enabled
// for a particular storage plugin
func (pm PluginManager) kcmIsMigrationEnabledForPlugin(pluginName string) bool {
	// CSIMigration feature should be enabled along with the plugin-specific one
	if !pm.featureGate.Enabled(features.CSIMigration) {
		return false
	}

	// only list your special cases here
	switch pluginName {
	case csilibplugins.AWSEBSInTreePluginName:
		return pm.featureGate.Enabled(features.KCMCSIMigrationAWS)
	default:
		// default to the normal path
		return pm.IsMigrationEnabledForPlugin(pluginName)
	}
}
