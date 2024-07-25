package conditiontransitiontimeinjection

import (
	"context"
	"fmt"
	"io"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"strconv"
	"strings"

	"k8s.io/apiserver/pkg/admission"
)

const (
	PluginName                                 = "openshift.io/ConditionTransitionTimeInjector"
	ConditionTransitionTimeInjectionAnnotation = "openshift.io/ConditionTransitionTimeInjection"

	InjectDefault            = ""
	InjectOnStatusChange     = "OnStatusChange"
	InjectOnReasonChange     = "OnReasonChange"
	InjectOnMessageChange    = "OnMessageChange"
	InjectOnGenerationChange = "OnGenerationChange"
)

// Register registers a plugin
func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
		return &mutateConditionTransitionTime{
			Handler: admission.NewHandler(admission.Create, admission.Update),
		}, nil
	})
}

// ValidateCustomResource is an implementation of admission.Interface.
// It looks at all new pods and overrides each container's image pull policy to Always.
type mutateConditionTransitionTime struct {
	*admission.Handler
}

var _ admission.MutationInterface = &mutateConditionTransitionTime{}

// Validate is an admission function that will validate a CRD in config.openshift.io.  uncastAttributes are attributes
// that are of type unstructured.
func (*mutateConditionTransitionTime) Admit(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
	if a.GetSubresource() != "status" {
		return nil
	}
	newObj := a.GetObject()
	if newObj == nil {
		return nil
	}
	newMetadata, err := meta.Accessor(newObj)
	if err != nil {
		return nil
	}
	injectionReasons := getInjectionReasons(newMetadata)
	if len(injectionReasons) == 0 {
		return nil
	}

	oldObj := a.GetOldObject()
	newConditions := getConditions(newObj)
	oldConditions := getConditions(oldObj)
	now := now()

	for i, newCondition := range newConditions {
		oldCondition := meta.FindStatusCondition(oldConditions, newCondition.Type)
		if oldCondition == nil {
			if err := setLastTransitionTime(newObj, i, now); err != nil {
				return fmt.Errorf("unable to inject last transition time for %q: %w", newCondition.Type, err)
			}
			continue
		}

		injectTransitionTime := false
		injectTransitionTime = injectTransitionTime || (injectionReasons.Has(InjectOnStatusChange) && newCondition.Status != oldCondition.Status)
		injectTransitionTime = injectTransitionTime || (injectionReasons.Has(InjectOnGenerationChange) && newCondition.ObservedGeneration != oldCondition.ObservedGeneration)
		injectTransitionTime = injectTransitionTime || (injectionReasons.Has(InjectOnMessageChange) && newCondition.Message != oldCondition.Message)
		injectTransitionTime = injectTransitionTime || (injectionReasons.Has(InjectOnReasonChange) && newCondition.Reason != oldCondition.Reason)

		if !injectTransitionTime {
			// we have an existing old transition time.  Be sure we preserve this.
			if err := setLastTransitionTime(newObj, i, oldCondition.LastTransitionTime); err != nil {
				return fmt.Errorf("unable to re-use last transition time for %q: %w", newCondition.Type, err)
			}
			continue
		}

		if err := setLastTransitionTime(newObj, i, now); err != nil {
			return fmt.Errorf("unable to inject last transition time for %q: %w", newCondition.Type, err)
		}
	}

	return nil
}

// for unit testing
var now func() metav1.Time = metav1.Now

func getInjectionReasons(metadata metav1.Object) sets.Set[string] {
	injectionPolicy, ok := metadata.GetAnnotations()[ConditionTransitionTimeInjectionAnnotation]
	if !ok {
		return nil
	}

	// if set to empty, use the default
	if len(strings.TrimSpace(injectionPolicy)) == 0 {
		return sets.New(InjectOnStatusChange)
	}

	ret := sets.Set[string]{}
	for _, curr := range strings.Split(injectionPolicy, ",") {
		ret.Insert(strings.TrimSpace(curr))
	}
	return ret
}

// coerce as much as we can to metav1.Condition.
func getConditions(obj runtime.Object) []metav1.Condition {
	if obj == nil {
		return nil
	}

	unstructuredObj, ok := obj.(runtime.Unstructured)
	// If we aren't unstructured, then we must not be a CR.  While this should be handled eventually, for now just do nothing.
	// Our immediate concern is for CRs.
	if !ok {
		return nil
	}

	unstructuredMap := unstructuredObj.UnstructuredContent()
	conditionsSlice, found, err := unstructured.NestedSlice(unstructuredMap, "status", "conditions")
	if !found {
		return nil
	}
	if err != nil {
		return nil
	}

	ret := []metav1.Condition{}
	for _, currConditionUncast := range conditionsSlice {
		currCondition, ok := currConditionUncast.(map[string]interface{})
		if !ok {
			continue
		}

		typedCondition := metav1.Condition{}
		typedCondition.Type, _, _ = unstructured.NestedString(currCondition, "type")
		tempString, _, _ := unstructured.NestedString(currCondition, "status")
		typedCondition.Status = metav1.ConditionStatus(tempString)
		typedCondition.Reason, _, _ = unstructured.NestedString(currCondition, "reason")
		typedCondition.Message, _, _ = unstructured.NestedString(currCondition, "message")
		typedCondition.ObservedGeneration, _, _ = unstructured.NestedInt64(currCondition, "observedGeneration")
		tempString, _, _ = unstructured.NestedString(currCondition, "lastTransitionTime")
		if len(tempString) > 0 {
			if err := typedCondition.LastTransitionTime.UnmarshalJSON([]byte(strconv.Quote(tempString))); err != nil {
				panic(fmt.Sprintf("got %q: %v", tempString, err))
			}
		}
		ret = append(ret, typedCondition)
	}

	return ret
}

// coerce as much as we can to metav1.Condition.
func setLastTransitionTime(obj runtime.Object, index int, desiredTime metav1.Time) error {
	if obj == nil {
		return nil
	}

	unstructuredObj, ok := obj.(runtime.Unstructured)
	// If we aren't unstructured, then we must not be a CR.  While this should be handled eventually, for now just do nothing.
	// Our immediate concern is for CRs.
	if !ok {
		return nil
	}

	unstructuredMap := unstructuredObj.UnstructuredContent()
	conditionsSlice, found, err := unstructured.NestedSlice(unstructuredMap, "status", "conditions")
	if !found {
		return fmt.Errorf("no conditions found")
	}
	if err != nil {
		return fmt.Errorf("unable to get conditions: %w", err)
	}
	conditionMap, ok := conditionsSlice[index].(map[string]interface{})
	tBytes, _ := desiredTime.MarshalJSON()
	conditionMap["lastTransitionTime"], _ = strconv.Unquote(string(tBytes))
	conditionsSlice[index] = conditionMap
	if err := unstructured.SetNestedSlice(unstructuredMap, conditionsSlice, "status", "conditions"); err != nil {
		return fmt.Errorf("unable to set conditions: %w", err)
	}
	unstructuredObj.SetUnstructuredContent(unstructuredMap)

	return nil
}
