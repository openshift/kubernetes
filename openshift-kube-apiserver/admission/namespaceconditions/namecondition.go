package namespaceconditions

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/warning"
)

// pluginHandlerWithNamespaceNameConditions skips running admission plugins if they deal in the namespaceToExclude list
type pluginHandlerWithNamespaceNameConditions struct {
	pluginName          string
	admissionPlugin     admission.Interface
	namespacesToExclude sets.String
}

var _ admission.ValidationInterface = &pluginHandlerWithNamespaceNameConditions{}
var _ admission.MutationInterface = &pluginHandlerWithNamespaceNameConditions{}

func (p pluginHandlerWithNamespaceNameConditions) Handles(operation admission.Operation) bool {
	return p.admissionPlugin.Handles(operation)
}

// Admit performs a mutating admission control check and emit metrics.
func (p pluginHandlerWithNamespaceNameConditions) Admit(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
	if !p.shouldRunAdmission(a) {
		p.recordSkippedWarning(ctx, a.GetNamespace())
		return nil
	}

	mutatingHandler, ok := p.admissionPlugin.(admission.MutationInterface)
	if !ok {
		return nil
	}
	return mutatingHandler.Admit(ctx, a, o)
}

// Validate performs a non-mutating admission control check and emits metrics.
func (p pluginHandlerWithNamespaceNameConditions) Validate(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
	if !p.shouldRunAdmission(a) {
		p.recordSkippedWarning(ctx, a.GetNamespace())
		return nil
	}

	validatingHandler, ok := p.admissionPlugin.(admission.ValidationInterface)
	if !ok {
		return nil
	}
	return validatingHandler.Validate(ctx, a, o)
}

func (p pluginHandlerWithNamespaceNameConditions) shouldRunAdmission(attr admission.Attributes) bool {
	namespaceName := attr.GetNamespace()
	if p.namespacesToExclude.Has(namespaceName) {
		return false
	}
	if (attr.GetResource().GroupResource() == schema.GroupResource{Resource: "namespaces"}) && p.namespacesToExclude.Has(attr.GetName()) {
		return false
	}

	return true
}

func (p pluginHandlerWithNamespaceNameConditions) recordSkippedWarning(ctx context.Context, ns string) {
	warning.AddWarning(ctx, "", fmt.Sprintf("Admission plugin %q skipped due to request namespace %q.", p.pluginName, ns))
}
