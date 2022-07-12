package imagecontentsourcepolicy

import (
	configv1 "github.com/openshift/api/config/v1"
	v1alpha1 "github.com/openshift/api/operator/v1alpha1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
)

func addConversionFuncs(scheme *runtime.Scheme) error {
	err := scheme.AddConversionFunc((*v1alpha1.ImageContentSourcePolicy)(nil), (*configv1.ImageDigestMirrorSet)(nil), func(a, b interface{}, scope conversion.Scope) error {
		in := a.(*v1alpha1.ImageContentSourcePolicy)
		out := b.(*configv1.ImageDigestMirrorSet)
		digestMirrors := []configv1.ImageDigestMirrors{}
		for _, repoMirrors := range in.Spec.RepositoryDigestMirrors {
			// digestMirror
			mirrors := []configv1.ImageMirror{}
			for _, m := range repoMirrors.Mirrors {
				mirrors = append(mirrors, configv1.ImageMirror(m))
			}
			digestMirror := configv1.ImageDigestMirrors{
				Source:  repoMirrors.Source,
				Mirrors: mirrors,
			}
			digestMirrors = append(digestMirrors, digestMirror)
		}
		out.Spec.ImageDigestMirrors = digestMirrors
		return nil
	})
	return err
}
