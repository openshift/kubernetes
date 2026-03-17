package authentication

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"math"
	"slices"
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

	celgo "github.com/google/cel-go/cel"
	"github.com/google/cel-go/checker"
	"github.com/google/cel-go/common/operators"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"

	configv1 "github.com/openshift/api/config/v1"
	authenticationcel "k8s.io/apiserver/pkg/authentication/cel"
	crvalidation "k8s.io/kubernetes/openshift-kube-apiserver/admission/customresourcevalidation"
)

const PluginName = "config.openshift.io/ValidateAuthentication"

const (
	wholeResourceExcessiveCostThreshold = 100000000
	excessiveCompileDuration            = time.Second
	costlyExpressionWarningCount        = 3

	// This is the default KAS request header size limit in bytes.
	// Because JWTs are only limited in size by the maximum request header size,
	// we can use this fixed value to make pessimistic size estimates by assuming
	// that the inputs were decoded from base64-encoded JSON.
	//
	// This isn't very precise, but can still be used to provide
	// end-users a signal that they are potentially doing very expensive
	// operations with CEL expressions whose cost is dependent
	// on the size of the input.
	fixedSize = 1 << 20
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
					cel: defaultCelStore(),
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
	compilingGroup singleFlightDoer
	compiledStore  compiledExpressionStore
	compiler       authenticationcel.Compiler
	sizeEstimator  checker.CostEstimator
	timerFactory   timerFactory
}

func defaultCelStore() *celStore {
	return &celStore{
		compiledStore:  lru.New(100),
		compilingGroup: new(singleflight.Group),
		compiler:       authenticationcel.NewDefaultCompiler(),
		sizeEstimator: &fixedSizeEstimator{
			size: fixedSize,
		},
		timerFactory: &excessiveCompileTimerFactory{},
	}
}

type singleFlightDoer interface {
	Do(key string, fn func() (any, error)) (any, error, bool)
}

type compiledExpressionStore interface {
	Add(key lru.Key, value interface{})
	Get(key lru.Key) (value interface{}, ok bool)
}

type timerFactory interface {
	Timer(time.Duration, func()) timer
}

type timer interface {
	Stop() bool
}

type excessiveCompileTimerFactory struct{}

func (ectf *excessiveCompileTimerFactory) Timer(duration time.Duration, do func()) timer {
	return time.AfterFunc(duration, do)
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
	errs = append(errs, validateAuthenticationSpecCreate(ctx, obj.Spec, a.cel)...)

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
	errs = append(errs, validateAuthenticationSpecUpdate(ctx, obj.Spec, oldObj.Spec, a.cel)...)

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

func validateAuthenticationSpecCreate(ctx context.Context, spec configv1.AuthenticationSpec, cel *celStore) field.ErrorList {
	return validateAuthenticationSpec(ctx, spec, cel)
}

func validateAuthenticationSpecUpdate(ctx context.Context, newspec, oldspec configv1.AuthenticationSpec, cel *celStore) field.ErrorList {
	return validateAuthenticationSpec(ctx, newspec, cel)
}

func validateAuthenticationSpec(ctx context.Context, spec configv1.AuthenticationSpec, cel *celStore) field.ErrorList {
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

	errs = append(errs, crvalidation.ValidateConfigMapReference(specField.Child("oauthMetadata"), spec.OAuthMetadata, false)...)

	// Perform External OIDC Provider related validations
	// ----------------

	// There is currently no guarantee that these fields are not set when the spec.Type is != OIDC.
	// To ensure we are enforcing approriate admission validations at all times, just always iterate through the list
	// of OIDC Providers and perform the validations.
	// If/when the openshift/api admission validations are updated to enforce that this field is not configured
	// when Type != OIDC, this loop should be a no-op due to an empty list.
	for i, provider := range spec.OIDCProviders {
		errs = append(errs, validateOIDCProvider(ctx, specField.Child("oidcProviders").Index(i), cel, provider)...)
	}
	// ----------------

	return errs
}

func validateAuthenticationStatus(status configv1.AuthenticationStatus) field.ErrorList {
	return crvalidation.ValidateConfigMapReference(field.NewPath("status", "integratedOAuthMetadata"), status.IntegratedOAuthMetadata, false)
}

type costRecorder struct {
	Recordings []costRecording
}

func (cr *costRecorder) AddRecording(field *field.Path, cost uint64) {
	cr.Recordings = append(cr.Recordings, costRecording{
		Field: field,
		Cost:  cost,
	})
}

type costRecording struct {
	Field *field.Path
	Cost  uint64
}

func validateOIDCProvider(ctx context.Context, path *field.Path, cel *celStore, provider configv1.OIDCProvider) field.ErrorList {
	costRecorder := &costRecorder{}

	errs := field.ErrorList{}
	claimMappingErrs, usernameResult, extraResults := validateClaimMappings(ctx, path, cel, costRecorder, provider.ClaimMappings)
	errs = append(errs, claimMappingErrs...)
	claimValidationResults, claimValidationErrs := validateClaimValidationRules(ctx, path, cel, costRecorder, provider.ClaimValidationRules...)
	errs = append(errs, claimValidationErrs...)
	errs = append(errs, validateUserValidationRules(ctx, path, cel, costRecorder, provider.UserValidationRules...)...)
	errs = append(errs, validateEmailVerifiedUsage(path, provider.ClaimMappings.Username.Expression, usernameResult, extraResults, claimValidationResults)...)
	var totalCELExpressionCost uint64 = 0

	for _, recording := range costRecorder.Recordings {
		totalCELExpressionCost = addCost(totalCELExpressionCost, recording.Cost)
	}

	if totalCELExpressionCost > wholeResourceExcessiveCostThreshold {
		costlyExpressions := getNMostCostlyExpressions(costlyExpressionWarningCount, costRecorder.Recordings...)
		warn := fmt.Sprintf("runtime cost of all CEL expressions exceeds %d points. top %d most costly expressions: %v", wholeResourceExcessiveCostThreshold, len(costlyExpressions), costlyExpressions)
		warning.AddWarning(ctx, "", warn)
		klog.Warning(warn)
	}

	return errs
}

// addCost adds a cost value to a total value,
// returning the resulting value.
// addCost handles integer overflow errors
// by just always returning the maximum uint64
// value if an overflow would occur.
func addCost(total, cost uint64) uint64 {
	if total > math.MaxUint64-cost {
		return math.MaxUint64
	}

	return total + cost
}

func getNMostCostlyExpressions(n int, records ...costRecording) []costRecording {
	// sort in descending order of cost
	slices.SortFunc(records, func(a, b costRecording) int {
		return cmp.Compare(b.Cost, a.Cost)
	})

	// safely get the N most expensive cost records
	if len(records) > n {
		return records[:n]
	}

	return records
}

func validateClaimMappings(ctx context.Context, path *field.Path, cel *celStore, costRecorder *costRecorder, claimMappings configv1.TokenClaimMappings) (field.ErrorList, *authenticationcel.CompilationResult, []authenticationcel.CompilationResult) {
	path = path.Child("claimMappings")

	out := field.ErrorList{}

	usernameResult, errs := validateUsernameClaimMapping(ctx, path, cel, costRecorder, claimMappings.Username)
	out = append(out, errs...)

	out = append(out, validateGroupsClaimMapping(ctx, path, cel, costRecorder, claimMappings.Groups)...)
	out = append(out, validateUIDClaimMapping(ctx, path, cel, costRecorder, claimMappings.UID)...)

	extraResults, errs := validateExtraClaimMapping(ctx, path, cel, costRecorder, claimMappings.Extra...)
	out = append(out, errs...)

	return out, usernameResult, extraResults
}

func validateUIDClaimMapping(ctx context.Context, path *field.Path, cel *celStore, costRecorder *costRecorder, uid *configv1.TokenClaimOrExpressionMapping) field.ErrorList {
	if uid == nil {
		return nil
	}

	out := field.ErrorList{}

	if uid.Expression != "" {
		childPath := path.Child("uid", "expression")

		_, errs := validateClaimMappingCELExpression(ctx, cel, costRecorder, childPath, &authenticationcel.ClaimMappingExpression{
			Expression: uid.Expression,
		})
		out = append(out, errs...)
	}

	return out
}

// validateUsernameClaimMapping validates the CEL expression in the username claim mapping,
// if one is specified. The username mapping determines the username of the authenticated user
// and may be specified as either a raw claim name or a CEL expression.
func validateUsernameClaimMapping(ctx context.Context, path *field.Path, cel *celStore, costRecorder *costRecorder, username configv1.UsernameClaimMapping) (*authenticationcel.CompilationResult, field.ErrorList) {
	if username.Expression == "" {
		return nil, nil
	}

	childPath := path.Child("username", "expression")

	result, errs := validateClaimMappingCELExpression(ctx, cel, costRecorder, childPath, &authenticationcel.ClaimMappingExpression{
		Expression: username.Expression,
	})
	if len(errs) > 0 {
		return nil, errs
	}
	return result, nil
}

// validateGroupsClaimMapping validates the CEL expression in the groups claim mapping,
// if one is specified. The groups mapping determines the groups of the authenticated user
// and may be specified as either a raw claim name or a CEL expression.
func validateGroupsClaimMapping(ctx context.Context, path *field.Path, cel *celStore, costRecorder *costRecorder, groups configv1.PrefixedClaimMapping) field.ErrorList {
	if groups.Expression == "" {
		return nil
	}

	childPath := path.Child("groups", "expression")

	_, errs := validateClaimMappingCELExpression(ctx, cel, costRecorder, childPath, &authenticationcel.ClaimMappingExpression{
		Expression: groups.Expression,
	})
	return errs
}

// validateClaimValidationRules validates the CEL expressions in each claim validation rule.
// Claim validation rules are evaluated against the raw JWT claims and must return a boolean.
// Each rule may also specify a messageExpression that is evaluated to produce a human-readable
// error message when the validation rule returns false.
func validateClaimValidationRules(ctx context.Context, path *field.Path, cel *celStore, costRecorder *costRecorder, rules ...configv1.TokenClaimValidationRule) ([]authenticationcel.CompilationResult, field.ErrorList) {
	out := field.ErrorList{}
	var results []authenticationcel.CompilationResult
	for i, rule := range rules {
		if rule.Type != configv1.TokenValidationRuleTypeCEL {
			continue
		}

		rulePath := path.Child("claimValidationRules").Index(i)
		result, errs := validateClaimMappingCELExpression(ctx, cel, costRecorder, rulePath.Child("cel", "expression"), &authenticationcel.ClaimValidationCondition{
			Expression: rule.CEL.Expression,
		})
		out = append(out, errs...)
		if len(errs) == 0 && result != nil {
			results = append(results, *result)
		}
	}
	return results, out
}

// validateUserValidationRules validates the CEL expressions in each user validation rule.
// User validation rules are evaluated against the mapped UserInfo object after all claim
// mappings have been applied, and must return a boolean. Each rule may also specify a
// messageExpression that is evaluated to produce a human-readable error message when
// the validation rule returns false.
func validateUserValidationRules(ctx context.Context, path *field.Path, cel *celStore, costRecorder *costRecorder, rules ...configv1.TokenUserValidationRule) field.ErrorList {
	out := field.ErrorList{}
	for i, rule := range rules {
		rulePath := path.Child("userValidationRules").Index(i)

		_, errs := validateUserCELExpression(ctx, cel, costRecorder, rulePath.Child("expression"), &authenticationcel.UserValidationCondition{
			Expression: rule.Expression,
		})
		out = append(out, errs...)
	}

	return out
}

func validateExtraClaimMapping(ctx context.Context, path *field.Path, cel *celStore, costRecorder *costRecorder, extras ...configv1.ExtraMapping) ([]authenticationcel.CompilationResult, field.ErrorList) {
	out := field.ErrorList{}
	var results []authenticationcel.CompilationResult
	for i, extra := range extras {
		result, errs := validateExtra(ctx, path.Child("extra").Index(i), cel, costRecorder, extra)
		out = append(out, errs...)
		if result != nil {
			results = append(results, *result)
		}
	}

	return results, out
}

func validateExtra(ctx context.Context, path *field.Path, cel *celStore, costRecorder *costRecorder, extra configv1.ExtraMapping) (*authenticationcel.CompilationResult, field.ErrorList) {
	childPath := path.Child("valueExpression")

	result, errs := validateClaimMappingCELExpression(ctx, cel, costRecorder, childPath, &authenticationcel.ExtraMappingExpression{
		Key:        extra.Key,
		Expression: extra.ValueExpression,
	})
	if len(errs) > 0 {
		return nil, errs
	}
	return result, nil
}

type celCompileResult struct {
	err               error
	cost              uint64
	compilationResult *authenticationcel.CompilationResult
}

func validateClaimMappingCELExpression(ctx context.Context, cel *celStore, costRecorder *costRecorder, path *field.Path, accessor authenticationcel.ExpressionAccessor) (*authenticationcel.CompilationResult, field.ErrorList) {
	return compileExpression(ctx, cel, costRecorder, path, accessor, cel.compiler.CompileClaimsExpression)
}

// validateUserCELExpression is like validateClaimMappingCELExpression but uses CompileUserExpression
// instead of CompileClaimsExpression, making user.* variables available to the expression.
func validateUserCELExpression(ctx context.Context, cel *celStore, costRecorder *costRecorder, path *field.Path, accessor authenticationcel.ExpressionAccessor) (*authenticationcel.CompilationResult, field.ErrorList) {
	return compileExpression(ctx, cel, costRecorder, path, accessor, cel.compiler.CompileUserExpression)
}

// compileExpression is the shared implementation for validating CEL expressions.
// The compileFn parameter allows callers to specify which compiler method to use,
// enabling reuse across different expression scopes (e.g. claims vs user expressions).
func compileExpression(ctx context.Context, cel *celStore, costRecorder *costRecorder, path *field.Path, accessor authenticationcel.ExpressionAccessor, compileFn func(authenticationcel.ExpressionAccessor) (authenticationcel.CompilationResult, error)) (*authenticationcel.CompilationResult, field.ErrorList) {
	if err := ctx.Err(); err != nil {
		return nil, field.ErrorList{field.InternalError(path, err)}
	}

	cacheKey := fmt.Sprintf("%T:%s", accessor, accessor.GetExpression())
	result, err, _ := cel.compilingGroup.Do(cacheKey, func() (interface{}, error) {
		if val, ok := cel.compiledStore.Get(cacheKey); ok {
			res, ok := val.(celCompileResult)
			if !ok {
				return nil, fmt.Errorf("expected return value from cache of compiled expressions to be of type celCompileResult but was %T", val)
			}
			return res, nil
		}

		// expression is not currently being compiled, and has not been compiled before (or has been long enough since it was last compiled that we dropped it).
		// Let's compile it.

		// Asynchronously handle excessive compilation time so we
		// can still log a warning in the event the process has died
		// before compilation of the expression has finished.
		warningChan := make(chan string, 1)
		timer := cel.timerFactory.Timer(excessiveCompileDuration, func() {
			defer close(warningChan)
			warn := fmt.Sprintf("cel expression %q took excessively long to compile (%s)", accessor.GetExpression(), excessiveCompileDuration)
			klog.Warning(warn)
			warningChan <- warn
		})

		compRes, compErr := compileFn(accessor)

		timer.Stop()

		res := celCompileResult{
			err:               compErr,
			compilationResult: &compRes,
		}

		if compRes.AST != nil && compErr == nil {
			cost, err := checker.Cost(compRes.AST.NativeRep(), &library.CostEstimator{
				SizeEstimator: cel.sizeEstimator,
			})
			// Because we are only warning on excessive cost, we shouldn't prevent the create/update of the resource if we can successfully
			// compile the expression but are unable to estimate the cost. The Structured Authentication Configuration feature does not
			// gate on cost of expressions, so we are doing a best-effort warning here.
			// Instead, default to our best estimate of the worst case cost.
			if err != nil {
				klog.Errorf("unable to estimate cost for expression %q: %v. Defaulting cost to %d", accessor.GetExpression(), err, fixedSize)
				cost = checker.CostEstimate{Max: fixedSize}
			}
			res.cost = cost.Max
		}

		select {
		case warn := <-warningChan:
			warning.AddWarning(ctx, "", warn)
		default:
		}

		cel.compiledStore.Add(cacheKey, res)
		return res, nil
	})
	if err != nil {
		return nil, field.ErrorList{field.InternalError(path, fmt.Errorf("running compilation of expression %q: %v", accessor.GetExpression(), err))}
	}

	compileRes, ok := result.(celCompileResult)
	if !ok {
		return nil, field.ErrorList{field.InternalError(path, fmt.Errorf("expected result to be of type celCompileResult, but got %T", result))}
	}

	if compileRes.err != nil {
		return nil, field.ErrorList{field.Invalid(path, accessor.GetExpression(), compileRes.err.Error())}
	}

	costRecorder.AddRecording(path, compileRes.cost)
	return compileRes.compilationResult, nil
}

type fixedSizeEstimator struct {
	size uint64
}

func (fcse *fixedSizeEstimator) EstimateSize(element checker.AstNode) *checker.SizeEstimate {
	return &checker.SizeEstimate{Min: fcse.size, Max: fcse.size}
}

func (fcse *fixedSizeEstimator) EstimateCallCost(function, overloadID string, target *checker.AstNode, args []checker.AstNode) *checker.CallEstimate {
	return nil
}

// validateEmailVerifiedUsage enforces that when claims.email is used in the
// username expression, claims.email_verified must be referenced in at least
// one of: username.expression, extra[*].valueExpression, or
// claimValidationRules[*].expression.
func validateEmailVerifiedUsage(path *field.Path, usernameExpression string, usernameResult *authenticationcel.CompilationResult, extraResults []authenticationcel.CompilationResult, claimValidationResults []authenticationcel.CompilationResult) field.ErrorList {
	if usernameResult == nil {
		return nil
	}

	if !usesEmailClaim(usernameResult.AST) {
		return nil
	}

	if usesEmailVerifiedClaim(usernameResult.AST) || anyUsesEmailVerifiedClaim(extraResults) || anyUsesEmailVerifiedClaim(claimValidationResults) {
		return nil
	}

	return field.ErrorList{field.Invalid(
		path.Child("claimMappings", "username", "expression"),
		usernameExpression,
		"claims.email_verified must be used in claimMappings.username.expression or claimMappings.extra[*].valueExpression or claimValidationRules[*].expression when claims.email is used in claimMappings.username.expression",
	)}
}

func anyUsesEmailVerifiedClaim(results []authenticationcel.CompilationResult) bool {
	for _, result := range results {
		if usesEmailVerifiedClaim(result.AST) {
			return true
		}
	}
	return false
}

// usesEmailClaim, usesEmailVerifiedClaim, hasSelectExp,
// isIdentOperand, and isConstField are copied from the upstream Kubernetes apiserver
// CEL validation logic introduced in https://github.com/kubernetes/kubernetes/pull/123737 (commit 121607e):
// https://github.com/kubernetes/kubernetes/blob/bfb362c57578518bed8e08a56a7318bab9b57429/staging/src/k8s.io/apiserver/pkg/apis/apiserver/validation/validation.go#L443
func usesEmailClaim(ast *celgo.Ast) bool {
	if ast == nil {
		return false
	}
	return hasSelectExp(ast.Expr(), "claims", "email")
}

func usesEmailVerifiedClaim(ast *celgo.Ast) bool {
	if ast == nil {
		return false
	}
	return hasSelectExp(ast.Expr(), "claims", "email_verified")
}

func hasSelectExp(exp *exprpb.Expr, operand, field string) bool {
	if exp == nil {
		return false
	}
	switch e := exp.ExprKind.(type) {
	case *exprpb.Expr_ConstExpr,
		*exprpb.Expr_IdentExpr:
		return false
	case *exprpb.Expr_SelectExpr:
		s := e.SelectExpr
		if s == nil {
			return false
		}
		if isIdentOperand(s.Operand, operand) && s.Field == field {
			return true
		}
		return hasSelectExp(s.Operand, operand, field)
	case *exprpb.Expr_CallExpr:
		c := e.CallExpr
		if c == nil {
			return false
		}
		if c.Target == nil && c.Function == operators.OptSelect && len(c.Args) == 2 &&
			isIdentOperand(c.Args[0], operand) && isConstField(c.Args[1], field) {
			return true
		}
		for _, arg := range c.Args {
			if hasSelectExp(arg, operand, field) {
				return true
			}
		}
		return hasSelectExp(c.Target, operand, field)
	case *exprpb.Expr_ListExpr:
		l := e.ListExpr
		if l == nil {
			return false
		}
		for _, element := range l.Elements {
			if hasSelectExp(element, operand, field) {
				return true
			}
		}
		return false
	case *exprpb.Expr_StructExpr:
		s := e.StructExpr
		if s == nil {
			return false
		}
		for _, entry := range s.Entries {
			if hasSelectExp(entry.GetMapKey(), operand, field) {
				return true
			}
			if hasSelectExp(entry.Value, operand, field) {
				return true
			}
		}
		return false
	case *exprpb.Expr_ComprehensionExpr:
		c := e.ComprehensionExpr
		if c == nil {
			return false
		}
		return hasSelectExp(c.IterRange, operand, field) ||
			hasSelectExp(c.AccuInit, operand, field) ||
			hasSelectExp(c.LoopCondition, operand, field) ||
			hasSelectExp(c.LoopStep, operand, field) ||
			hasSelectExp(c.Result, operand, field)
	default:
		return false
	}
}

func isIdentOperand(exp *exprpb.Expr, operand string) bool {
	if exp == nil || len(operand) == 0 {
		return false
	}
	id := exp.GetIdentExpr()
	return id != nil && id.Name == operand
}

func isConstField(exp *exprpb.Expr, field string) bool {
	if exp == nil || len(field) == 0 {
		return false
	}
	c := exp.GetConstExpr()
	return c != nil && c.GetStringValue() == field
}
