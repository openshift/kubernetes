package features

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	feature "k8s.io/apiserver/pkg/util/feature"

	configv1 "github.com/openshift/api/config/v1"
	openshiftfeatures "github.com/openshift/library-go/pkg/features"
)

// OpenShiftFeatureGates contains list of feature gates that will
// be collectively honored by each of openshift's hyperkube binaries
var OpenshiftFeatureGates = []configv1.FeatureGateName{
	configv1.FeatureGateRouteExternalCertificate,
}

func init() {
	runtime.Must(registerOpenshiftFeatures())
}

func registerOpenshiftFeatures() error {
	return openshiftfeatures.InitializeFeatureGates(
		feature.DefaultMutableFeatureGate,
		OpenshiftFeatureGates...,
	)
}
