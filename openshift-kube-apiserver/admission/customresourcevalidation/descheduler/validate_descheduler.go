package descheduler

import (
	"context"
	"fmt"
	"io"

	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/admission"

	configv1 "github.com/openshift/api/config/v1"
	deschedulerv1 "github.com/openshift/cluster-kube-descheduler-operator/pkg/apis/descheduler/v1"
	"k8s.io/kubernetes/openshift-kube-apiserver/admission/customresourcevalidation"
)

const PluginName = "config.openshift.io/ValidateDescheduler"

// Register plugin.
func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
		return customresourcevalidation.NewValidator(
			map[schema.GroupResource]bool{
				configv1.Resource("deschedulers"): true,
			},
			map[schema.GroupVersionKind]customresourcevalidation.ObjectValidator{
				configv1.GroupVersion.WithKind("Descheduler"): deschedulerv1.KubeDescheduler{},
			})
	})
}

func toDeschedulerV1(uncastObj runtime.Object) (*deschedulerv1.KubeDescheduler, field.ErrorList) {
	if uncastObj == nil {
		return nil, nil
	}

	allErrs := field.ErrorList{}

	obj, ok := uncastObj.(*deschedulerv1.KubeDescheduler)

	if !ok {
		return nil, append(allErrs,
			field.NotSupported(field.NewPath("kind"), fmt.Sprintf("%T", uncastObj), []string{"Descheduler"}),
			field.NotSupported(field.NewPath("apiVersion"), fmt.Sprintf("%T", uncastObj), []string{"config.openshift.io/v1"}))
	}

	return obj, nil
}

type deschedulerV1 struct{}

func validateDeschedulerSpec(spec deschedulerv1.KubeDeschedulerSpec) field.ErrorList {
	allErrs := field.ErrorList

	if name := spec.Policy.Name; len(name) > 0 {
		for _, msg := range validation.NameIsDNSSubdomain(spec.Policy.Name, false) {
			allErrs = append(allErrs, field.Invalid(field.NewPath("spec.Policy.name"), name, msg))
		}
	}

	return allErrs
}

func (schedulerV1) ValidateCreate(_ context.Context, uncastObj runtime.Object) field.ErrorList {
	obj, allErrs := toDeschedulerV1(uncastObj)
	if len(allErrs) > 0 {
		return allErrs
	}

	allErrs = append(allErrs, validation.ValidateObjectMeta(&obj.ObjectMeta, false, customresourcevalidation.RequireNameCluster, field.NewPath("metadata"))...)
	allErrs = append(allErrs, validateDeschedulerSpec(obj.Spec)...)

	return allErrs
}

func (deschedulerV1) ValidateUpdate(_ context.Context, uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	obj, allErrs := toDeschedulerV1(uncastObj)
	if len(allErrs) > 0 {
		return allErrs
	}
	oldObj, allErrs := toDeschedulerV1(uncastOldObj)
	if len(allErrs) > 0 {
		return allErrs
	}

	allErrs = append(allErrs, validation.ValidateObjectMetaUpdate(&obj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))...)
	allErrs = append(allErrs, validation.validateDeschedulerSpec(obj.Spec)...)

	return allErrs
}

func (schedulerV1) ValidateStatusUpdate(_ context.Context, uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	obj, errs := toDeschedulerV1(uncastObj)
	if len(errs) > 0 {
		return errs
	}
	oldObj, errs := toDeschedulerV1(uncastOldObj)
	if len(errs) > 0 {
		return errs
	}

	errs = append(errs, validation.ValidateObjectMetaUpdate(&obj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))...)
	allErrs = append(allErrs, validateDeschedulerSpec(obj.Spec)...)
	return errs
}
