package imagecontentsourcepolicy

import (
	"fmt"
	"io"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/config/helpers"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/klog/v2"
)

const PluginName = "oprator.openshift.io/ValidateImageContentSourcePolicy"
const ConvertPluginName = "oprator.openshift.io/ConvertImageContentSourcePolicy"

// Register registers a plugin
func Register(plugins *admission.Plugins) {
	// plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
	// 	return customresourcevalidation.NewValidator(
	// 		map[schema.GroupResource]bool{
	// 			v1alpha1.Resource("imagecontentsourcepolicies"): true,
	// 		},
	// 		map[schema.GroupVersionKind]customresourcevalidation.ObjectValidator{
	// 			v1alpha1.GroupVersion.WithKind("ImageContentSourcePolicy"): imagecontentsourcepolicyV1alpha1{},
	// 		})
	// })
	plugins.Register(ConvertPluginName, func(config io.Reader) (admission.Interface, error) {
		pluginConfig, err := readConfig(config)
		if err != nil {
			return nil, err
		}
		if pluginConfig == nil {
			klog.Infof("Admission plugin %q is not configured so it will be disabled.", "oprator.openshift.io/ConvertImageContentSourcePolicy")
			return nil, nil
		}

		return NewImageDigestMirrorSet(pluginConfig), nil
	})
}

type imagecontentsourcepolicyV1alpha1 struct {
	*admission.Handler
	config *configv1.ImageDigestMirrorSet
}

// func (imagecontentsourcepolicyV1alpha1) ValidateCreate(uncastObj runtime.Object) field.ErrorList {
// 	errs := field.ErrorList{
// 		field.Invalid(field.NewPath("Kind", "ImageContentSourcePolicy"), "", fmt.Sprintln("ImageContentSourcePolicy is deprecated, please use ImageDigestMirrorSet"))}
// 	return errs
// }

// func (imagecontentsourcepolicyV1alpha1) ValidateUpdate(uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
// 	errs := field.ErrorList{
// 		field.Invalid(field.NewPath("Kind", "ImageContentSourcePolicy"), "", fmt.Sprintln("ImageContentSourcePolicy is deprecated, please use ImageDigestMirrorSet"))}
// 	return errs
// }

// func (imagecontentsourcepolicyV1alpha1) ValidateStatusUpdate(uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
// 	errs := field.ErrorList{
// 		field.Invalid(field.NewPath("Kind", "ImageContentSourcePolicy"), "", fmt.Sprintln("ImageContentSourcePolicy is deprecated, please use ImageDigestMirrorSet"))}
// 	return errs
// }

var (
	localSchemeBuilder = runtime.NewSchemeBuilder(
		// addKnownTypes,
		// runonceduration.Install,

		addConversionFuncs,
	)
	Install = localSchemeBuilder.AddToScheme
)

func readConfig(reader io.Reader) (*configv1.ImageDigestMirrorSet, error) {
	obj, err := helpers.ReadYAMLToInternal(reader, Install)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, nil
	}
	config, ok := obj.(*configv1.ImageDigestMirrorSet)
	if !ok {
		return nil, fmt.Errorf("unexpected config object %#v", obj)
	}
	return config, nil
}

// func addConversionFuncs(scheme *runtime.Scheme) error {
// 	err := scheme.AddConversionFunc((*v1alpha1.ImageContentSourcePolicy)(nil), (*configv1.ImageDigestMirrorSet)(nil), func(a, b interface{}, scope conversion.Scope) error {
// 		in := a.(*v1alpha1.ImageContentSourcePolicy)
// 		out := b.(*configv1.ImageDigestMirrorSet)
// 		digestMirrors := []configv1.ImageDigestMirrors{}
// 		for _, repoMirrors := range in.Spec.RepositoryDigestMirrors {
// 			// digestMirror
// 			mirrors := []configv1.ImageMirror{}
// 			for _, m := range repoMirrors.Mirrors {
// 				mirrors = append(mirrors, configv1.ImageMirror(m))
// 			}
// 			digestMirror := configv1.ImageDigestMirrors{
// 				Source:  repoMirrors.Source,
// 				Mirrors: mirrors,
// 			}
// 			digestMirrors = append(digestMirrors, digestMirror)
// 		}
// 		out.Spec.ImageDigestMirrors = digestMirrors
// 		return nil
// 	})
// 	return err
// }

// NewImageDigestMirrorSet creates a new ImageDigestMirrorSet admission plugin
func NewImageDigestMirrorSet(config *configv1.ImageDigestMirrorSet) admission.Interface {
	return &imagecontentsourcepolicyV1alpha1{
		Handler: admission.NewHandler(admission.Create),
		config:  config,
	}
}
