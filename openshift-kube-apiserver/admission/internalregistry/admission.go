package internalregistry

import (
	"context"
	"fmt"
	"io"

	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apiserver/pkg/admission"
)

const PluginName = "registry.openshift.io/ProtectAnnotation"

func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(config io.Reader) (admission.Interface, error) {
		return protectAnnotation{Handler: admission.NewHandler(admission.Update)}, nil
	})
}

// protectAnnotation plugin prevents annotations set by OCM from being overwritten by other service accounts.
type protectAnnotation struct {
	*admission.Handler
}

const protectedAnnotationKey = "openshift.io/internal-registry-pull-secret-ref"

var allowedServiceAccounts = []string{
	"system:serviceaccounts:openshift-infra",
	"system:serviceaccounts:openshift-controller-manager",
	"system:serviceaccounts:openshift-controller-manager-operator",
}

func (m protectAnnotation) Validate(ctx context.Context, a admission.Attributes, o admission.ObjectInterfaces) error {
	if a.GetResource().Resource != "serviceaccounts" || len(a.GetSubresource()) > 0 {
		return nil
	}
	if !slices.Contains(a.GetUserInfo().GetGroups(), "system:serviceaccounts") {
		// user is not a service account, allow
		return nil
	}
	if slices.IndexFunc(a.GetUserInfo().GetGroups(), func(s string) bool {
		return slices.Contains(allowedServiceAccounts, s)
	}) > 0 {
		return nil
	}
	changed, err := annotationChanged(a, protectedAnnotationKey)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return admission.NewForbidden(a, fmt.Errorf("'%s' annotation can only be changed by openshift-controller-manager", protectedAnnotationKey))
}

func annotationChanged(a admission.Attributes, key string) (bool, error) {
	before, err := meta.Accessor(a.GetOldObject())
	if err != nil {
		return false, err
	}
	after, err := meta.Accessor(a.GetObject())
	if err != nil {
		return false, err
	}
	valueBefore, keyExistedBefore := before.GetAnnotations()[key]
	valueAfter, keyExistsAfter := after.GetAnnotations()[key]
	if (keyExistedBefore == keyExistsAfter) && (valueBefore == valueAfter) {
		// annotation was not changed
		return false, nil
	}
	return true, nil
}
