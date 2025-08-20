package version

var (
	// SCOS is a setting to enable CentOS Stream CoreOS-only modifications
	SCOS = false
)

// IsSCOS returns true if CentOS Stream CoreOS-only modifications are enabled
func IsSCOS() bool {
	return SCOS
}