package sccadmission

import (
	"context"

	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	"k8s.io/klog/v2"

	"github.com/openshift/apiserver-library-go/pkg/securitycontextconstraints/sccmatching"
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

	sccName := provider.GetSCCName()
	sccUsers := provider.GetSCCUsers()
	sccGroups := provider.GetSCCGroups()

	// Detailed logging of inputs
	klog.Infof("SCC debug [%s] - Checking authorization for:", sccName)
	klog.Infof("SCC debug [%s] - UserInfo: Name=%s, Groups=%v", sccName, c.userInfo.GetName(), c.userInfo.GetGroups())
	klog.Infof("SCC debug [%s] - ServiceAccount: Namespace=%s, Name=%s", sccName, c.namespace, c.serviceAccountName)
	klog.Infof("SCC debug [%s] - SCC config: Users=%v, Groups=%v", sccName, sccUsers, sccGroups)

	// Debug SA check
	saUserInfo := serviceaccount.UserInfo(c.namespace, c.serviceAccountName, "")
	klog.Infof("SCC debug [%s] - SA UserInfo: Name=%s, Groups=%v", sccName, saUserInfo.GetName(), saUserInfo.GetGroups())

	// Now check actual authorization
	saAllowed := c.allowedForServiceAccount(ctx, provider)
	userAllowed := c.allowedForUser(ctx, provider)

	klog.Infof("SCC debug [%s] - Authorization results: SA auth check=%v, User auth check=%v",
		sccName, saAllowed, userAllowed)

	if saAllowed {
		klog.Infof("SCC debug [%s] - Selecting 'serviceaccount' as the subject type", sccName)
		return serviceAccount
	}

	if userAllowed {
		klog.Infof("SCC debug [%s] - Selecting 'user' as the subject type", sccName)
		return user
	}

	klog.Infof("SCC debug [%s] - No authorization found for either subject type", sccName)

	return ""
}

func (c *sccAuthorizationChecker) allowedForUserOrSA(ctx context.Context, provider sccmatching.SecurityContextConstraintsProvider) bool {
	return c.allowedForUser(ctx, provider) || c.allowedForServiceAccount(ctx, provider)
}
