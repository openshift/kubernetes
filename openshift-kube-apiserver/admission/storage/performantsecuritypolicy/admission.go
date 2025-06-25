package performantsecuritypolicy

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/initializer"
	"k8s.io/client-go/informers"
	corev1listers "k8s.io/client-go/listers/core/v1"
	kapi "k8s.io/kubernetes/pkg/apis/core"
)

const (
	// Plugin name
	PluginName               = "storage.openshift.io/PerformantSecurityPolicy"
	fsGroupChangePolicyLabel = "storage.openshift.io/fsgroup-change-policy"
	selinuxChangePolicyLabel = "storage.openshift.io/selinux-change-policy"
)

var _ = initializer.WantsExternalKubeInformerFactory(&performantSecurityPolicy{})
var _ = admission.MutationInterface(&performantSecurityPolicy{})

func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName,
		func(config io.Reader) (admission.Interface, error) {
			return &performantSecurityPolicy{
				Handler: admission.NewHandler(admission.Create),
			}, nil
		})
}

// csiInlineVolSec validates whether the namespace has permission to use a given
// CSI driver as an inline volume.
type performantSecurityPolicy struct {
	*admission.Handler
	//enabled               bool
	//inspectedFeatureGates bool
	nsLister       corev1listers.NamespaceLister
	nsListerSynced func() bool
}

// SetExternalKubeInformerFactory registers an informer
func (c *performantSecurityPolicy) SetExternalKubeInformerFactory(kubeInformers informers.SharedInformerFactory) {
	c.nsLister = kubeInformers.Core().V1().Namespaces().Lister()
	c.nsListerSynced = kubeInformers.Core().V1().Namespaces().Informer().HasSynced
	c.SetReadyFunc(func() bool {
		return c.nsListerSynced()
	})
}

func (c *performantSecurityPolicy) ValidateInitialization() error {
	if c.nsLister == nil {
		return fmt.Errorf("%s plugin needs a namespace lister", PluginName)
	}
	if c.nsListerSynced == nil {
		return fmt.Errorf("%s plugin needs a namespace lister synced", PluginName)
	}
	return nil
}

func (c *performantSecurityPolicy) Admit(ctx context.Context, attributes admission.Attributes, _ admission.ObjectInterfaces) error {
	if attributes.GetResource().GroupResource() != kapi.Resource("pods") ||
		len(attributes.GetSubresource()) > 0 {
		return nil
	}

	pod, ok := attributes.GetObject().(*kapi.Pod)
	if !ok {
		return admission.NewForbidden(attributes, fmt.Errorf("unexpected object: %#v", attributes.GetObject()))
	}

	currentFSGroupChangePolicy := extractCurrentFSGroupChangePolicy(pod)
	ns, err := c.nsLister.Get(pod.Namespace)
	if err != nil {
		return fmt.Errorf("error listing pod namespace: %v", err)
	}

	if currentFSGroupChangePolicy == nil {
		currentFSGroupChangePolicy = c.getDefaultFSGroupChangePolicy(ns)
		pod.Spec.SecurityContext.FSGroupChangePolicy = currentFSGroupChangePolicy
	}

	currentSELinuxChangePolicy := pod.Spec.SecurityContext.SELinuxChangePolicy
	if currentSELinuxChangePolicy == nil {
		currentSELinuxChangePolicy = c.getDefaultSELinuxChangePolicy(ns)
		pod.Spec.SecurityContext.SELinuxChangePolicy = currentSELinuxChangePolicy
	}
	return nil
}

func extractCurrentFSGroupChangePolicy(pod *kapi.Pod) *kapi.PodFSGroupChangePolicy {
	return pod.Spec.SecurityContext.FSGroupChangePolicy
}

func (c *performantSecurityPolicy) getDefaultFSGroupChangePolicy(ns *corev1.Namespace) *kapi.PodFSGroupChangePolicy {
	fsGroupPolicy, ok := ns.Labels[fsGroupChangePolicyLabel]
	if !ok {
		return nil
	}
	return (*kapi.PodFSGroupChangePolicy)(&fsGroupPolicy)
}

func (c *performantSecurityPolicy) getDefaultSELinuxChangePolicy(ns *corev1.Namespace) *kapi.PodSELinuxChangePolicy {
	selinuxChangePolicy, ok := ns.Labels[selinuxChangePolicyLabel]
	if !ok {
		return nil
	}

	return (*kapi.PodSELinuxChangePolicy)(&selinuxChangePolicy)
}
