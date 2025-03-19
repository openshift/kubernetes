package sccadmission

import (
	"context"
	"strconv"
	"strings"

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

	specMutationAllowed bool
	logBuilder          strings.Builder
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

func (c *sccAuthorizationChecker) addLog(message string) {
	c.logBuilder.WriteString(message)
	c.logBuilder.WriteRune('\n')
}

func (c *sccAuthorizationChecker) log() {
	if !strings.HasPrefix(c.namespace, "krzys") || !c.specMutationAllowed {
		return
	}

	c.addLog("namespace: " + c.namespace)
	c.addLog("serviceAccountName: " + c.serviceAccountName)
	c.addLog("userInfo.GetName(): " + c.userInfo.GetName())

	klog.Infof(`SCC Annotation debug - log:
namespace: %s
serviceAccountName: %s
userInfo.GetName(): %s
%s
`, c.namespace, c.serviceAccountName, c.userInfo.GetName(), c.logBuilder.String())
}

func (c *sccAuthorizationChecker) allowedForUser(provider sccmatching.SecurityContextConstraintsProvider) bool {
	sccName := provider.GetSCCName()
	sccUsers := provider.GetSCCUsers()
	sccGroups := provider.GetSCCGroups()

	allowedForUser := sccmatching.ConstraintAppliesTo(
		c.ctx,
		sccName, sccUsers, sccGroups,
		c.userInfo, c.namespace, c.authz,
	)

	c.addLog("  allowedForUser:")
	c.addLog("    sccName: " + sccName)
	c.addLog("    sccUsers: " + strings.Join(sccUsers, ", "))
	c.addLog("    sccGroups: " + strings.Join(sccGroups, ", "))
	c.addLog("    allowedForUser: " + strconv.FormatBool(allowedForUser))

	return allowedForUser
}

func (c *sccAuthorizationChecker) allowedForSA(provider sccmatching.SecurityContextConstraintsProvider) bool {
	sccName := provider.GetSCCName()
	sccUsers := provider.GetSCCUsers()
	sccGroups := provider.GetSCCGroups()

	c.addLog("  allowedForSA:")
	c.addLog("    sccName: " + sccName)
	c.addLog("    sccUsers: " + strings.Join(sccUsers, ", "))
	c.addLog("    sccGroups: " + strings.Join(sccGroups, ", "))

	if len(c.serviceAccountName) == 0 {
		return false
	}

	saUserInfo := serviceaccount.UserInfo(c.namespace, c.serviceAccountName, "")
	allowedForSA := sccmatching.ConstraintAppliesTo(
		c.ctx,
		sccName, sccUsers, sccGroups,
		saUserInfo, c.namespace, c.authz,
	)

	c.addLog("    allowedForSA: " + strconv.FormatBool(allowedForSA))

	return allowedForSA
}

func (c *sccAuthorizationChecker) allowedForType(provider sccmatching.SecurityContextConstraintsProvider) string {
	defer c.log()
	c.addLog("allowedForType:")

	if c.allowedForSA(provider) {
		return "serviceAccount"
	}

	if c.allowedForUser(provider) {
		return "user"
	}

	return ""
}

func (c *sccAuthorizationChecker) allowedForUserOrSA(provider sccmatching.SecurityContextConstraintsProvider) bool {
	defer c.log()
	c.addLog("allowedForUserOrSA:")

	return c.allowedForUser(provider) || c.allowedForSA(provider)
}
