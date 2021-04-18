package deprecatedapirequest

import (
	"fmt"
	"strings"
	"testing"

	openshiftapi "github.com/openshift/api"
	"github.com/stretchr/testify/assert"
	apiextentions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/install"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	flowcontrol "k8s.io/kubernetes/pkg/apis/flowcontrol/install"
	netowrking "k8s.io/kubernetes/pkg/apis/networking/install"
)

func TestDeprecatedAPIRemovedRelease(t *testing.T) {

	// utiltiy function
	gvkToGVR := func(kind schema.GroupVersionKind) schema.GroupVersionResource {
		// TODO do this properly
		gvr := schema.GroupVersionResource{
			Group:    kind.Group,
			Version:  kind.Version,
			Resource: strings.ToLower(kind.Kind),
		}
		switch {
		case gvr.Resource == "ingress":
			gvr.Resource = "ingresses"
		case !strings.HasSuffix(gvr.Resource, "list"):
			gvr.Resource = gvr.Resource + "s"
		}
		return gvr
	}

	// install types
	scheme := runtime.NewScheme()
	openshiftapi.Install(scheme)
	openshiftapi.InstallKube(scheme)
	apiextentions.Install(scheme)
	flowcontrol.Install(scheme)
	netowrking.Install(scheme)

	// copy the deprecatedApiRemovedRelease map
	copyOfMap := map[schema.GroupVersionResource]string{}
	for k, v := range deprecatedApiRemovedRelease {
		copyOfMap[k] = v
	}

	for gvk, _ := range scheme.AllKnownTypes() {
		obj, err := scheme.New(gvk)
		assert.NoError(t, err)
		deprecatedObj, ok := obj.(interface{ APILifecycleRemoved() (int, int) })
		if !ok {
			continue
		}
		// asset map contains expected entry
		gvr := gvkToGVR(gvk)
		assert.Contains(t, copyOfMap, gvr)
		major, minor := deprecatedObj.APILifecycleRemoved()
		assert.Equal(t, fmt.Sprintf("%d.%d", major, minor), copyOfMap[gvr])
		delete(copyOfMap, gvr)
	}

	// assert map didn't have any extra entries
	assert.Empty(t, copyOfMap)

	if t.Failed() {
		t.Log("NOTE: run `go generate ./openshift-kube-apiserver/...` to re-generate deprecated_map.go")
	}

}
