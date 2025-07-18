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
	"k8s.io/apiserver/pkg/cel/library"
	"k8s.io/apiserver/pkg/warning"
	"k8s.io/klog/v2"
	"k8s.io/utils/lru"

	"github.com/google/cel-go/checker"

	configv1 "github.com/openshift/api/config/v1"
	authenticationcel "k8s.io/apiserver/pkg/authentication/cel"
	crvalidation "k8s.io/kubernetes/openshift-kube-apiserver/admission/customresourcevalidation"
)

const PluginName = "config.openshift.io/ValidateAuthentication"

const (
	individualExpressionExcessiveCostThreshold = 10000000
	fixedCost                                  = 1 << 20
	wholeResourceExcessiveCostThreshold        = 100000000
	excessiveCompileDuration                   = time.Second
)

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
						sizeEstimator: &fixedCostSizeEstimator{
							cost: fixedCost,
						},
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
	sizeEstimator  checker.CostEstimator
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
		results := validateOIDCProvider(ctx, specField.Child("oidcProviders").Index(i), cel, provider)
		var expressionCost uint64 = 0
		excessiveCosts := []string{}
		for _, result := range results {
			if result.internalErr != nil {
				errs = append(errs, field.InternalError(result.path, result.internalErr))
				continue
			}

			if result.val != nil {
				if result.val.Error() != nil {
					errs = append(errs, field.Invalid(result.path, result.value, result.val.Error().Error()))
				}

				if result.val.Warning() != "" {
					warns = append(warns, result.val.Warning())
				}

				if coster, ok := result.val.(Coster); ok {
					cost := coster.Cost()
					if cost >= individualExpressionExcessiveCostThreshold {
						excessiveCosts = append(excessiveCosts, result.path.String())
					}

					expressionCost += cost
				}
			}
		}

		if expressionCost > wholeResourceExcessiveCostThreshold {
			warns = append(warns, fmt.Sprintf("runtime cost of all CEL expressions exceeds %d points. expressions larger than %d points: %v", wholeResourceExcessiveCostThreshold, individualExpressionExcessiveCostThreshold, excessiveCosts))
		}
	}
	// ----------------

	return errs, warns
}

func validateAuthenticationStatus(status configv1.AuthenticationStatus) field.ErrorList {
	return crvalidation.ValidateConfigMapReference(field.NewPath("status", "integratedOAuthMetadata"), status.IntegratedOAuthMetadata, false)
}

type oidcProviderValidationResult struct {
	path        *field.Path
	value       any
	val         ValidationResult
	internalErr error
}

type ValidationResult interface {
	Error() error
	Warning() string
}

type Coster interface {
	Cost() uint64
}

func validateOIDCProvider(ctx context.Context, path *field.Path, cel *celStore, provider configv1.OIDCProvider) []oidcProviderValidationResult {
	return validateClaimMappings(ctx, path, cel, provider.ClaimMappings)
}

func validateClaimMappings(ctx context.Context, path *field.Path, cel *celStore, claimMappings configv1.TokenClaimMappings) []oidcProviderValidationResult {
	path = path.Child("claimMappings")

	out := []oidcProviderValidationResult{}

	out = append(out, validateUIDClaimMapping(ctx, path, cel, claimMappings.UID)...)
	out = append(out, validateExtraClaimMapping(ctx, path, cel, claimMappings.Extra...)...)

	return out
}

func validateUIDClaimMapping(ctx context.Context, path *field.Path, cel *celStore, uid *configv1.TokenClaimOrExpressionMapping) []oidcProviderValidationResult {
	if uid == nil {
		return nil
	}

	out := []oidcProviderValidationResult{}
	if uid.Expression != "" {
		childPath := path.Child("uid", "expression")
		res, err := validateCELExpression(ctx, cel, &authenticationcel.ClaimMappingExpression{
			Expression: uid.Expression,
		})

		out = append(out, oidcProviderValidationResult{
			path:        childPath,
			val:         res,
			value:       uid.Expression,
			internalErr: err,
		})
	}

	return out
}

func validateExtraClaimMapping(ctx context.Context, path *field.Path, cel *celStore, extras ...configv1.ExtraMapping) []oidcProviderValidationResult {
	out := []oidcProviderValidationResult{}
	for i, extra := range extras {
		out = append(out, validateExtra(ctx, path.Child("extra").Index(i), cel, extra))
	}

	return out
}

func validateExtra(ctx context.Context, path *field.Path, cel *celStore, extra configv1.ExtraMapping) oidcProviderValidationResult {
	childPath := path.Child("valueExpression")
	res, err := validateCELExpression(ctx, cel, &authenticationcel.ExtraMappingExpression{
		Key:        extra.Key,
		Expression: extra.ValueExpression,
	})

	return oidcProviderValidationResult{
		path:        childPath,
		val:         res,
		value:       extra.ValueExpression,
		internalErr: err,
	}
}

type celCompileResult struct {
	err  error
	cost uint64
}

type validationResult struct {
	compRes celCompileResult
	warn    string
}

func (vr validationResult) Error() error {
	return vr.compRes.err
}

func (vr validationResult) Warning() string {
	return vr.warn
}

func (vr validationResult) Cost() uint64 {
	return vr.compRes.cost
}

type panickedErr struct {
	error
}

func validateCELExpression(ctx context.Context, cel *celStore, accessor authenticationcel.ExpressionAccessor) (*validationResult, error) {
	// if context has been canceled, don't try to compile any expressions
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result, err, _ := cel.compilingGroup.Do(accessor.GetExpression(), func() (interface{}, error) {
		// if the expression is not currently being compiled, it might have already been compiled
		if val, ok := cel.compiledStore.Get(accessor.GetExpression()); ok {
			res, ok := val.(celCompileResult)
			if !ok {
				return nil, fmt.Errorf("expected return value from cache of compiled expressions to be of type celCompileResult but was %T", val)
			}

			return validationResult{
				compRes: res,
			}, nil
		}

		// expression is not currently being compiled, and has not been compiled before (or has been long enough since it was last compiled that we dropped it).
		// Let's compile it.
		warningString := ""
		compiled := make(chan celCompileResult)
		defer close(compiled)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					// convert the panic into an error state for the expression
					compiled <- celCompileResult{
						err: panickedErr{fmt.Errorf("recovered from a panic while compiling: %v", r)},
					}
				}
			}()

			res, compErr := cel.compiler.CompileClaimsExpression(accessor)
			cost, err := checker.Cost(res.AST.NativeRep(), &library.CostEstimator{
				SizeEstimator: cel.sizeEstimator,
			})
			if err != nil {
				klog.Errorf("unable to estimate cost for expression %q: %v. Defaulting cost to %d", accessor.GetExpression(), err, fixedCost)
				cost = checker.CostEstimate{Max: fixedCost}
			}

			compiled <- celCompileResult{
				err:  compErr,
				cost: cost.Max,
			}
		}()

		warning := make(chan string, 1)
		timer := time.AfterFunc(excessiveCompileDuration, func() {
			defer close(warning)
			warning <- fmt.Sprintf("cel expression %q took excessively long to compile (%s)", accessor.GetExpression(), excessiveCompileDuration)
		})

		res := <-compiled

		timer.Stop()

		// check if we received a warning. If not, continue
		select {
		case warn := <-warning:
			warningString = warn
		default:
			break
		}

		if res.err != nil {
			if panicErr, ok := res.err.(panickedErr); ok {
				return nil, panicErr
			}
		}

		validationRes := validationResult{
			compRes: res,
			warn:    warningString,
		}

		cel.compiledStore.Add(accessor.GetExpression(), res)

		return validationRes, nil
	})
	if err != nil {
		return nil, fmt.Errorf("running compilation of expression %q: %v", accessor.GetExpression(), err)
	}

	validRes := result.(validationResult)
	return &validRes, nil
}

type fixedCostSizeEstimator struct {
	cost uint64
}

func (fcse *fixedCostSizeEstimator) EstimateSize(element checker.AstNode) *checker.SizeEstimate {
	return &checker.SizeEstimate{Min: fcse.cost, Max: fcse.cost}
}

func (fcse *fixedCostSizeEstimator) EstimateCallCost(function, overloadID string, target *checker.AstNode, args []checker.AstNode) *checker.CallEstimate {
	return nil
}
