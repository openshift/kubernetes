package imagecontentsourcepolicy

import (
	"fmt"
	"io"

	v1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/kubernetes/openshift-kube-apiserver/admission/customresourcevalidation"
)

const PluginName = "oprator.openshift.io/ValidateImageContentSourcePolicy"

// Register registers a plugin
func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
		return customresourcevalidation.NewValidator(
			map[schema.GroupResource]bool{
				v1alpha1.Resource("imagecontentsourcepolicies"): true,
			},
			map[schema.GroupVersionKind]customresourcevalidation.ObjectValidator{
				v1alpha1.GroupVersion.WithKind("ImageContentSourcePolicy"): imagecontentsourcepolicyV1alpha1{},
			})
	})
}

type imagecontentsourcepolicyV1alpha1 struct {
}

func (imagecontentsourcepolicyV1alpha1) ValidateCreate(uncastObj runtime.Object) field.ErrorList {
	errs := field.ErrorList{
		field.Invalid(field.NewPath("Kind", "ImageContentSourcePolicy"), "", fmt.Sprintln("ImageContentSourcePolicy is deprecated, please use ImageDigestMirrorSet"))}
	return errs
}

func (imagecontentsourcepolicyV1alpha1) ValidateUpdate(uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	errs := field.ErrorList{
		field.Invalid(field.NewPath("Kind", "ImageContentSourcePolicy"), "", fmt.Sprintln("ImageContentSourcePolicy is deprecated, please use ImageDigestMirrorSet"))}
	return errs
}

func (imagecontentsourcepolicyV1alpha1) ValidateStatusUpdate(uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	errs := field.ErrorList{
		field.Invalid(field.NewPath("Kind", "ImageContentSourcePolicy"), "", fmt.Sprintln("ImageContentSourcePolicy is deprecated, please use ImageDigestMirrorSet"))}
	return errs
}
