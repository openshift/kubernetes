package infrastructure

import (
	"fmt"
	"io"
	"regexp"

	apiv1 "github.com/openshift/api/config/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/kubernetes/openshift-kube-apiserver/admission/customresourcevalidation"
	"k8s.io/kubernetes/pkg/apis/core/validation"
)

const (
	PluginName = "config.openshift.io/ValidateInfrastructure"
)

type infrastructureV1 struct{}

var reservedRegexes = []*regexp.Regexp{
	//should not contain 'kubernetes.io'
	regexp.MustCompile(`kubernetes\.io`),
	//should not contain 'openshift.io'
	regexp.MustCompile(`openshift\.io`),
	//should not start with 'aws:'
	regexp.MustCompile(`^aws:`),
}

// Register registers a plugin
func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
		return customresourcevalidation.NewValidator(
			map[schema.GroupResource]bool{
				apiv1.GroupVersion.WithResource("infrastructures").GroupResource(): true,
			},
			map[schema.GroupVersionKind]customresourcevalidation.ObjectValidator{
				apiv1.GroupVersion.WithKind("Infrastructure"): infrastructureV1{},
			})
	})
}

func toInfrastructureV1(uncastObj runtime.Object) (*apiv1.Infrastructure, field.ErrorList) {
	if uncastObj == nil {
		return nil, nil
	}

	obj, ok := uncastObj.(*apiv1.Infrastructure)
	if !ok {
		return nil, field.ErrorList{
			field.NotSupported(field.NewPath("kind"), fmt.Sprintf("%T", uncastObj), []string{"APIRequestCount"}),
			field.NotSupported(field.NewPath("apiVersion"), fmt.Sprintf("%T", uncastObj), []string{"apiserver.openshift.io/v1"}),
		}
	}

	return obj, nil
}

func (infrastructureV1) ValidateCreate(uncastObj runtime.Object) field.ErrorList {
	obj, allErrs := toInfrastructureV1(uncastObj)
	if len(allErrs) > 0 {
		return allErrs
	}

	allErrs = append(allErrs, validation.ValidateObjectMeta(&obj.ObjectMeta, false, customresourcevalidation.RequireNameCluster, field.NewPath("metadata"))...)
	if errs := validateAWSResourceTagKeysIfAny(obj); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}
	if errs := validateUniqueTagKeys(obj); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}
	return allErrs
}

func (infrastructureV1) ValidateStatusUpdate(uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	obj, errs := toInfrastructureV1(uncastObj)
	if len(errs) > 0 {
		return errs
	}
	oldObj, errs := toInfrastructureV1(uncastOldObj)
	if len(errs) > 0 {
		return errs
	}

	errs = append(errs, validation.ValidateObjectMetaUpdate(&obj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))...)

	return errs
}

func (infrastructureV1) ValidateUpdate(uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	obj, errs := toInfrastructureV1(uncastObj)
	if len(errs) > 0 {
		return errs
	}
	oldObj, errs := toInfrastructureV1(uncastOldObj)
	if len(errs) > 0 {
		return errs
	}

	errs = append(errs, validation.ValidateObjectMetaUpdate(&obj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))...)
	if tagKeyErrs := validateAWSResourceTagKeysIfAny(obj); len(tagKeyErrs) > 0 {
		errs = append(errs, tagKeyErrs...)
	}
	if duplicateErrs := validateUniqueTagKeys(obj); len(duplicateErrs) > 0 {
		errs = append(errs, duplicateErrs...)
	}
	return errs
}

func validateAWSResourceTagKeysIfAny(infra *apiv1.Infrastructure) field.ErrorList {
	errs := field.ErrorList{}
	if len(infra.Spec.PlatformSpec.AWS.ResourceTags) > 0 {
		for i, tag := range infra.Spec.PlatformSpec.AWS.ResourceTags {
			for _, reservedRex := range reservedRegexes {
				if err := validateTagKey(reservedRex, i, tag.Key); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errs
}

func validateUniqueTagKeys(infra *apiv1.Infrastructure) field.ErrorList {
	errs := field.ErrorList{}
	if len(infra.Spec.PlatformSpec.AWS.ResourceTags) > 0 {
		unique := sets.NewString()
		for index, tag := range infra.Spec.PlatformSpec.AWS.ResourceTags {
			if unique.Has(tag.Key) {
				// second occurrence - is not unique
				err := field.Duplicate(
					field.NewPath("spec", getResourceTagsPath(index)...),
					tag.Key)
				errs = append(errs, err)
				continue
			}
			unique.Insert(tag.Key)
		}
	}
	return errs
}

func getResourceTagsPath(index int) []string {
	pathSlice := []string{"platformSpec", "AWS", "resourceTags"}
	item := fmt.Sprintf("[%d]", index)
	pathSlice = append(pathSlice, item)
	return pathSlice
}

func validateTagKey(invalidExpr *regexp.Regexp, index int, key string) *field.Error {
	if invalidExpr.MatchString(key) {
		return field.Invalid(
			field.NewPath("spec", getResourceTagsPath(index)...),
			key,
			fmt.Sprintf("%s should not match regex %v", key, invalidExpr))
	}
	return nil
}
