package route

import (
	"fmt"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/openshift/library-go/pkg/apiserver/admission/admissionrestconfig"
	routecommon "github.com/openshift/library-go/pkg/route"
	authorizationv1client "k8s.io/client-go/kubernetes/typed/authorization/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/component-base/featuregate"

	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/rest"
	"k8s.io/kubernetes/openshift-kube-apiserver/admission/customresourcevalidation"
)

type validateCustomResourceWithClient struct {
	admission.ValidationInterface

	secretGetter        corev1client.SecretsGetter
	sarc                authorizationv1client.SubjectAccessReviewInterface
	routeValidationOpts routecommon.RouteValidationOptions
}

func NewValidateRoute() (admission.Interface, error) {
	ret := &validateCustomResourceWithClient{}

	delegate, err := customresourcevalidation.NewValidator(
		map[schema.GroupResource]bool{
			configv1.GroupVersion.WithResource("routes").GroupResource(): true,
		},
		map[schema.GroupVersionKind]customresourcevalidation.ObjectValidator{
			configv1.GroupVersion.WithKind("Route"): routeV1{
				secretsGetter:  ret.getSecretsGetter(),
				sarc:           ret.getSubjectAccessReviewsGetter(),
				validationOpts: ret.getRouteValidationOptions(),
			},
		})
	if err != nil {
		return nil, err
	}
	ret.ValidationInterface = delegate

	return ret, nil
}

var _ admissionrestconfig.WantsRESTClientConfig = &validateCustomResourceWithClient{}

func (a *validateCustomResourceWithClient) getSecretsGetter() corev1client.SecretsGetter {
	return a.secretGetter
}

func (a *validateCustomResourceWithClient) getSubjectAccessReviewsGetter() authorizationv1client.SubjectAccessReviewInterface {
	return a.sarc
}

func (a *validateCustomResourceWithClient) getRouteValidationOptions() routecommon.RouteValidationOptions {
	return a.routeValidationOpts
}

func (a *validateCustomResourceWithClient) SetRESTClientConfig(restClientConfig rest.Config) {
	var err error

	a.secretGetter, err = corev1client.NewForConfig(&restClientConfig)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}

	authclient, err := authorizationv1client.NewForConfig(&restClientConfig)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}

	a.sarc = authclient.SubjectAccessReviews()

	a.routeValidationOpts = routecommon.RouteValidationOptions{
		AllowExternalCertificates: feature.DefaultMutableFeatureGate.Enabled(featuregate.Feature(configv1.FeatureGateRouteExternalCertificate)),
	}

}

func (a *validateCustomResourceWithClient) ValidateInitialization() error {
	if a.secretGetter == nil {
		return fmt.Errorf(PluginName + " needs an secretsGetter")
	}

	if a.sarc == nil {
		return fmt.Errorf(PluginName + " needs an subjectAccessReviewsGetter")
	}

	if initializationValidator, ok := a.ValidationInterface.(admission.InitializationValidator); ok {
		return initializationValidator.ValidateInitialization()
	}

	return nil
}
