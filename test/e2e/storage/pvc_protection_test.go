/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package storage

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const pvcProtectionTestNamespace = "e2e-pvc-protection-1234"

// pvcProtectionTestClaim returns a claim in the state the Unschedulable spec
// expects at the moment it creates the second pod: bound, protected by the
// pvc-protection finalizer and already marked for deletion.
func pvcProtectionTestClaim() *v1.PersistentVolumeClaim {
	deletedAt := metav1.NewTime(time.Date(2026, time.November, 13, 2, 36, 54, 0, time.UTC))
	return &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "pvc-protection9b4q5",
			Namespace:         pvcProtectionTestNamespace,
			UID:               types.UID("11111111-2222-3333-4444-555555555555"),
			ResourceVersion:   "4711",
			DeletionTimestamp: &deletedAt,
			Finalizers:        []string{"kubernetes.io/pvc-protection"},
		},
		Status: v1.PersistentVolumeClaimStatus{Phase: v1.ClaimBound},
	}
}

func pvcProtectionTestPod(name string, phase v1.PodPhase, conditions ...v1.PodCondition) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: pvcProtectionTestNamespace,
			UID:       types.UID("uid-" + name),
		},
		Spec: v1.PodSpec{NodeName: "node-1"},
		Status: v1.PodStatus{
			Phase:      phase,
			Conditions: conditions,
		},
	}
}

// capturePVCProtectionLog collects everything framework.Logf writes while fn
// runs. GinkgoWriter is process global, so this must not be used from tests
// that call t.Parallel().
func capturePVCProtectionLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	ginkgo.GinkgoWriter.TeeTo(&buf)
	t.Cleanup(ginkgo.GinkgoWriter.ClearTeeWriters)
	fn()
	return buf.String()
}

func TestLogPVCProtectionFailureState(t *testing.T) {
	claim := pvcProtectionTestClaim()
	unprotectedClaim := pvcProtectionTestClaim()
	unprotectedClaim.DeletionTimestamp = nil
	unprotectedClaim.Finalizers = nil

	holder := pvcProtectionTestPod("pvc-tester-holder", v1.PodRunning, v1.PodCondition{
		Type:   v1.PodScheduled,
		Status: v1.ConditionTrue,
	})
	second := pvcProtectionTestPod("pvc-tester-second", v1.PodPending, v1.PodCondition{
		Type:    v1.PodScheduled,
		Status:  v1.ConditionFalse,
		Reason:  v1.PodReasonUnschedulable,
		Message: "0/6 nodes are available",
	})

	testCases := []struct {
		name      string
		objects   []runtime.Object
		injectErr bool
		pods      []pvcProtectionPodRef
		want      []string
		notWant   []string
	}{
		{
			name:    "reports claim and pod state",
			objects: []runtime.Object{claim, holder, second},
			pods: []pvcProtectionPodRef{
				{role: "claim holder", pod: holder},
				{role: "unschedulable", pod: second},
			},
			want: []string{
				`claim "pvc-protection9b4q5": uid=11111111-2222-3333-4444-555555555555 resourceVersion=4711 phase=Bound deletionTimestamp=2026-11-13 02:36:54 +0000 UTC finalizers=[kubernetes.io/pvc-protection]`,
				`claim holder pod "pvc-tester-holder": uid=uid-pvc-tester-holder node="node-1" phase=Running PodScheduled=True reason="" message=""`,
				`unschedulable pod "pvc-tester-second": uid=uid-pvc-tester-second node="node-1" phase=Pending PodScheduled=False reason="Unschedulable" message="0/6 nodes are available"`,
			},
			notWant: []string{"unable to retrieve", "already deleted"},
		},
		{
			// The failure mode this change exists to tell apart from the one
			// above: the claim is no longer protected or being deleted.
			name:    "reports a claim that is neither deleted nor protected",
			objects: []runtime.Object{unprotectedClaim},
			want:    []string{`phase=Bound deletionTimestamp=<nil> finalizers=[]`},
		},
		{
			name:      "reports retrieval failures without failing",
			injectErr: true,
			pods: []pvcProtectionPodRef{
				{role: "claim holder", pod: holder},
			},
			want: []string{
				`claim "pvc-protection9b4q5": unable to retrieve: Internal error occurred: injected API failure`,
				`claim holder pod "pvc-tester-holder": unable to retrieve: Internal error occurred: injected API failure`,
			},
		},
		{
			// Expected whenever a spec fails after deleting the second pod.
			name:    "distinguishes an already deleted object from a failure",
			objects: []runtime.Object{claim},
			pods: []pvcProtectionPodRef{
				{role: "unschedulable", pod: second},
			},
			want:    []string{`unschedulable pod "pvc-tester-second": already deleted`},
			notWant: []string{"unable to retrieve"},
		},
		{
			name:    "reports a pod that was never created",
			objects: []runtime.Object{claim},
			pods: []pvcProtectionPodRef{
				{role: "claim holder", pod: nil},
			},
			want:    []string{"claim holder pod: not created"},
			notWant: []string{"unable to retrieve"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(tc.objects...)
			if tc.injectErr {
				client.PrependReactor("get", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewInternalError(errors.New("injected API failure"))
				})
			}

			out := capturePVCProtectionLog(t, func() {
				logPVCProtectionFailureState(context.Background(), client, pvcProtectionTestNamespace, claim.Name, tc.pods...)
			})

			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("diagnostics missing %q, got:\n%s", want, out)
				}
			}
			for _, notWant := range tc.notWant {
				if strings.Contains(out, notWant) {
					t.Errorf("diagnostics unexpectedly contain %q, got:\n%s", notWant, out)
				}
			}
		})
	}
}

func TestPVCProtectionPodScheduled(t *testing.T) {
	testCases := []struct {
		name string
		pod  *v1.Pod
		want string
	}{
		{
			name: "condition present",
			pod: pvcProtectionTestPod("with-condition", v1.PodPending, v1.PodCondition{
				Type:    v1.PodScheduled,
				Status:  v1.ConditionFalse,
				Reason:  v1.PodReasonUnschedulable,
				Message: "0/6 nodes are available",
			}),
			want: `PodScheduled=False reason="Unschedulable" message="0/6 nodes are available"`,
		},
		{
			name: "condition absent",
			pod:  pvcProtectionTestPod("no-condition", v1.PodPending),
			want: "PodScheduled=<absent>",
		},
		{
			name: "other conditions only",
			pod: pvcProtectionTestPod("other-condition", v1.PodRunning, v1.PodCondition{
				Type:   v1.PodReady,
				Status: v1.ConditionTrue,
			}),
			want: "PodScheduled=<absent>",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pvcProtectionPodScheduled(tc.pod); got != tc.want {
				t.Errorf("pvcProtectionPodScheduled() = %q, want %q", got, tc.want)
			}
		})
	}
}
