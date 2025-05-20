package main

import (
	"strings"

	et "github.com/openshift-eng/openshift-tests-extension/pkg/extension/extensiontests"
)

var (
	techPreviewFeatureGates = []string{
		"VolumeAttributesClass", // Kubernetes beta, disabled by default
	}
)

// Explicitly copy [FeatureGate:xyz] to [OCPFeatureGate:xyz] for feature gates
// that are enabled in TechPreview jobs and disabled otherwise.
// openshift-tests already recognizes [OCPFeatureGate:xyz] and will run the test only
// if the feature gate is enabled in OCP API.
func copyTechPreviewFeatureGates(specs et.ExtensionTestSpecs) {
	specs.Walk(func(spec *et.ExtensionTestSpec) {
		for _, featureGate := range techPreviewFeatureGates {
			upstreamAnnotation := "[FeatureGate:" + featureGate + "]"
			if strings.Contains(spec.Name, upstreamAnnotation) {
				spec.Name += " [OCPFeatureGate:" + featureGate + "]"
			}
		}
	})
}
