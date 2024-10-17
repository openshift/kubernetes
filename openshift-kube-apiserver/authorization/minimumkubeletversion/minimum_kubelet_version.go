package minimumkubeletversion

import (
	"context"
	"fmt"
	"strings"

	"github.com/blang/semver/v4"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authorization/authorizer"
	corev1informers "k8s.io/client-go/informers/core/v1"
	api "k8s.io/kubernetes/pkg/apis/core"
	"k8s.io/kubernetes/pkg/auth/nodeidentifier"
)

var minimumKubeletVersion *semver.Version

func SetMinimumKubeletVersion(version string) {
	if version == "" {
		return
	}
	v := semver.MustParse(version)
	minimumKubeletVersion = &v
}

type minimumKubeletVersionAuth struct {
	nodeIdentifier nodeidentifier.NodeIdentifier
	nodeInformer   corev1informers.NodeInformer
}

func NewMinimumKubeletVersion(nodeIdentifier nodeidentifier.NodeIdentifier,
	nodeInformer corev1informers.NodeInformer,
) authorizer.Authorizer {

	return &minimumKubeletVersionAuth{
		nodeIdentifier: nodeIdentifier,
		nodeInformer:   nodeInformer,
	}
}

func (m *minimumKubeletVersionAuth) Authorize(ctx context.Context, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	if minimumKubeletVersion == nil {
		return authorizer.DecisionNoOpinion, "", nil
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

	node, err := m.nodeInformer.Lister().Get(nodeName)
	if err != nil {
		return authorizer.DecisionNoOpinion, fmt.Sprintf("failed to get node %s: %v", nodeName, err), nil
	}

	tooOld, errStr := IsKubeletVersionTooOld(node, minimumKubeletVersion)
	if tooOld {
		return authorizer.DecisionDeny, errStr, nil
	}

	return authorizer.DecisionNoOpinion, errStr, nil
}

func IsKubeletVersionTooOld(node *corev1.Node, minVersion *semver.Version) (bool, string) {
	version, err := semver.Parse(strings.TrimPrefix(node.Status.NodeInfo.KubeletVersion, "v"))
	if err != nil {
		return false, fmt.Sprintf("failed to parse node version %s: %v", node.Status.NodeInfo.KubeletVersion, err)
	}

	version.Pre = nil
	version.Build = nil

	name := node.ObjectMeta.Name
	if minVersion.GT(version) {
		return true, fmt.Sprintf("kubelet version of node %s is %v, which is lower than minimumKubeletVersion of %v", name, version, *minVersion)
	}
	return false, ""
}
