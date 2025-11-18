package features

import (
	"os"

	"k8s.io/apimachinery/pkg/util/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
)

// This override should only be used with a support exception. It provides a way
// to turn off DisableCPUQuotaWithExclusiveCPUs feature gate in a way that still
// allows the cluster to be upgradable. This is only needed on 4.20, as it's meant as a stop-gap
// for customers to go through a lengthy process to update their workloads to handle this change.
// Owners: @haircommander
func OpenShiftEnableCPUQuotaWithExclusiveCPUsOverride() {
	overrideVariable := "OCP_4_20_SUPPORT_EXCEPTION_ENABLE_CPU_QUOTA_WITH_EXCLUSIVE_CPUS"
	_, enabled := os.LookupEnv(overrideVariable)
	if enabled {
		klog.Infof("Environment variable %s is set, setting feature gate %s to false", overrideVariable, string(DisableCPUQuotaWithExclusiveCPUs))
		fg := map[string]bool{string(DisableCPUQuotaWithExclusiveCPUs): false}
		runtime.Must(utilfeature.DefaultMutableFeatureGate.SetFromMap(fg))
	}
}

func init() {
	OpenShiftEnableCPUQuotaWithExclusiveCPUsOverride()
}
