package minimumkubeletversion

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	listersv1 "k8s.io/client-go/listers/core/v1"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/auth/nodeidentifier"
)

type minimumKubeletVersion struct {
	nodeIdentifier nodeidentifier.NodeIdentifier
	nodeLister     listersv1.NodeLister
}

func NewMinimumKubeletVersion(nodeIdentifier nodeidentifier.NodeIdentifier,
	nodeLister listersv1.NodeLister,
) authorizer.Authorizer {

	return &minimumKubeletVersion{
		nodeIdentifier: nodeIdentifier,
		nodeLister:     nodeLister,
	}
}

func (m *minimumKubeletVersion) Authorize(ctx context.Context, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	user := attrs.GetUser()
	if user == nil {
		return authorizer.DecisionNoOpinion, "", fmt.Errorf("user missing from context")
	}

	nodeName, isNode := m.nodeIdentifier.NodeIdentity(attrs.GetUser())
	if !isNode {
		// ignore requests from non-nodes
		return authorizer.DecisionNoOpinion, "", nil
	}

	if len(nodeName) == 0 {
		return authorizer.DecisionNoOpinion, fmt.Sprintf("unknown node for user %q", attrs.GetUser().GetName()), nil
	}

	// Short-circut if "subjectaccessreviews", or a "get" or "update" on the node object.
	// Regardless of kubelet version, it should be allowed to do these things.
	if attrs.IsResourceRequest() {
		requestResource := schema.GroupResource{Group: attrs.GetAPIGroup(), Resource: attrs.GetResource()}
		switch requestResource {
		case api.Resource("nodes"):
			if v := attrs.GetVerb(); v == "get" || v == "update" {
				return authorizer.DecisionNoOpinion, "", nil
			}
		// TODO(haircommander): do we need other flavors of access reviews here?
		case api.Resource("subjectaccessreviews"):
			return authorizer.DecisionNoOpinion, "", nil
		}
	}

	node, err := m.nodeLister.Get(nodeName)
	if err != nil {
		return authorizer.DecisionNoOpinion, fmt.Sprintf("failed to get node %s: %v", nodeName, err), nil
	}

	version, err := semver.Parse(strings.TrimPrefix(node.Status.NodeInfo.KubeletVersion, "v"))
	if err != nil {
		return authorizer.DecisionNoOpinion, fmt.Sprintf("failed to parse node version %s: %v", node.Status.NodeInfo.KubeletVersion, err), nil
	}

	version.Pre = nil
	version.Build = nil
	// TODO(haircommander: get this value legitimately
	minVersion := semver.MustParse("1.30.0")

	if minVersion.GT(version) {
		// Specifically deny here because the NodeAuthorizer may have already approved
		return authorizer.DecisionDeny, fmt.Sprintf("kubelet version of node %s is %v, which is lower than minimumKubeletVersion of %v", nodeName, version, minVersion), nil
	}

	return authorizer.DecisionNoOpinion, "", nil
}
