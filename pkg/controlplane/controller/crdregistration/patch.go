package crdregistration

func getGroupPriorityMin(group string) int32 {
	switch group {
	case "machine.openshift.io":
		return 1100
	case "config.openshift.io":
		return 1100
	case "operator.openshift.io":
		return 1080
	default:
		return 1000
	}
}
