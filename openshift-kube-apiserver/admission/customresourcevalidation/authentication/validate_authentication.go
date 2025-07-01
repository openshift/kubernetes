package authentication

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/sync/singleflight"
	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/warning"
	"k8s.io/klog/v2"
	"k8s.io/utils/lru"

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
				configv1.GroupVersion.WithKind("Authentication"): authenticationV1{
					cel: &celStore{
						compiledStore:  lru.New(100),
						compilingGroup: new(singleflight.Group),
						compiler:       authenticationcel.NewDefaultCompiler(),
					},
				},
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

type celStore struct {
	compilingGroup *singleflight.Group
	compiledStore  *lru.Cache
	compiler       authenticationcel.Compiler
}

type authenticationV1 struct {
	cel *celStore
}

func (a authenticationV1) ValidateCreate(ctx context.Context, uncastObj runtime.Object) field.ErrorList {
	obj, errs := toAuthenticationV1(uncastObj)
	if len(errs) > 0 {
		return errs
	}

	errs = append(errs, validation.ValidateObjectMeta(&obj.ObjectMeta, false, crvalidation.RequireNameCluster, field.NewPath("metadata"))...)
	err, warns := validateAuthenticationSpecCreate(ctx, obj.Spec, a.cel)
	errs = append(errs, err...)
	for _, warn := range warns {
		warning.AddWarning(ctx, "", warn)
		klog.Warning(warn)
	}

	return errs
}

func (a authenticationV1) ValidateUpdate(ctx context.Context, uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	obj, errs := toAuthenticationV1(uncastObj)
	if len(errs) > 0 {
		return errs
	}
	oldObj, errs := toAuthenticationV1(uncastOldObj)
	if len(errs) > 0 {
		return errs
	}

	errs = append(errs, validation.ValidateObjectMetaUpdate(&obj.ObjectMeta, &oldObj.ObjectMeta, field.NewPath("metadata"))...)

	err, warns := validateAuthenticationSpecUpdate(ctx, obj.Spec, oldObj.Spec, a.cel)
	errs = append(errs, err...)
	for _, warn := range warns {
		warning.AddWarning(ctx, "", warn)
	}

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

func validateAuthenticationSpecCreate(ctx context.Context, spec configv1.AuthenticationSpec, cel *celStore) (field.ErrorList, []string) {
	return validateAuthenticationSpec(ctx, spec, cel)
}

func validateAuthenticationSpecUpdate(ctx context.Context, newspec, oldspec configv1.AuthenticationSpec, cel *celStore) (field.ErrorList, []string) {
	return validateAuthenticationSpec(ctx, newspec, cel)
}

func validateAuthenticationSpec(ctx context.Context, spec configv1.AuthenticationSpec, cel *celStore) (field.ErrorList, []string) {
	errs := field.ErrorList{}
	warns := []string{}
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

	errs = append(errs, crvalidation.ValidateConfigMapReference(specField.Child("oauthMetadata"), spec.OAuthMetadata, false)...)

	// Perform External OIDC Provider related validations
	// ----------------

	// There is currently no guarantee that these fields are not set when the spec.Type is != OIDC.
	// To ensure we are enforcing approriate admission validations at all times, just always iterate through the list
	// of OIDC Providers and perform the validations.
	// If/when the openshift/api admission validations are updated to enforce that this field is not configured
	// when Type != OIDC, this loop should be a no-op due to an empty list.
	for i, provider := range spec.OIDCProviders {
		err, warnings := validateOIDCProvider(ctx, specField.Child("oidcProviders").Index(i), cel, provider)
		errs = append(errs, err...)
		warns = append(warns, warnings...)
	}
	// ----------------

	return errs, warns
}

func validateAuthenticationStatus(status configv1.AuthenticationStatus) field.ErrorList {
	return crvalidation.ValidateConfigMapReference(field.NewPath("status", "integratedOAuthMetadata"), status.IntegratedOAuthMetadata, false)
}

func validateOIDCProvider(ctx context.Context, path *field.Path, cel *celStore, provider configv1.OIDCProvider) (field.ErrorList, []string) {
	return validateClaimMappings(ctx, path, cel, provider.ClaimMappings)
}

func validateClaimMappings(ctx context.Context, path *field.Path, cel *celStore, claimMappings configv1.TokenClaimMappings) (field.ErrorList, []string) {
	path = path.Child("claimMappings")
	errs := field.ErrorList{}
	warns := []string{}

	err, warn := validateUIDClaimMapping(ctx, path, cel, claimMappings.UID)
	errs = append(errs, err...)
	if len(warn) > 0 {
		warns = append(warns, warn)
	}

	err, warnings := validateExtraClaimMapping(ctx, path, cel, claimMappings.Extra...)
	errs = append(errs, err...)
	warns = append(warns, warnings...)

	return errs, warns
}

func validateUIDClaimMapping(ctx context.Context, path *field.Path, cel *celStore, uid *configv1.TokenClaimOrExpressionMapping) (field.ErrorList, string) {
	if uid == nil {
		return nil, ""
	}

	var warn string
	if uid.Expression != "" {
		childPath := path.Child("uid", "expression")
		var err error
		err, warn = validateCELExpression(ctx, cel, &authenticationcel.ClaimMappingExpression{
			Expression: uid.Expression,
		})

		if len(warn) > 0 {
			warn = fmt.Sprintf("validating %s: %s", childPath, warn)
		}

		if err != nil {
			return field.ErrorList{field.Invalid(childPath, uid.Expression, err.Error())}, warn
		}
	}

	return nil, warn
}

func validateExtraClaimMapping(ctx context.Context, path *field.Path, cel *celStore, extras ...configv1.ExtraMapping) (field.ErrorList, []string) {
	errs := field.ErrorList{}
	warns := []string{}
	for i, extra := range extras {
		err, warn := validateExtra(ctx, path.Child("extra").Index(i), cel, extra)
		errs = append(errs, err...)
		if len(warn) > 0 {
			warns = append(warns, warn)
		}
	}
	return errs, warns
}

func validateExtra(ctx context.Context, path *field.Path, cel *celStore, extra configv1.ExtraMapping) (field.ErrorList, string) {
	childPath := path.Child("valueExpression")
	err, warn := validateCELExpression(ctx, cel, &authenticationcel.ExtraMappingExpression{
		Key:        extra.Key,
		Expression: extra.ValueExpression,
	})

	if len(warn) > 0 {
		warn = fmt.Sprintf("validating %s: %s", childPath, warn)
	}

	if err != nil {
		return field.ErrorList{field.Invalid(childPath, extra.ValueExpression, err.Error())}, warn
	}

	return nil, warn
}

type compileResult struct {
	err  error
	warn string
}

func validateCELExpression(ctx context.Context, cel *celStore, accessor authenticationcel.ExpressionAccessor) (error, string) {
	// if context has been canceled, don't try to compile any expressions
	if err := ctx.Err(); err != nil {
		return err, ""
	}

	result, err, _ := cel.compilingGroup.Do(accessor.GetExpression(), func() (interface{}, error) {
		// if the expression is not currently being compiled, it might have already been compiled
		if val, ok := cel.compiledStore.Get(accessor.GetExpression()); ok {
			if val != nil {
				res := val.(compileResult)
				return res, nil
			}

			return nil, nil
		}

		// expression is not currently being compiled, and has not been compiled before (or has been long enough since it was last compiled that we dropped it).
		// Let's compile it.
		warningString := ""
		compiled := make(chan error)
		defer close(compiled)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					// convert the panic into an error state for the expression
					compiled <- fmt.Errorf("recovered from a panic while compiling expression %q: %v", accessor.GetExpression(), r)
				}
			}()

			_, err := cel.compiler.CompileClaimsExpression(accessor)

			compiled <- err
		}()

		warning := make(chan string, 1)
		timer := time.AfterFunc(time.Second, func() {
			defer close(warning)
			warning <- fmt.Sprintf("cel expression %q took more than 1 second to compile", accessor.GetExpression())
		})

		err := <-compiled

		timer.Stop()

		// check if we received a warning. If not, continue
		select {
		case warn := <-warning:
			warningString = warn
		default:
			break
		}

		compilationResult := compileResult{
			err, warningString,
		}

		cel.compiledStore.Add(accessor.GetExpression(), compilationResult)

		return compilationResult, nil
	})
	if err != nil {
		return fmt.Errorf("running compilation of expression %q: %v", accessor.GetExpression(), err), ""
	}

	compileRes := result.(compileResult)
	return compileRes.err, compileRes.warn
}
