package admission

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// we patch this method so that we can mutate the returned podspec to indicate the pod that will exist AFTER SCC mutation
var extractPodSpecFromTemplate = func(obj runtime.Object, template *corev1.PodTemplateSpec) (*metav1.ObjectMeta, *corev1.PodSpec, error) {
	return ExtractPodSpecFromTemplate_original(template)
}

func SetExtractPodSpecFromTemplate(fn func(obj runtime.Object, template *corev1.PodTemplateSpec) (*metav1.ObjectMeta, *corev1.PodSpec, error)) {
	extractPodSpecFromTemplate = fn
}
