package authentication

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
	authenticationcel "k8s.io/apiserver/pkg/authentication/cel"
	crvalidation "k8s.io/kubernetes/openshift-kube-apiserver/admission/customresourcevalidation"
)

const PluginName = "config.openshift.io/ValidateAuthentication"

// Register registers a plugin
func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
		return crvalidation.NewValidator(
			map[schema.GroupResource]bool{
				configv1.GroupVersion.WithResource("authentications").GroupResource(): true,
			},
			map[schema.GroupVersionKind]crvalidation.ObjectValidator{
				configv1.GroupVersion.WithKind("Authentication"): authenticationV1{},
			})
	})
}

func toAuthenticationV1(uncastObj runtime.Object) (*configv1.Authentication, field.ErrorList) {
	if uncastObj == nil {
		return nil, nil
	}

	obj, ok := uncastObj.(*configv1.Authentication)
	if !ok {
		return nil, field.ErrorList{
			field.NotSupported(field.NewPath("kind"), fmt.Sprintf("%T", uncastObj), []string{"Authentication"}),
			field.NotSupported(field.NewPath("apiVersion"), fmt.Sprintf("%T", uncastObj), []string{"config.openshift.io/v1"}),
		}
	}

	return obj, nil
}

type authenticationV1 struct{}

func (authenticationV1) ValidateCreate(_ context.Context, uncastObj runtime.Object) field.ErrorList {
	obj, errs := toAuthenticationV1(uncastObj)
	if len(errs) > 0 {
		return errs
	}

	errs = append(errs, validation.ValidateObjectMeta(&obj.ObjectMeta, false, crvalidation.RequireNameCluster, field.NewPath("metadata"))...)
	errs = append(errs, validateAuthenticationSpecCreate(obj.Spec)...)

	return errs
}

func (authenticationV1) ValidateUpdate(_ context.Context, uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	obj, errs := toAuthenticationV1(uncastObj)
	if len(errs) > 0 {
		return errs
	}
	oldObj, errs := toAuthenticationV1(uncastOldObj)
	if len(errs) > 0 {
		return errs
	}

	errs = append(errs, validation.ValidateObjectMetaUpdate(&obj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))...)
	errs = append(errs, validateAuthenticationSpecUpdate(obj.Spec, oldObj.Spec)...)

	return errs
}

func (authenticationV1) ValidateStatusUpdate(_ context.Context, uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	obj, errs := toAuthenticationV1(uncastObj)
	if len(errs) > 0 {
		return errs
	}
	oldObj, errs := toAuthenticationV1(uncastOldObj)
	if len(errs) > 0 {
		return errs
	}

	errs = append(errs, validation.ValidateObjectMetaUpdate(&obj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))...)
	errs = append(errs, validateAuthenticationStatus(obj.Status)...)

	return errs
}

func validateAuthenticationSpecCreate(spec configv1.AuthenticationSpec) field.ErrorList {
	return validateAuthenticationSpec(spec)
}

func validateAuthenticationSpecUpdate(newspec, oldspec configv1.AuthenticationSpec) field.ErrorList {
	return validateAuthenticationSpec(newspec)
}

func validateAuthenticationSpec(spec configv1.AuthenticationSpec) field.ErrorList {
	errs := field.ErrorList{}
	specField := field.NewPath("spec")

	if spec.WebhookTokenAuthenticator != nil {
		switch spec.Type {
		case configv1.AuthenticationTypeNone, configv1.AuthenticationTypeIntegratedOAuth, "":
			// validate the secret name in WebhookTokenAuthenticator
			errs = append(
				errs,
				crvalidation.ValidateSecretReference(
					specField.Child("webhookTokenAuthenticator").Child("kubeConfig"),
					spec.WebhookTokenAuthenticator.KubeConfig,
					false,
				)...,
			)
		default:
			errs = append(errs, field.Invalid(specField.Child("webhookTokenAuthenticator"),
				spec.WebhookTokenAuthenticator, fmt.Sprintf("this field cannot be set with the %q .spec.type", spec.Type),
			))
		}
	}

	if spec.Type == configv1.AuthenticationTypeOIDC {
		for i, provider := range spec.OIDCProviders {
			errs = append(errs, validateOIDCProvider(specField.Child("oidcProviders").Index(i), provider)...)
		}
	}

	errs = append(errs, crvalidation.ValidateConfigMapReference(specField.Child("oauthMetadata"), spec.OAuthMetadata, false)...)

	return errs
}

func validateAuthenticationStatus(status configv1.AuthenticationStatus) field.ErrorList {
	return crvalidation.ValidateConfigMapReference(field.NewPath("status", "integratedOAuthMetadata"), status.IntegratedOAuthMetadata, false)
}

func validateOIDCProvider(path *field.Path, provider configv1.OIDCProvider) field.ErrorList {
	errs := field.ErrorList{}
	return errs
}

func validateClaimMappings(path *field.Path, claimMappings configv1.TokenClaimMappings) field.ErrorList {
	path = path.Child("claimMappings")
	errs := field.ErrorList{}
	errs = append(errs, validateUIDClaimMapping(path, claimMappings.UID))
	errs = append(errs, validateExtraClaimMapping(path, claimMappings.Extra...)...)
	return errs
}

func validateUIDClaimMapping(path *field.Path, uid *configv1.TokenClaimOrExpressionMapping) *field.Error {
	if uid == nil {
		return nil
	}

	if uid.Expression != "" {
		err := validateCELExpression(&authenticationcel.ClaimMappingExpression{
			Expression: uid.Expression,
		})
		if err != nil {
			return field.Invalid(path.Child("uid", "expression"), uid.Expression, err.Error())
		}
	}

	return nil
}

func validateExtraClaimMapping(path *field.Path, extras ...configv1.ExtraMapping) field.ErrorList {
	errs := field.ErrorList{}
	for i, extra := range extras {
		errs = append(errs, validateExtra(path.Child("extra").Index(i), extra))
	}
	return errs
}

func validateExtra(path *field.Path, extra configv1.ExtraMapping) *field.Error {
	err := validateCELExpression(&authenticationcel.ExtraMappingExpression{
		Key:        extra.Key,
		Expression: extra.ValueExpression,
	})
	if err != nil {
		return field.Invalid(path, extra.ValueExpression, err.Error())
	}

	return nil
}

func validateCELExpression(accessor authenticationcel.ExpressionAccessor) error {
	_, err := authenticationcel.NewDefaultCompiler().CompileClaimsExpression(accessor)
	return err
}
