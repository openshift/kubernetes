package ingress

import (
	"fmt"
	"io"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/route/routeapihelpers"
	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/kubernetes/openshift-kube-apiserver/admission/customresourcevalidation"
)

const PluginName = "config.openshift.io/ValidateIngress"

// Register registers a plugin
func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
		return customresourcevalidation.NewValidator(
			map[schema.GroupResource]bool{
				configv1.Resource("ingresses"): true,
			},
			map[schema.GroupVersionKind]customresourcevalidation.ObjectValidator{
				configv1.GroupVersion.WithKind("Ingress"): ingressV1{},
			})
	})
}

func toIngressV1(uncastObj runtime.Object) (*configv1.Ingress, field.ErrorList) {
	if uncastObj == nil {
		return nil, nil
	}

	allErrs := field.ErrorList{}

	obj, ok := uncastObj.(*configv1.Ingress)
	if !ok {
		return nil, append(allErrs,
			field.NotSupported(field.NewPath("kind"), fmt.Sprintf("%T", uncastObj), []string{"Ingress"}),
			field.NotSupported(field.NewPath("apiVersion"), fmt.Sprintf("%T", uncastObj), []string{"config.openshift.io/v1"}))
	}

	return obj, nil
}

type ingressV1 struct {
}

func validateIngressSpecCreate(spec configv1.IngressSpec) field.ErrorList {
	allErrs := field.ErrorList{}

	// on create, we use the tightest validation
	for i, currRoute := range spec.ComponentRoutes {
		allErrs = append(allErrs,
			routeapihelpers.ValidateHost(
				string(currRoute.Hostname),
				"",
				field.NewPath("spec.componentRoutes").Index(i).Child("hostname"),
			)...)
	}

	return allErrs
}

func validateIngressSpecUpdate(spec, oldSpec configv1.IngressSpec) field.ErrorList {
	allErrs := field.ErrorList{}

	// on update, if the componentroute.hostname has changed, we use the tightest validation.
	// if the componentroute.hostname has not changed, we do not enforce new validation.
	// empty hostnames always produce an error.  See unit test.
	for i, currRoute := range spec.ComponentRoutes {
		currFieldPath := field.NewPath("spec.componentRoutes").Index(i).Child("hostname")
		currRouteHostnameErrors := routeapihelpers.ValidateHost(
			string(currRoute.Hostname),
			"",
			currFieldPath,
		)
		if len(currRouteHostnameErrors) == 0 {
			continue
		}
		if len(currRoute.Hostname) == 0 {
			allErrs = append(allErrs, currRouteHostnameErrors...)
			continue
		}

		previousRouteHostName := configv1.Hostname("")
		for _, oldRoute := range oldSpec.ComponentRoutes {
			if oldRoute.Name == currRoute.Name && oldRoute.Namespace == currRoute.Namespace {
				previousRouteHostName = oldRoute.Hostname
				break
			}
		}
		// we don't enforce new validation rules if the hostname has not changed
		if previousRouteHostName == currRoute.Hostname {
			continue
		}

		// if the hostname has changed, then the new route must pass new validation.
		allErrs = append(allErrs, currRouteHostnameErrors...)
	}

	return allErrs
}

func (ingressV1) ValidateCreate(uncastObj runtime.Object) field.ErrorList {
	obj, allErrs := toIngressV1(uncastObj)
	if len(allErrs) > 0 {
		return allErrs
	}

	allErrs = append(allErrs, validation.ValidateObjectMeta(&obj.ObjectMeta, false, customresourcevalidation.RequireNameCluster, field.NewPath("metadata"))...)
	allErrs = append(allErrs, validateIngressSpecCreate(obj.Spec)...)

	return allErrs
}

func (ingressV1) ValidateUpdate(uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	obj, allErrs := toIngressV1(uncastObj)
	if len(allErrs) > 0 {
		return allErrs
	}
	oldObj, allErrs := toIngressV1(uncastOldObj)
	if len(allErrs) > 0 {
		return allErrs
	}

	allErrs = append(allErrs, validation.ValidateObjectMetaUpdate(&obj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))...)
	allErrs = append(allErrs, validateIngressSpecUpdate(obj.Spec, oldObj.Spec)...)

	return allErrs
}

func (ingressV1) ValidateStatusUpdate(uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	obj, errs := toIngressV1(uncastObj)
	if len(errs) > 0 {
		return errs
	}
	oldObj, errs := toIngressV1(uncastOldObj)
	if len(errs) > 0 {
		return errs
	}

	// TODO validate the obj.  remember that status validation should *never* fail on spec validation errors.
	errs = append(errs, validation.ValidateObjectMetaUpdate(&obj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))...)

	return errs
}
