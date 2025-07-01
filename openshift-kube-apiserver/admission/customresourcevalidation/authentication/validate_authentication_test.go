package authentication

import (
	"context"
	"errors"
	"testing"
	"time"

	configv1 "github.com/openshift/api/config/v1"
	"golang.org/x/sync/singleflight"
	"k8s.io/apimachinery/pkg/util/validation/field"
	authenticationcel "k8s.io/apiserver/pkg/authentication/cel"
	"k8s.io/utils/lru"
)

func TestFailValidateAuthenticationSpec(t *testing.T) {
	errorCases := map[string]struct {
		spec       configv1.AuthenticationSpec
		errorType  field.ErrorType
		errorField string
	}{
		"invalid metadata ref": {
			spec: configv1.AuthenticationSpec{
				Type: "",
				OAuthMetadata: configv1.ConfigMapNameReference{
					Name: "../shadow",
				},
			},
			errorType:  field.ErrorTypeInvalid,
			errorField: "spec.oauthMetadata.name",
		},
		"invalid webhook ref": {
			spec: configv1.AuthenticationSpec{
				WebhookTokenAuthenticator: &configv1.WebhookTokenAuthenticator{
					KubeConfig: configv1.SecretNameReference{Name: "this+that"},
				},
			},
			errorType:  field.ErrorTypeInvalid,
			errorField: "spec.webhookTokenAuthenticator.kubeConfig.name",
		},
		"valid webhook ref": {
			spec: configv1.AuthenticationSpec{
				WebhookTokenAuthenticator: &configv1.WebhookTokenAuthenticator{
					KubeConfig: configv1.SecretNameReference{Name: "this"},
				},
			},
		},
		"invalid webhook ref for a Type": {
			spec: configv1.AuthenticationSpec{
				Type: "OIDC",
				WebhookTokenAuthenticator: &configv1.WebhookTokenAuthenticator{
					KubeConfig: configv1.SecretNameReference{Name: "this"},
				},
			},
			errorType:  field.ErrorTypeInvalid,
			errorField: "spec.webhookTokenAuthenticator",
		},
		"invalid UID CEL expression": {
			spec: configv1.AuthenticationSpec{
				Type: "OIDC",
				OIDCProviders: []configv1.OIDCProvider{
					{
						ClaimMappings: configv1.TokenClaimMappings{
							UID: &configv1.TokenClaimOrExpressionMapping{
								Expression: "!@^#&(!^@(*#&(",
							},
						},
					},
				},
			},
			errorType:  field.ErrorTypeInvalid,
			errorField: "spec.oidcProviders[0].claimMappings.uid.expression",
		},
		"invalid Extra CEL expression": {
			spec: configv1.AuthenticationSpec{
				Type: "OIDC",
				OIDCProviders: []configv1.OIDCProvider{
					{
						ClaimMappings: configv1.TokenClaimMappings{
							Extra: []configv1.ExtraMapping{
								{
									Key:             "foo/bar",
									ValueExpression: "!@*(&#^(!@*)&^&",
								},
							},
						},
					},
				},
			},
			errorType:  field.ErrorTypeInvalid,
			errorField: "spec.oidcProviders[0].claimMappings.extra[0].valueExpression",
		},
	}

	for tcName, tc := range errorCases {
		errs, _ := validateAuthenticationSpec(context.TODO(), tc.spec, &celStore{
			compiler:       authenticationcel.NewDefaultCompiler(),
			compilingGroup: new(singleflight.Group),
			compiledStore:  lru.New(100),
		})
		if (len(errs) > 0) != (len(tc.errorType) != 0) {
			t.Errorf("'%s': expected failure: %t, got: %t", tcName, len(tc.errorType) != 0, len(errs) > 0)
		}

		for _, e := range errs {
			if e.Type != tc.errorType {
				t.Errorf("'%s': expected errors of type '%s', got %v:", tcName, tc.errorType, e)
			}

			if e.Field != tc.errorField {
				t.Errorf("'%s': expected errors in field '%s', got %v:", tcName, tc.errorField, e)
			}
		}
	}
}

func TestSucceedValidateAuthenticationSpec(t *testing.T) {
	successCases := map[string]configv1.AuthenticationSpec{
		"integrated oauth authn type": {
			Type: "IntegratedOAuth",
		},
		"_none_ authn type": {
			Type: "None",
		},
		"empty authn type": {
			Type: "",
		},
		"integrated oauth + oauth metadata": {
			OAuthMetadata: configv1.ConfigMapNameReference{
				Name: "configmapwithmetadata",
			},
		},
		"webhook set": {
			WebhookTokenAuthenticators: []configv1.DeprecatedWebhookTokenAuthenticator{
				{KubeConfig: configv1.SecretNameReference{Name: "wheniwaslittleiwantedtobecomeawebhook"}},
			},
		},
		"some webhooks": {
			WebhookTokenAuthenticators: []configv1.DeprecatedWebhookTokenAuthenticator{
				{KubeConfig: configv1.SecretNameReference{Name: "whatacoolnameforasecret"}},
				{KubeConfig: configv1.SecretNameReference{Name: "whatacoolnameforasecret2"}},
				{KubeConfig: configv1.SecretNameReference{Name: "thisalsoisacoolname"}},
				{KubeConfig: configv1.SecretNameReference{Name: "letsnotoverdoit"}},
			},
		},
		"all fields set": {
			Type: "IntegratedOAuth",
			OAuthMetadata: configv1.ConfigMapNameReference{
				Name: "suchname",
			},
			WebhookTokenAuthenticators: []configv1.DeprecatedWebhookTokenAuthenticator{
				{KubeConfig: configv1.SecretNameReference{Name: "thisisawebhook"}},
				{KubeConfig: configv1.SecretNameReference{Name: "thisisawebhook2"}},
				{KubeConfig: configv1.SecretNameReference{Name: "thisisawebhook33"}},
			},
		},
		"valid uid CEL expression": {
			Type: "OIDC",
			OIDCProviders: []configv1.OIDCProvider{
				{
					ClaimMappings: configv1.TokenClaimMappings{
						UID: &configv1.TokenClaimOrExpressionMapping{
							Expression: "claims.uid",
						},
					},
				},
			},
		},
		"valid Extra CEL expression": {
			Type: "OIDC",
			OIDCProviders: []configv1.OIDCProvider{
				{
					ClaimMappings: configv1.TokenClaimMappings{
						Extra: []configv1.ExtraMapping{
							{
								Key:             "foo/bar",
								ValueExpression: "claims.roles",
							},
						},
					},
				},
			},
		},
	}

	for tcName, s := range successCases {
		errs, _ := validateAuthenticationSpec(context.TODO(), s, &celStore{
			compiler:       authenticationcel.NewDefaultCompiler(),
			compilingGroup: new(singleflight.Group),
			compiledStore:  lru.New(100),
		})
		if len(errs) != 0 {
			t.Errorf("'%s': expected success, but failed: %v", tcName, errs.ToAggregate().Error())
		}
	}
}

func TestFailValidateAuthenticationStatus(t *testing.T) {
	errorCases := map[string]struct {
		status     configv1.AuthenticationStatus
		errorType  field.ErrorType
		errorField string
	}{
		"wrong reference name": {
			status: configv1.AuthenticationStatus{
				IntegratedOAuthMetadata: configv1.ConfigMapNameReference{
					Name: "something_wrong",
				},
			},
			errorType:  field.ErrorTypeInvalid,
			errorField: "status.integratedOAuthMetadata.name",
		},
	}

	for tcName, tc := range errorCases {
		errs := validateAuthenticationStatus(tc.status)
		if len(errs) == 0 {
			t.Errorf("'%s': should have failed but did not", tcName)
		}

		for _, e := range errs {
			if e.Type != tc.errorType {
				t.Errorf("'%s': expected errors of type '%s', got %v:", tcName, tc.errorType, e)
			}

			if e.Field != tc.errorField {
				t.Errorf("'%s': expected errors in field '%s', got %v:", tcName, tc.errorField, e)
			}
		}
	}
}

func TestSucceedValidateAuthenticationStatus(t *testing.T) {
	successCases := map[string]configv1.AuthenticationStatus{
		"basic case": {
			IntegratedOAuthMetadata: configv1.ConfigMapNameReference{
				Name: "hey-there",
			},
		},
		"empty reference": {
			IntegratedOAuthMetadata: configv1.ConfigMapNameReference{
				Name: "",
			},
		},
		"empty status": {},
	}

	for tcName, s := range successCases {
		errs := validateAuthenticationStatus(s)
		if len(errs) != 0 {
			t.Errorf("'%s': expected success, but failed: %v", tcName, errs.ToAggregate().Error())
		}
	}
}

func TestValidateCELExpression(t *testing.T) {
	type testcase struct {
		name       string
		cel        func() *celStore
		ctx        func() context.Context
		shouldErr  bool
		shouldWarn bool
	}

	expression := &authenticationcel.ClaimMappingExpression{
		Expression: `["foo", "bar"].exists(x, x == "foo")`,
	}

	testcases := []testcase{
		{
			name: "does not return a warning when compilation takes less than a second",
			cel: func() *celStore {
				return &celStore{
					compiler: &mockCompiler{
						delay: 200 * time.Millisecond,
						err:   nil,
					},
					compilingGroup: new(singleflight.Group),
					compiledStore:  lru.New(1),
				}
			},
			ctx: func() context.Context { return context.TODO() },
		},
		{
			name: "returns a warning when compilation takes longer than a second",
			cel: func() *celStore {
				return &celStore{
					compiler: &mockCompiler{
						delay: 1500 * time.Millisecond,
						err:   nil,
					},
					compilingGroup: new(singleflight.Group),
					compiledStore:  lru.New(1),
				}
			},
			ctx:        func() context.Context { return context.TODO() },
			shouldWarn: true,
		},
		{
			name: "still returns error if compilation takes longer than a second and errors out",
			cel: func() *celStore {
				return &celStore{
					compiler: &mockCompiler{
						delay: 1500 * time.Millisecond,
						err:   errors.New("boom"),
					},
					compilingGroup: new(singleflight.Group),
					compiledStore:  lru.New(1),
				}
			},
			ctx:        func() context.Context { return context.TODO() },
			shouldWarn: true,
			shouldErr:  true,
		},
		{
			name: "returns an error if the context has been canceled",
			cel: func() *celStore {
				return &celStore{
					compiler: &mockCompiler{
						delay: 1500 * time.Millisecond,
						err:   nil,
					},
					compilingGroup: new(singleflight.Group),
					compiledStore:  lru.New(1),
				}
			},
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.TODO())
				cancel()
				return ctx
			},
			shouldErr: true,
		},
		{
			name: "waits for already compiling expression to finish compiling and returns its results",
			cel: func() *celStore {
				compGroup := new(singleflight.Group)

				_ = compGroup.DoChan(expression.Expression, func() (interface{}, error) {
					// Hog the group for a bit
					time.Sleep(time.Second)

					return compileResult{
						err:  errors.New("boom"),
						warn: "warning",
					}, nil
				})

				return &celStore{
					compiler:       nil, // should never end up calling this
					compilingGroup: compGroup,
					compiledStore:  lru.New(1),
				}
			},
			ctx:        func() context.Context { return context.TODO() },
			shouldErr:  true,
			shouldWarn: true,
		},
		{
			name: "returns already compiled expression results if the expression has been compiled before",
			cel: func() *celStore {
				compiledLRU := lru.New(1)
				res := compileResult{
					err:  errors.New("boom"),
					warn: "warning",
				}
				compiledLRU.Add(expression.Expression, res)

				return &celStore{
					compiler:       nil, // should never end up calling this
					compilingGroup: new(singleflight.Group),
					compiledStore:  compiledLRU,
				}
			},
			ctx:        func() context.Context { return context.TODO() },
			shouldErr:  true,
			shouldWarn: true,
		},
		{
			name: "handles panic in compilation goroutine",
			cel: func() *celStore {
				return &celStore{
					compiler:       nil, // causes panic
					compilingGroup: new(singleflight.Group),
					compiledStore:  lru.New(1),
				}
			},
			ctx:       func() context.Context { return context.TODO() },
			shouldErr: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err, warn := validateCELExpression(tc.ctx(), tc.cel(), expression)

			if tc.shouldErr != (err != nil) {
				t.Fatalf("error expectation does not match actual. expected? %v . received: %v", tc.shouldErr, err)
			}

			if tc.shouldWarn != (len(warn) > 0) {
				t.Fatalf("warning expectation does not match actual. expected? %v . received: %s", tc.shouldErr, warn)
			}
		})
	}
}

type mockCompiler struct {
	delay time.Duration
	err   error
}

func (mc *mockCompiler) CompileClaimsExpression(expressionAccessor authenticationcel.ExpressionAccessor) (authenticationcel.CompilationResult, error) {
	time.Sleep(mc.delay)
	return authenticationcel.CompilationResult{}, mc.err
}

func (mc *mockCompiler) CompileUserExpression(expressionAccessor authenticationcel.ExpressionAccessor) (authenticationcel.CompilationResult, error) {
	time.Sleep(mc.delay)
	return authenticationcel.CompilationResult{}, mc.err
}
