package conditiontransitiontimeinjection

import (
	"context"
	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
	"reflect"
	"sigs.k8s.io/yaml"
	"testing"
	"time"
)

func Test_mutateConditionTransitionTime_Admit(t *testing.T) {
	nowTime, err := time.Parse(time.RFC3339, "2024-07-02T15:04:05Z")
	if err != nil {
		panic(err)
	}

	oldNow := now
	now = func() metav1.Time {
		return metav1.Time{
			Time: nowTime,
		}
	}
	defer func() {
		now = oldNow
	}()

	type fields struct {
		Handler *admission.Handler
	}
	type args struct {
		ctx context.Context
		a   admission.Attributes
		o   admission.ObjectInterfaces
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		desiredObj runtime.Object
		wantErr    bool
	}{
		{
			name: "simple",
			args: args{
				ctx: nil,
				a: admission.NewAttributesRecord(
					mustYamlToUnstructured(`
kind: ClusterOperator
apiVersion: config.openshift.io/v1
metadata:
  annotations:
    openshift.io/ConditionTransitionTimeInjection: OnStatusChange
status:
  conditions:
  - type: Foo
    status: "True"
  - type: Bar
    status: "True"
    lastTransitionTime: 2024-07-06T15:09:01Z
`),
					nil,
					schema.GroupVersionKind{},
					"namespace",
					"name",
					schema.GroupVersionResource{},
					"status",
					admission.Update,
					nil,
					false,
					nil,
				),
				o: nil,
			},
			desiredObj: mustYamlToUnstructured(`
kind: ClusterOperator
apiVersion: config.openshift.io/v1
metadata:
  annotations:
    openshift.io/ConditionTransitionTimeInjection: OnStatusChange
status:
  conditions:
  - type: Foo
    status: "True"
    lastTransitionTime: 2024-07-02T15:04:05Z
  - type: Bar
    status: "True"
    lastTransitionTime: 2024-07-02T15:04:05Z # we overwrite this value because we have no old value
`),
		},
		{
			name: "update the right one",
			args: args{
				ctx: nil,
				a: admission.NewAttributesRecord(
					mustYamlToUnstructured(`
kind: ClusterOperator
apiVersion: config.openshift.io/v1
metadata:
  annotations:
    openshift.io/ConditionTransitionTimeInjection: OnStatusChange
status:
  conditions:
  - type: Foo
    status: "True"
  - type: Bar
    status: "True"
  - type: Baz
    status: "False"
`),
					mustYamlToUnstructured(`
kind: ClusterOperator
apiVersion: config.openshift.io/v1
metadata:
  annotations:
    openshift.io/ConditionTransitionTimeInjection: OnStatusChange
status:
  conditions:
  - type: Foo
    status: "False"
    lastTransitionTime: 2024-07-01T15:09:01Z
  - type: Bar
    status: "True"
    lastTransitionTime: 2024-07-06T15:09:01Z
`),
					schema.GroupVersionKind{},
					"namespace",
					"name",
					schema.GroupVersionResource{},
					"status",
					admission.Update,
					nil,
					false,
					nil,
				),
				o: nil,
			},
			desiredObj: mustYamlToUnstructured(`
kind: ClusterOperator
apiVersion: config.openshift.io/v1
metadata:
  annotations:
    openshift.io/ConditionTransitionTimeInjection: OnStatusChange
status:
  conditions:
  - type: Foo
    status: "True"
    lastTransitionTime: 2024-07-02T15:04:05Z # this condition's status changed, so inject it.
  - type: Bar
    status: "True"
    lastTransitionTime: 2024-07-06T15:09:01Z # we should take this value from the existing
  - type: Baz
    status: "False"
    lastTransitionTime: 2024-07-02T15:04:05Z # this condition is new, so inject it
`),
		},
		{
			name: "no injection unless requested",
			args: args{
				ctx: nil,
				a: admission.NewAttributesRecord(
					mustYamlToUnstructured(`
kind: ClusterOperator
apiVersion: config.openshift.io/v1
metadata:
  annotations:
status:
  conditions:
  - type: Foo
    status: "True"
  - type: Bar
    status: "True"
    lastTransitionTime: 2024-07-06T15:09:01Z
`),
					nil,
					schema.GroupVersionKind{},
					"namespace",
					"name",
					schema.GroupVersionResource{},
					"status",
					admission.Update,
					nil,
					false,
					nil,
				),
				o: nil,
			},
			desiredObj: mustYamlToUnstructured(`
kind: ClusterOperator
apiVersion: config.openshift.io/v1
metadata:
  annotations:
status:
  conditions:
  - type: Foo
    status: "True"
  - type: Bar
    status: "True"
    lastTransitionTime: 2024-07-06T15:09:01Z
`),
		},
		{
			name: "update no injection unless requested",
			args: args{
				ctx: nil,
				a: admission.NewAttributesRecord(
					mustYamlToUnstructured(`
kind: ClusterOperator
apiVersion: config.openshift.io/v1
metadata:
  annotations:
status:
  conditions:
  - type: Foo
    status: "True"
  - type: Bar
    status: "True"
  - type: Baz
    status: "False"
`),
					mustYamlToUnstructured(`
kind: ClusterOperator
apiVersion: config.openshift.io/v1
metadata:
  annotations:
    openshift.io/ConditionTransitionTimeInjection: OnStatusChange
status:
  conditions:
  - type: Foo
    status: "False"
    lastTransitionTime: 2024-07-01T15:09:01Z
  - type: Bar
    status: "True"
    lastTransitionTime: 2024-07-06T15:09:01Z
`),
					schema.GroupVersionKind{},
					"namespace",
					"name",
					schema.GroupVersionResource{},
					"status",
					admission.Update,
					nil,
					false,
					nil,
				),
				o: nil,
			},
			desiredObj: mustYamlToUnstructured(`
kind: ClusterOperator
apiVersion: config.openshift.io/v1
metadata:
  annotations:
status:
  conditions:
  - type: Foo
    status: "True"
  - type: Bar
    status: "True"
  - type: Baz
    status: "False"
`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mu := &mutateConditionTransitionTime{
				Handler: tt.fields.Handler,
			}
			if err := mu.Admit(tt.args.ctx, tt.args.a, tt.args.o); (err != nil) != tt.wantErr {
				t.Errorf("Admit() error = %v, wantErr %v", err, tt.wantErr)
			}
			actualObj := tt.args.a.GetObject()
			if !reflect.DeepEqual(actualObj, tt.desiredObj) {
				t.Error(cmp.Diff(actualObj, tt.desiredObj))
			}
		})
	}
}

func mustYamlToUnstructured(in string) runtime.Object {
	jsonString, err := yaml.YAMLToJSON([]byte(in))
	if err != nil {
		panic(err)
	}
	ret, _, err := unstructured.UnstructuredJSONScheme.Decode(jsonString, nil, &unstructured.Unstructured{})
	if err != nil {
		panic(err)
	}
	return ret
}
