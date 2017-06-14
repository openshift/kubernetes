/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validation

import (
	"fmt"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/api"
)

// ValidateSecurityContextConstraintsName can be used to check whether the given
// security context constraint name is valid.
// Prefix indicates this name will be used as part of generation, in which case
// trailing dashes are allowed.
var ValidateSecurityContextConstraintsName = NameIsDNSSubdomain

func ValidateSecurityContextConstraints(scc *api.SecurityContextConstraints) field.ErrorList {
	allErrs := ValidateObjectMeta(&scc.ObjectMeta, false, ValidateSecurityContextConstraintsName, field.NewPath("metadata"))

	if scc.Priority != nil {
		if *scc.Priority < 0 {
			allErrs = append(allErrs, field.Invalid(field.NewPath("priority"), *scc.Priority, "priority cannot be negative"))
		}
	}

	// ensure the user strat has a valid type
	runAsUserPath := field.NewPath("runAsUser")
	switch scc.RunAsUser.Type {
	case api.RunAsUserStrategyMustRunAs, api.RunAsUserStrategyMustRunAsNonRoot, api.RunAsUserStrategyRunAsAny, api.RunAsUserStrategyMustRunAsRange:
		//good types
	default:
		msg := fmt.Sprintf("invalid strategy type.  Valid values are %s, %s, %s", api.RunAsUserStrategyMustRunAs, api.RunAsUserStrategyMustRunAsNonRoot, api.RunAsUserStrategyRunAsAny)
		allErrs = append(allErrs, field.Invalid(runAsUserPath.Child("type"), scc.RunAsUser.Type, msg))
	}

	// if specified, uid cannot be negative
	if scc.RunAsUser.UID != nil {
		if *scc.RunAsUser.UID < 0 {
			allErrs = append(allErrs, field.Invalid(runAsUserPath.Child("uid"), *scc.RunAsUser.UID, "uid cannot be negative"))
		}
	}

	// ensure the selinux strat has a valid type
	seLinuxContextPath := field.NewPath("seLinuxContext")
	switch scc.SELinuxContext.Type {
	case api.SELinuxStrategyMustRunAs, api.SELinuxStrategyRunAsAny:
		//good types
	default:
		msg := fmt.Sprintf("invalid strategy type.  Valid values are %s, %s", api.SELinuxStrategyMustRunAs, api.SELinuxStrategyRunAsAny)
		allErrs = append(allErrs, field.Invalid(seLinuxContextPath.Child("type"), scc.SELinuxContext.Type, msg))
	}

	// ensure the fsgroup strat has a valid type
	if scc.FSGroup.Type != api.FSGroupStrategyMustRunAs && scc.FSGroup.Type != api.FSGroupStrategyRunAsAny {
		allErrs = append(allErrs, field.NotSupported(field.NewPath("fsGroup", "type"), scc.FSGroup.Type,
			[]string{string(api.FSGroupStrategyMustRunAs), string(api.FSGroupStrategyRunAsAny)}))
	}
	allErrs = append(allErrs, validateIDRanges(scc.FSGroup.Ranges, field.NewPath("fsGroup"))...)

	if scc.SupplementalGroups.Type != api.SupplementalGroupsStrategyMustRunAs &&
		scc.SupplementalGroups.Type != api.SupplementalGroupsStrategyRunAsAny {
		allErrs = append(allErrs, field.NotSupported(field.NewPath("supplementalGroups", "type"), scc.SupplementalGroups.Type,
			[]string{string(api.SupplementalGroupsStrategyMustRunAs), string(api.SupplementalGroupsStrategyRunAsAny)}))
	}
	allErrs = append(allErrs, validateIDRanges(scc.SupplementalGroups.Ranges, field.NewPath("supplementalGroups"))...)

	// validate capabilities
	allErrs = append(allErrs, validateSCCCapsAgainstDrops(scc.RequiredDropCapabilities, scc.DefaultAddCapabilities, field.NewPath("defaultAddCapabilities"))...)
	allErrs = append(allErrs, validateSCCCapsAgainstDrops(scc.RequiredDropCapabilities, scc.AllowedCapabilities, field.NewPath("allowedCapabilities"))...)

	if hasCap(api.CapabilityAll, scc.AllowedCapabilities) && len(scc.RequiredDropCapabilities) > 0 {
		allErrs = append(allErrs, field.Invalid(field.NewPath("requiredDropCapabilities"), scc.RequiredDropCapabilities,
			"required capabilities must be empty when all capabilities are allowed by a wildcard"))
	}

	return allErrs
}

// validateSCCCapsAgainstDrops ensures an allowed cap is not listed in the required drops.
func validateSCCCapsAgainstDrops(requiredDrops []api.Capability, capsToCheck []api.Capability, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if requiredDrops == nil {
		return allErrs
	}
	for _, cap := range capsToCheck {
		if hasCap(cap, requiredDrops) {
			allErrs = append(allErrs, field.Invalid(fldPath, cap,
				fmt.Sprintf("capability is listed in %s and requiredDropCapabilities", fldPath.String())))
		}
	}
	return allErrs
}

// hasCap checks for needle in haystack.
func hasCap(needle api.Capability, haystack []api.Capability) bool {
	for _, c := range haystack {
		if needle == c {
			return true
		}
	}
	return false
}

// validateIDRanges ensures the range is valid.
func validateIDRanges(rng []api.IDRange, fldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	for i, r := range rng {
		// if 0 <= Min <= Max then we do not need to validate max.  It is always greater than or
		// equal to 0 and Min.
		minPath := fldPath.Child("ranges").Index(i).Child("min")
		maxPath := fldPath.Child("ranges").Index(i).Child("max")

		if r.Min < 0 {
			allErrs = append(allErrs, field.Invalid(minPath, r.Min, "min cannot be negative"))
		}
		if r.Max < 0 {
			allErrs = append(allErrs, field.Invalid(maxPath, r.Max, "max cannot be negative"))
		}
		if r.Min > r.Max {
			allErrs = append(allErrs, field.Invalid(minPath, r, "min cannot be greater than max"))
		}
	}

	return allErrs
}

func ValidateSecurityContextConstraintsUpdate(newScc, oldScc *api.SecurityContextConstraints) field.ErrorList {
	allErrs := ValidateObjectMetaUpdate(&newScc.ObjectMeta, &oldScc.ObjectMeta, field.NewPath("metadata"))
	allErrs = append(allErrs, ValidateSecurityContextConstraints(newScc)...)
	return allErrs
}
