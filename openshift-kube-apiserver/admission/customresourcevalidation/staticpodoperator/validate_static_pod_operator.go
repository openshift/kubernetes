package staticpodoperator

import (
	"context"
	"fmt"
	"io"
	"reflect"

	operatorv1 "github.com/openshift/api/operator/v1"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/admission"

	"k8s.io/kubernetes/openshift-kube-apiserver/admission/customresourcevalidation"
)

const PluginName = "operator.openshift.io/ValidateStaticPodOperator"

func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
		return customresourcevalidation.NewValidator(
			map[schema.GroupResource]bool{
				operatorv1.Resource("etcds"):                  true,
				operatorv1.Resource("kubeapiservers"):         true,
				operatorv1.Resource("kubecontrollermanagers"): true,
				operatorv1.Resource("kubeschedulers"):         true,
			},
			map[schema.GroupVersionKind]customresourcevalidation.ObjectValidator{
				operatorv1.GroupVersion.WithKind("Etcd"): staticPodOperatorValidator[*operatorv1.Etcd]{
					getStatus: func(o *operatorv1.Etcd) operatorv1.StaticPodOperatorStatus {
						return o.Status.StaticPodOperatorStatus
					},
				},
				operatorv1.GroupVersion.WithKind("KubeAPIServer"): staticPodOperatorValidator[*operatorv1.KubeAPIServer]{
					getStatus: func(o *operatorv1.KubeAPIServer) operatorv1.StaticPodOperatorStatus {
						return o.Status.StaticPodOperatorStatus
					},
				},
				operatorv1.GroupVersion.WithKind("KubeControllerManager"): staticPodOperatorValidator[*operatorv1.KubeControllerManager]{
					getStatus: func(o *operatorv1.KubeControllerManager) operatorv1.StaticPodOperatorStatus {
						return o.Status.StaticPodOperatorStatus
					},
				},
				operatorv1.GroupVersion.WithKind("KubeScheduler"): staticPodOperatorValidator[*operatorv1.KubeScheduler]{
					getStatus: func(o *operatorv1.KubeScheduler) operatorv1.StaticPodOperatorStatus {
						return o.Status.StaticPodOperatorStatus
					},
				},
			})
	})
}

func toKubeControllerManager(uncastObj runtime.Object) (*operatorv1.KubeControllerManager, field.ErrorList) {
	if uncastObj == nil {
		return nil, nil
	}

	allErrs := field.ErrorList{}

	obj, ok := uncastObj.(*operatorv1.KubeControllerManager)
	if !ok {
		return nil, append(allErrs,
			field.NotSupported(field.NewPath("kind"), fmt.Sprintf("%T", uncastObj), []string{"KubeControllerManager"}),
			field.NotSupported(field.NewPath("apiVersion"), fmt.Sprintf("%T", uncastObj), []string{"operator.openshift.io/v1"}))
	}

	return obj, nil
}

type staticPodOperatorValidator[T runtime.Object] struct {
	getStatus func(o T) operatorv1.StaticPodOperatorStatus
}

func (staticPodOperatorValidator[T]) ValidateCreate(_ context.Context, uncastObj runtime.Object) field.ErrorList {
	return nil
}

func (staticPodOperatorValidator[T]) ValidateUpdate(_ context.Context, uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	return nil
}

func (v staticPodOperatorValidator[T]) ValidateStatusUpdate(_ context.Context, uncastObj runtime.Object, uncastOldObj runtime.Object) field.ErrorList {
	obj, ok := uncastObj.(T)
	if !ok {
		panic(fmt.Sprintf("cannot validate object of unexpected type %T (expected %T)", uncastObj, reflect.TypeFor[T]()))
	}
	oldObj, ok := uncastOldObj.(T)
	if !ok {
		panic(fmt.Sprintf("cannot validate object of unexpected type %T (expected %T)", uncastOldObj, reflect.TypeFor[T]()))
	}

	status := v.getStatus(obj)
	oldStatus := v.getStatus(oldObj)

	var errors field.ErrorList
	for _, node := range status.NodeStatuses {
		preexisting := false
		for _, oldNode := range oldStatus.NodeStatuses {
			if node.NodeName == oldNode.NodeName {
				preexisting = true
			}
		}
		if preexisting {
			continue
		}

		if node.CurrentRevision > 0 {
			errors = append(errors, field.Invalid(field.NewPath("status", "nodeStatuses"), node, "new node statuses can not have nonzero currentRevision"))
		}
		if node.TargetRevision > 0 {
			errors = append(errors, field.Invalid(field.NewPath("status", "nodeStatuses"), node, "new node statuses can not have nonzero targetRevision"))
		}
	}

	return errors
}
