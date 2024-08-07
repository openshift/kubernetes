package internalregistry

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/meta/testrestmapper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	auditinternal "k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestProtectAnnotationValidate(t *testing.T) {

	testCases := []struct {
		name       string
		attributes admission.Attributes
		wantError  bool
	}{
		{
			name: "serviceaccount edit",
			attributes: attributes(
				withOldObject(sa(withProtectedAnnotationValue("before"))),
				withObject(sa(withProtectedAnnotationValue("after"))),
				withUser("system:serviceaccounts", "foo"),
			),
			wantError: true,
		},
		{
			name: "serviceaccount delete",
			attributes: attributes(
				withOldObject(sa(withProtectedAnnotationValue("before"))),
				withObject(sa()),
				withUser("system:serviceaccounts", "foo"),
			),
			wantError: true,
		},
		{
			name: "serviceaccount add",
			attributes: attributes(
				withObject(sa(withProtectedAnnotationValue("after"))),
				withOldObject(sa()),
				withUser("system:serviceaccounts", "foo"),
			),
			wantError: true,
		},
		{
			name: "serviceaccount no change",
			attributes: attributes(
				withOldObject(sa(withProtectedAnnotationValue("before"), withAnnotation("foo", "before"))),
				withObject(sa(withProtectedAnnotationValue("before"), withAnnotation("foo", "after"))),
				withUser("system:serviceaccounts", "foo"),
			),
			wantError: false,
		},
		{
			name: "serviceaccount edit subresource",
			attributes: attributes(
				withOldObject(sa(withProtectedAnnotationValue("before"))),
				withObject(sa(withProtectedAnnotationValue("after"))),
				withUser("system:serviceaccounts", "foo"),
				withSubResource(),
			),
			wantError: false,
		},
		{
			name: "ocm serviceaccount edit",
			attributes: attributes(
				withOldObject(sa(withProtectedAnnotationValue("before"))),
				withObject(sa(withProtectedAnnotationValue("after"))),
				withUser("system:serviceaccounts", "system:serviceaccounts:openshift-infra"),
			),
			wantError: false,
		},
		{
			name: "ocm serviceaccount delete",
			attributes: attributes(
				withOldObject(sa(withProtectedAnnotationValue("before"))),
				withObject(sa()),
				withUser("system:serviceaccounts", "system:serviceaccounts:openshift-infra"),
			),
			wantError: false,
		},
		{
			name: "ocm serviceaccount add",
			attributes: attributes(
				withObject(sa(withProtectedAnnotationValue("after"))),
				withOldObject(sa()),
				withUser("system:serviceaccounts", "system:serviceaccounts:openshift-infra"),
			),
			wantError: false,
		},
		{
			name: "ocm serviceaccount no change",
			attributes: attributes(
				withOldObject(sa(withProtectedAnnotationValue("before"), withAnnotation("foo", "before"))),
				withObject(sa(withProtectedAnnotationValue("before"), withAnnotation("foo", "after"))),
				withUser("system:serviceaccounts", "system:serviceaccounts:openshift-infra"),
			),
			wantError: false,
		},
		{
			name: "user edit",
			attributes: attributes(
				withOldObject(sa(withProtectedAnnotationValue("before"))),
				withObject(sa(withProtectedAnnotationValue("after"))),
				withUser("system:authenticated"),
			),
			wantError: false,
		},
		{
			name: "not a service account",
			attributes: attributes(
				withOldObject(pod(withProtectedAnnotationValue("before"))),
				withObject(pod(withProtectedAnnotationValue("after"))),
				withUser("system:serviceaccounts"),
			),
			wantError: false,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := protectAnnotation{}.Validate(context.TODO(), tc.attributes, nil)
			if (err != nil) != tc.wantError {
				if tc.wantError {
					t.Fatal("expected error")
				}
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}

}

type attributesRecord struct {
	kind        schema.GroupVersionKind
	namespace   string
	name        string
	resource    schema.GroupVersionResource
	subresource string
	operation   admission.Operation
	options     runtime.Object
	dryRun      bool
	object      runtime.Object
	oldObject   runtime.Object
	userInfo    user.Info
}

func (a attributesRecord) GetName() string                          { return a.name }
func (a attributesRecord) GetNamespace() string                     { return a.namespace }
func (a attributesRecord) GetResource() schema.GroupVersionResource { return a.resource }
func (a attributesRecord) GetSubresource() string                   { return a.subresource }
func (a attributesRecord) GetOperation() admission.Operation        { return a.operation }
func (a attributesRecord) GetOperationOptions() runtime.Object      { return a.options }
func (a attributesRecord) IsDryRun() bool                           { return a.dryRun }
func (a attributesRecord) GetObject() runtime.Object                { return a.object }
func (a attributesRecord) GetOldObject() runtime.Object             { return a.oldObject }
func (a attributesRecord) GetKind() schema.GroupVersionKind         { return a.kind }
func (a attributesRecord) GetUserInfo() user.Info                   { return a.userInfo }
func (a attributesRecord) AddAnnotation(key, value string) error    { panic("implement me") }
func (a attributesRecord) AddAnnotationWithLevel(key, value string, level auditinternal.Level) error {
	panic("implement me")
}
func (a attributesRecord) GetReinvocationContext() admission.ReinvocationContext {
	panic("implement me")
}

func attributes(opts ...func(*attributesRecord)) *attributesRecord {
	a := &attributesRecord{
		namespace: "test-ns",
		operation: admission.Update,
		options:   &metav1.UpdateOptions{},
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func withSubResource() func(*attributesRecord) {
	return func(a *attributesRecord) {
		a.subresource = "sub-resource"
	}
}

func withObject(o runtime.Object) func(*attributesRecord) {
	return func(record *attributesRecord) {
		record.object = o
		record.kind = o.GetObjectKind().GroupVersionKind()
		record.resource = gvk2gvr(record.kind)
	}
}

func gvk2gvr(gvk schema.GroupVersionKind) schema.GroupVersionResource {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	mapper := testrestmapper.TestOnlyStaticRESTMapper(scheme)
	mapping, err := mapper.RESTMapping(gvk.GroupKind())
	if err != nil {
		panic(err)
	}
	return mapping.Resource
}

func withOldObject(o runtime.Object) func(*attributesRecord) {
	return func(record *attributesRecord) {
		record.oldObject = o
	}
}

func withUser(g ...string) func(*attributesRecord) {
	return func(record *attributesRecord) {
		record.userInfo = &user.DefaultInfo{Name: "test", Groups: g}
	}
}

func pod(opts ...func(runtime.Object)) *corev1.Pod {
	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
	}
	pod.Name = "test-pod"
	for _, opt := range opts {
		opt(pod)
	}
	return pod
}

func sa(opts ...func(runtime.Object)) *corev1.ServiceAccount {
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ServiceAccount",
			APIVersion: "v1",
		},
	}
	sa.Name = "test-sa"
	for _, opt := range opts {
		opt(sa)
	}
	return sa
}

func withProtectedAnnotationValue(v string) func(object runtime.Object) {
	return withAnnotation(protectedAnnotationKey, v)
}

func withAnnotation(k, v string) func(object runtime.Object) {
	return func(o runtime.Object) {
		m, err := meta.Accessor(o)
		if err != nil {
			panic(err)
		}
		if m.GetAnnotations() == nil {
			m.SetAnnotations(make(map[string]string))
		}
		m.GetAnnotations()[k] = v
	}
}
