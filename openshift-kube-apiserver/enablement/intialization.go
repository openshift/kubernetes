package enablement

import (
	"context"
	"fmt"
	"io/ioutil"
	"path"

	configv1 "github.com/openshift/api/config/v1"
	kubecontrolplanev1 "github.com/openshift/api/kubecontrolplane/v1"
	osinv1 "github.com/openshift/api/osin/v1"
	"github.com/openshift/apiserver-library-go/pkg/securitycontextconstraints/sccadmission"
	"github.com/openshift/library-go/pkg/config/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/authentication/serviceaccount"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"
	aggregatorapiserver "k8s.io/kube-aggregator/pkg/apiserver"
	"k8s.io/kubernetes/openshift-kube-apiserver/configdefault"
	"k8s.io/kubernetes/pkg/apis/core"
	v1 "k8s.io/kubernetes/pkg/apis/core/v1"
	"k8s.io/kubernetes/pkg/capabilities"
	kubelettypes "k8s.io/kubernetes/pkg/kubelet/types"
	"k8s.io/kubernetes/plugin/pkg/auth/authorizer/rbac/bootstrappolicy"
	podsecurityadmission "k8s.io/pod-security-admission/admission"
)

func GetOpenshiftConfig(openshiftConfigFile string) (*kubecontrolplanev1.KubeAPIServerConfig, error) {
	// try to decode into our new types first.  right now there is no validation, no file path resolution.  this unsticks the operator to start.
	// TODO add those things
	configContent, err := ioutil.ReadFile(openshiftConfigFile)
	if err != nil {
		return nil, err
	}
	scheme := runtime.NewScheme()
	utilruntime.Must(kubecontrolplanev1.Install(scheme))
	codecs := serializer.NewCodecFactory(scheme)
	obj, err := runtime.Decode(codecs.UniversalDecoder(kubecontrolplanev1.GroupVersion, configv1.GroupVersion, osinv1.GroupVersion), configContent)
	if err != nil {

		return nil, err
	}

	// Resolve relative to CWD
	absoluteConfigFile, err := api.MakeAbs(openshiftConfigFile, "")
	if err != nil {
		return nil, err
	}
	configFileLocation := path.Dir(absoluteConfigFile)

	config := obj.(*kubecontrolplanev1.KubeAPIServerConfig)
	if err := helpers.ResolvePaths(configdefault.GetKubeAPIServerConfigFileReferences(config), configFileLocation); err != nil {
		return nil, err
	}
	configdefault.SetRecommendedKubeAPIServerConfigDefaults(config)
	configdefault.ResolveDirectoriesForSATokenVerification(config)

	return config, nil
}

func ForceGlobalInitializationForOpenShift() {
	// This allows to move crqs, sccs, and rbrs to CRD
	aggregatorapiserver.AddAlwaysLocalDelegateForPrefix("/apis/quota.openshift.io/v1/clusterresourcequotas")
	aggregatorapiserver.AddAlwaysLocalDelegateForPrefix("/apis/security.openshift.io/v1/securitycontextconstraints")
	aggregatorapiserver.AddAlwaysLocalDelegateForPrefix("/apis/authorization.openshift.io/v1/rolebindingrestrictions")
	aggregatorapiserver.AddAlwaysLocalDelegateGroupResource(schema.GroupResource{Group: "authorization.openshift.io", Resource: "rolebindingrestrictions"})

	// This allows the CRD registration to avoid fighting with the APIService from the operator
	aggregatorapiserver.AddOverlappingGroupVersion(schema.GroupVersion{Group: "authorization.openshift.io", Version: "v1"})

	// Allow privileged containers
	capabilities.Initialize(capabilities.Capabilities{
		AllowPrivileged: true,
		PrivilegedSources: capabilities.PrivilegedSources{
			HostNetworkSources: []string{kubelettypes.ApiserverSource, kubelettypes.FileSource},
			HostPIDSources:     []string{kubelettypes.ApiserverSource, kubelettypes.FileSource},
			HostIPCSources:     []string{kubelettypes.ApiserverSource, kubelettypes.FileSource},
		},
	})

	podsecurityadmission.SetExtractPodSpecFromTemplate(extractPodSpecFromTemplateWithSCCMutation)

	// add permissions we require on our kube-apiserver
	// TODO, we should scrub these out
	bootstrappolicy.ClusterRoles = bootstrappolicy.OpenshiftClusterRoles
	bootstrappolicy.ClusterRoleBindings = bootstrappolicy.OpenshiftClusterRoleBindings

	// we need to have the authorization chain place something before system:masters
	// SkipSystemMastersAuthorizer disable implicitly added system/master authz, and turn it into another authz mode "SystemMasters", to be added via authorization-mode
	server.SkipSystemMastersAuthorizer()
}

//_ = initializer.WantsAuthorizer(&constraint{})
//_ = initializer.WantsExternalKubeClientSet(&constraint{})
//_ = WantsSecurityInformer(&constraint{})
var SCCAdmissionPlugin = sccadmission.NewConstraint()

// we patch this method so that we can mutate the returned podspec to indicate the pod that will exist AFTER SCC mutation
// we do this by taking the external pod, converting to internal, building a synthetic podsecurityadmission attributes, passing to SCC,
// checking for error and returning the mutated podspec.
func extractPodSpecFromTemplateWithSCCMutation(obj runtime.Object, template *corev1.PodTemplateSpec) (*metav1.ObjectMeta, *corev1.PodSpec, error) {

	objectMeta, originalPodSpec, err := podsecurityadmission.ExtractPodSpecFromTemplate_original(template)
	if err != nil {
		return objectMeta, originalPodSpec, err
	}
	if originalPodSpec == nil {
		return nil, nil, nil
	}
	containerMetadata, err := meta.Accessor(obj)
	if err != nil {
		return objectMeta, originalPodSpec, fmt.Errorf("unable to get metadata for SCC mutation: %w", err)
	}

	pod := &corev1.Pod{
		ObjectMeta: *objectMeta.DeepCopy(),
		Spec:       *originalPodSpec.DeepCopy(),
	}
	if len(pod.Namespace) == 0 {
		pod.Namespace = containerMetadata.GetNamespace()
	}
	if len(pod.Name) == 0 {
		pod.Name = "pod-for-container-named-" + containerMetadata.GetName()
	}
	internalPod := &core.Pod{}
	if err := v1.Convert_v1_Pod_To_core_Pod(pod, internalPod, nil); err != nil {
		return nil, nil, err
	}

	admissionAttributes := admission.NewAttributesRecord(
		internalPod,
		nil,
		corev1.SchemeGroupVersion.WithKind("Pod"),
		pod.Namespace,
		pod.Name,
		corev1.SchemeGroupVersion.WithResource("pods"),
		"",
		admission.Create,
		nil,
		false,
		&user.DefaultInfo{
			Name:   serviceaccount.MakeUsername(pod.Namespace, pod.Spec.ServiceAccountName),
			UID:    "",
			Groups: append([]string{user.AllAuthenticated}, serviceaccount.MakeGroupNames(pod.Namespace)...),
			Extra:  nil,
		})
	if err := SCCAdmissionPlugin.Admit(context.Background(), admissionAttributes, nil); err != nil {
		// don't fail the request, just warn if SCC will fail
		klog.ErrorS(err, "failed to mutate object for PSA using SCC")
		utilruntime.HandleError(fmt.Errorf("failed to mutate object for PSA using SCC: %w", err))
		// TODO remove this failure we're causing when SCC fails, but for now we actually need to see our test fail because that was almost really bad.
		return objectMeta, originalPodSpec, err
	}

	if err := v1.Convert_core_Pod_To_v1_Pod(internalPod, pod, nil); err != nil {
		return nil, nil, err
	}

	return objectMeta, &pod.Spec, nil
}
