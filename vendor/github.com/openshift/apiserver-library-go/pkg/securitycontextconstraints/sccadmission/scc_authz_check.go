package sccadmission

import (
	"context"

	"github.com/openshift/apiserver-library-go/pkg/securitycontextconstraints/sccmatching"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

type sccAuthorizationChecker struct {
	authz              authorizer.Authorizer
	userInfo           user.Info
	namespace          string
	serviceAccountName string
}

func newSCCAuthorizationChecker(authz authorizer.Authorizer, attr admission.Attributes, serviceAccountName string) *sccAuthorizationChecker {
	return &sccAuthorizationChecker{
		authz:              authz,
		userInfo:           attr.GetUserInfo(),
		namespace:          attr.GetNamespace(),
		serviceAccountName: serviceAccountName,
	}
}

func (c *sccAuthorizationChecker) allowedForUser(ctx context.Context, provider sccmatching.SecurityContextConstraintsProvider) bool {
	var (
		sccName   = provider.GetSCCName()
		sccUsers  = provider.GetSCCUsers()
		sccGroups = provider.GetSCCGroups()
	)

	return sccmatching.ConstraintAppliesTo(ctx, sccName, sccUsers, sccGroups, c.userInfo, c.namespace, c.authz)
}

func (c *sccAuthorizationChecker) allowedForServiceAccount(ctx context.Context, provider sccmatching.SecurityContextConstraintsProvider) bool {
	if len(c.serviceAccountName) == 0 {
		return false
	}

	var (
		sccName    = provider.GetSCCName()
		sccUsers   = provider.GetSCCUsers()
		sccGroups  = provider.GetSCCGroups()
		saUserInfo = serviceaccount.UserInfo(c.namespace, c.serviceAccountName, "")
	)

	return sccmatching.ConstraintAppliesTo(ctx, sccName, sccUsers, sccGroups, saUserInfo, c.namespace, c.authz)
}

func (c *sccAuthorizationChecker) allowedFor(ctx context.Context, provider sccmatching.SecurityContextConstraintsProvider) string {
	const (
		serviceAccount = "serviceaccount"
		user           = "user"
	)

	// ServiceAccounts have a higher priority than a user, as they indicate that the workloads is
	// properly set up with the correct permissions for the ServiceAccount. It means that the PSA label
	// syncer will be able to properly label the Namespace with the correct PodSecurityStandard.
	if c.allowedForServiceAccount(ctx, provider) {
		return serviceAccount
	}

	if c.allowedForUser(ctx, provider) {
		return user
	}

	return ""
}

func (c *sccAuthorizationChecker) allowedForUserOrSA(ctx context.Context, provider sccmatching.SecurityContextConstraintsProvider) bool {
	return c.allowedForUser(ctx, provider) || c.allowedForServiceAccount(ctx, provider)
}
