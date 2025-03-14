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
	ctx                context.Context
	authz              authorizer.Authorizer
	userInfo           user.Info
	namespace          string
	serviceAccountName string
}

func newSCCAuthorizerChecker(ctx context.Context, authz authorizer.Authorizer, attr admission.Attributes, serviceAccountName string) *sccAuthorizationChecker {
	return &sccAuthorizationChecker{
		ctx:                ctx,
		authz:              authz,
		userInfo:           attr.GetUserInfo(),
		namespace:          attr.GetNamespace(),
		serviceAccountName: serviceAccountName,
	}
}

func (c *sccAuthorizationChecker) allowedForUser(provider sccmatching.SecurityContextConstraintsProvider) bool {
	sccName := provider.GetSCCName()
	sccUsers := provider.GetSCCUsers()
	sccGroups := provider.GetSCCGroups()

	return sccmatching.ConstraintAppliesTo(
		c.ctx,
		sccName, sccUsers, sccGroups,
		c.userInfo, c.namespace, c.authz,
	)
}

func (c *sccAuthorizationChecker) allowedForSA(provider sccmatching.SecurityContextConstraintsProvider) bool {
	sccName := provider.GetSCCName()
	sccUsers := provider.GetSCCUsers()
	sccGroups := provider.GetSCCGroups()

	klog.Infof(
		"SCC Annotation debug - sccName: %q, sccUsers: %v, sccGroups: %v, serviceAccountName: %q",
		sccName, sccUsers, sccGroups, c.serviceAccountName,
	)

	if len(c.serviceAccountName) == 0 {
		klog.Infof("SCC Annotation debug - serviceAccountName is empty")

		return false
	}

	saUserInfo := serviceaccount.UserInfo(c.namespace, c.serviceAccountName, "")
	allowedForSA := sccmatching.ConstraintAppliesTo(
		c.ctx,
		sccName, sccUsers, sccGroups,
		saUserInfo, c.namespace, c.authz,
	)

	klog.Infof("SCC Annotation debug - allowedForSA: %v", allowedForSA)

	return allowedForSA
}

func (c *sccAuthorizationChecker) allowedForType(provider sccmatching.SecurityContextConstraintsProvider) string {
	if c.allowedForSA(provider) {
		klog.Infof("SCC Annotation debug - allowedForType: serviceAccount")

		return "serviceAccount"
	}

	if c.allowedForUser(provider) {
		klog.Infof("SCC Annotation debug - allowedForType: user")

		return "user"
	}

	return ""
}

func (c *sccAuthorizationChecker) allowedForUserOrSA(provider sccmatching.SecurityContextConstraintsProvider) bool {
	return c.allowedForUser(provider) || c.allowedForSA(provider)
}
