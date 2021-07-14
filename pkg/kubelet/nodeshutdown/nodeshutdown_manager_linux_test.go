// +build linux

/*
Copyright 2020 The Kubernetes Authors.

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

package nodeshutdown

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	featuregatetesting "k8s.io/component-base/featuregate/testing"
	"k8s.io/kubernetes/pkg/apis/scheduling"
	pkgfeatures "k8s.io/kubernetes/pkg/features"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/pkg/kubelet/nodeshutdown/systemd"
)

type fakeDbus struct {
	currentInhibitDelay        time.Duration
	overrideSystemInhibitDelay time.Duration
	shutdownChan               chan bool

	didInhibitShutdown      bool
	didOverrideInhibitDelay bool
}

func (f *fakeDbus) CurrentInhibitDelay() (time.Duration, error) {
	if f.didOverrideInhibitDelay {
		return f.overrideSystemInhibitDelay, nil
	}
	return f.currentInhibitDelay, nil
}

func (f *fakeDbus) InhibitShutdown() (systemd.InhibitLock, error) {
	f.didInhibitShutdown = true
	return systemd.InhibitLock(0), nil
}

func (f *fakeDbus) ReleaseInhibitLock(lock systemd.InhibitLock) error {
	return nil
}

func (f *fakeDbus) ReloadLogindConf() error {
	return nil
}

func (f *fakeDbus) MonitorShutdown() (<-chan bool, error) {
	return f.shutdownChan, nil
}

func (f *fakeDbus) OverrideInhibitDelay(inhibitDelayMax time.Duration) error {
	f.didOverrideInhibitDelay = true
	return nil
}

func makePod(name string, priority int32, terminationGracePeriod *int64) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			UID:  types.UID(name),
		},
		Spec: v1.PodSpec{
			Priority:                      &priority,
			TerminationGracePeriodSeconds: terminationGracePeriod,
		},
	}
}

func TestManager(t *testing.T) {
	systemDbusTmp := systemDbus
	defer func() {
		systemDbus = systemDbusTmp
	}()
	normalPodNoGracePeriod := makePod("normal-pod-nil-grace-period", scheduling.DefaultPriorityWhenNoDefaultClassExists, nil /* terminationGracePeriod */)
	criticalPodNoGracePeriod := makePod("critical-pod-nil-grace-period", scheduling.SystemCriticalPriority, nil /* terminationGracePeriod */)

	shortGracePeriod := int64(2)
	normalPodGracePeriod := makePod("normal-pod-grace-period", scheduling.DefaultPriorityWhenNoDefaultClassExists, &shortGracePeriod /* terminationGracePeriod */)
	criticalPodGracePeriod := makePod("critical-pod-grace-period", scheduling.SystemCriticalPriority, &shortGracePeriod /* terminationGracePeriod */)

	longGracePeriod := int64(1000)
	normalPodLongGracePeriod := makePod("normal-pod-long-grace-period", scheduling.DefaultPriorityWhenNoDefaultClassExists, &longGracePeriod /* terminationGracePeriod */)

	var tests = []struct {
		desc                             string
		activePods                       []*v1.Pod
		shutdownGracePeriodRequested     time.Duration
		shutdownGracePeriodCriticalPods  time.Duration
		systemInhibitDelay               time.Duration
		overrideSystemInhibitDelay       time.Duration
		expectedDidOverrideInhibitDelay  bool
		expectedPodToGracePeriodOverride map[string]int64
		expectedError                    error
	}{
		{
			desc:                             "no override (total=30s, critical=10s)",
			activePods:                       []*v1.Pod{normalPodNoGracePeriod, criticalPodNoGracePeriod},
			shutdownGracePeriodRequested:     time.Duration(30 * time.Second),
			shutdownGracePeriodCriticalPods:  time.Duration(10 * time.Second),
			systemInhibitDelay:               time.Duration(40 * time.Second),
			overrideSystemInhibitDelay:       time.Duration(40 * time.Second),
			expectedDidOverrideInhibitDelay:  false,
			expectedPodToGracePeriodOverride: map[string]int64{"normal-pod-nil-grace-period": 20, "critical-pod-nil-grace-period": 10},
		},
		{
			desc:                             "no override (total=30s, critical=10s) pods with terminationGracePeriod and without",
			activePods:                       []*v1.Pod{normalPodNoGracePeriod, criticalPodNoGracePeriod, normalPodGracePeriod, criticalPodGracePeriod},
			shutdownGracePeriodRequested:     time.Duration(30 * time.Second),
			shutdownGracePeriodCriticalPods:  time.Duration(10 * time.Second),
			systemInhibitDelay:               time.Duration(40 * time.Second),
			overrideSystemInhibitDelay:       time.Duration(40 * time.Second),
			expectedDidOverrideInhibitDelay:  false,
			expectedPodToGracePeriodOverride: map[string]int64{"normal-pod-nil-grace-period": 20, "critical-pod-nil-grace-period": 10, "normal-pod-grace-period": 2, "critical-pod-grace-period": 2},
		},
		{
			desc:                             "no override (total=30s, critical=10s) pod with long terminationGracePeriod is overridden",
			activePods:                       []*v1.Pod{normalPodNoGracePeriod, criticalPodNoGracePeriod, normalPodGracePeriod, criticalPodGracePeriod, normalPodLongGracePeriod},
			shutdownGracePeriodRequested:     time.Duration(30 * time.Second),
			shutdownGracePeriodCriticalPods:  time.Duration(10 * time.Second),
			systemInhibitDelay:               time.Duration(40 * time.Second),
			overrideSystemInhibitDelay:       time.Duration(40 * time.Second),
			expectedDidOverrideInhibitDelay:  false,
			expectedPodToGracePeriodOverride: map[string]int64{"normal-pod-nil-grace-period": 20, "critical-pod-nil-grace-period": 10, "normal-pod-grace-period": 2, "critical-pod-grace-period": 2, "normal-pod-long-grace-period": 20},
		},
		{
			desc:                             "no override (total=30, critical=0)",
			activePods:                       []*v1.Pod{normalPodNoGracePeriod, criticalPodNoGracePeriod},
			shutdownGracePeriodRequested:     time.Duration(30 * time.Second),
			shutdownGracePeriodCriticalPods:  time.Duration(0 * time.Second),
			systemInhibitDelay:               time.Duration(40 * time.Second),
			overrideSystemInhibitDelay:       time.Duration(40 * time.Second),
			expectedDidOverrideInhibitDelay:  false,
			expectedPodToGracePeriodOverride: map[string]int64{"normal-pod-nil-grace-period": 30, "critical-pod-nil-grace-period": 0},
		},
		{
			desc:                             "override successful (total=30, critical=10)",
			activePods:                       []*v1.Pod{normalPodNoGracePeriod, criticalPodNoGracePeriod},
			shutdownGracePeriodRequested:     time.Duration(30 * time.Second),
			shutdownGracePeriodCriticalPods:  time.Duration(10 * time.Second),
			systemInhibitDelay:               time.Duration(5 * time.Second),
			overrideSystemInhibitDelay:       time.Duration(30 * time.Second),
			expectedDidOverrideInhibitDelay:  true,
			expectedPodToGracePeriodOverride: map[string]int64{"normal-pod-nil-grace-period": 20, "critical-pod-nil-grace-period": 10},
		},
		{
			desc:                             "override unsuccessful",
			activePods:                       []*v1.Pod{normalPodNoGracePeriod, criticalPodNoGracePeriod},
			shutdownGracePeriodRequested:     time.Duration(30 * time.Second),
			shutdownGracePeriodCriticalPods:  time.Duration(10 * time.Second),
			systemInhibitDelay:               time.Duration(5 * time.Second),
			overrideSystemInhibitDelay:       time.Duration(5 * time.Second),
			expectedDidOverrideInhibitDelay:  true,
			expectedPodToGracePeriodOverride: map[string]int64{"normal-pod-nil-grace-period": 5, "critical-pod-nil-grace-period": 0},
			expectedError:                    fmt.Errorf("unable to update logind InhibitDelayMaxSec to 30s (ShutdownGracePeriod), current value of InhibitDelayMaxSec (5s) is less than requested ShutdownGracePeriod"),
		},
		{
			desc:                            "override unsuccessful, zero time",
			activePods:                      []*v1.Pod{normalPodNoGracePeriod, criticalPodNoGracePeriod},
			shutdownGracePeriodRequested:    time.Duration(5 * time.Second),
			shutdownGracePeriodCriticalPods: time.Duration(5 * time.Second),
			systemInhibitDelay:              time.Duration(0 * time.Second),
			overrideSystemInhibitDelay:      time.Duration(0 * time.Second),
			expectedError:                   fmt.Errorf("unable to update logind InhibitDelayMaxSec to 5s (ShutdownGracePeriod), current value of InhibitDelayMaxSec (0s) is less than requested ShutdownGracePeriod"),
		},
		{
			desc:                             "no override, all time to critical pods",
			activePods:                       []*v1.Pod{normalPodNoGracePeriod, criticalPodNoGracePeriod},
			shutdownGracePeriodRequested:     time.Duration(5 * time.Second),
			shutdownGracePeriodCriticalPods:  time.Duration(5 * time.Second),
			systemInhibitDelay:               time.Duration(5 * time.Second),
			overrideSystemInhibitDelay:       time.Duration(5 * time.Second),
			expectedDidOverrideInhibitDelay:  false,
			expectedPodToGracePeriodOverride: map[string]int64{"normal-pod-nil-grace-period": 0, "critical-pod-nil-grace-period": 5},
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			activePodsFunc := func() []*v1.Pod {
				return tc.activePods
			}

			type PodKillInfo struct {
				Name        string
				GracePeriod int64
			}

			podKillChan := make(chan PodKillInfo)
			killPodsFunc := func(pod *v1.Pod, status v1.PodStatus, gracePeriodOverride *int64) error {
				var gracePeriod int64
				if gracePeriodOverride != nil {
					gracePeriod = *gracePeriodOverride
				}
				podKillChan <- PodKillInfo{Name: pod.Name, GracePeriod: gracePeriod}
				return nil
			}

			fakeShutdownChan := make(chan bool)
			fakeDbus := &fakeDbus{currentInhibitDelay: tc.systemInhibitDelay, shutdownChan: fakeShutdownChan, overrideSystemInhibitDelay: tc.overrideSystemInhibitDelay}
			systemDbus = func() (dbusInhibiter, error) {
				return fakeDbus, nil
			}
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, pkgfeatures.GracefulNodeShutdown, true)()

			manager, _ := NewManager(activePodsFunc, killPodsFunc, func() {}, tc.shutdownGracePeriodRequested, tc.shutdownGracePeriodCriticalPods, nil)
			manager.clock = clock.NewFakeClock(time.Now())

			err := manager.Start()
			if tc.expectedError != nil {
				if err == nil {
					t.Errorf("unexpected error message. Got: <nil> want %s", tc.expectedError.Error())
				} else if !strings.Contains(err.Error(), tc.expectedError.Error()) {
					t.Errorf("unexpected error message. Got: %s want %s", err.Error(), tc.expectedError.Error())
				}
			} else {
				assert.NoError(t, err, "expected manager.Start() to not return error")
				assert.True(t, fakeDbus.didInhibitShutdown, "expected that manager inhibited shutdown")
				assert.NoError(t, manager.ShutdownStatus(), "expected that manager does not return error since shutdown is not active")
				assert.Equal(t, manager.Admit(nil).Admit, true)

				// Send fake shutdown event
				select {
				case fakeShutdownChan <- true:
				case <-time.After(1 * time.Second):
					t.Fatal()
				}

				// Wait for all the pods to be killed
				killedPodsToGracePeriods := map[string]int64{}
				for i := 0; i < len(tc.activePods); i++ {
					select {
					case podKillInfo := <-podKillChan:
						killedPodsToGracePeriods[podKillInfo.Name] = podKillInfo.GracePeriod
						continue
					case <-time.After(1 * time.Second):
						t.Fatal()
					}
				}

				assert.Error(t, manager.ShutdownStatus(), "expected that manager returns error since shutdown is active")
				assert.Equal(t, manager.Admit(nil).Admit, false)
				assert.Equal(t, tc.expectedPodToGracePeriodOverride, killedPodsToGracePeriods)
				assert.Equal(t, tc.expectedDidOverrideInhibitDelay, fakeDbus.didOverrideInhibitDelay, "override system inhibit delay differs")
			}
		})
	}
}

func TestFeatureEnabled(t *testing.T) {
	var tests = []struct {
		desc                         string
		shutdownGracePeriodRequested time.Duration
		featureGateEnabled           bool
		expectEnabled                bool
	}{
		{
			desc:                         "shutdownGracePeriodRequested 0; disables feature",
			shutdownGracePeriodRequested: time.Duration(0 * time.Second),
			featureGateEnabled:           true,
			expectEnabled:                false,
		},
		{
			desc:                         "feature gate disabled; disables feature",
			shutdownGracePeriodRequested: time.Duration(100 * time.Second),
			featureGateEnabled:           false,
			expectEnabled:                false,
		},
		{
			desc:                         "feature gate enabled; shutdownGracePeriodRequested > 0; enables feature",
			shutdownGracePeriodRequested: time.Duration(100 * time.Second),
			featureGateEnabled:           true,
			expectEnabled:                true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			activePodsFunc := func() []*v1.Pod {
				return nil
			}
			killPodsFunc := func(pod *v1.Pod, status v1.PodStatus, gracePeriodOverride *int64) error {
				return nil
			}
			defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, pkgfeatures.GracefulNodeShutdown, tc.featureGateEnabled)()

			manager, _ := NewManager(activePodsFunc, killPodsFunc, func() {}, tc.shutdownGracePeriodRequested, 0 /*shutdownGracePeriodCriticalPods*/, nil)
			manager.clock = clock.NewFakeClock(time.Now())

			assert.Equal(t, tc.expectEnabled, manager.isFeatureEnabled())
		})
	}
}
func TestRestart(t *testing.T) {
	systemDbusTmp := systemDbus
	defer func() {
		systemDbus = systemDbusTmp
	}()

	shutdownGracePeriodRequested := 30 * time.Second
	shutdownGracePeriodCriticalPods := 10 * time.Second
	systemInhibitDelay := 40 * time.Second
	overrideSystemInhibitDelay := 40 * time.Second
	activePodsFunc := func() []*v1.Pod {
		return nil
	}
	killPodsFunc := func(pod *v1.Pod, status v1.PodStatus, gracePeriodOverride *int64) error {
		return nil
	}
	syncNodeStatus := func() {}

	var shutdownChan chan bool
	var connChan = make(chan struct{}, 1)

	systemDbus = func() (dbusInhibiter, error) {
		defer func() {
			connChan <- struct{}{}
		}()

		shutdownChan = make(chan bool)
		dbus := &fakeDbus{currentInhibitDelay: systemInhibitDelay, shutdownChan: shutdownChan, overrideSystemInhibitDelay: overrideSystemInhibitDelay}
		return dbus, nil
	}

	manager, _ := NewManager(activePodsFunc, killPodsFunc, syncNodeStatus, shutdownGracePeriodRequested, shutdownGracePeriodCriticalPods, nil)
	err := manager.Start()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	for i := 0; i != 5; i++ {
		select {
		case <-time.After(dbusReconnectPeriod * 5):
			t.Fatal("wait dbus connect timeout")
		case <-connChan:
		}

		time.Sleep(time.Second)
		close(shutdownChan)
	}
}

func Test_migrateConfig(t *testing.T) {
	type args struct {
		shutdownGracePeriodRequested    time.Duration
		shutdownGracePeriodCriticalPods time.Duration
	}
	tests := []struct {
		name string
		args args
		want []kubeletconfig.PodPriorityShutdownGracePeriod
	}{
		{
			args: args{
				shutdownGracePeriodRequested:    300 * time.Second,
				shutdownGracePeriodCriticalPods: 120 * time.Second,
			},
			want: []kubeletconfig.PodPriorityShutdownGracePeriod{
				{
					Priority:                   scheduling.DefaultPriorityWhenNoDefaultClassExists,
					ShutdownGracePeriodSeconds: 180,
				},
				{
					Priority:                   scheduling.SystemCriticalPriority,
					ShutdownGracePeriodSeconds: 120,
				},
			},
		},
		{
			args: args{
				shutdownGracePeriodRequested:    100 * time.Second,
				shutdownGracePeriodCriticalPods: 0 * time.Second,
			},
			want: []kubeletconfig.PodPriorityShutdownGracePeriod{
				{
					Priority:                   scheduling.DefaultPriorityWhenNoDefaultClassExists,
					ShutdownGracePeriodSeconds: 100,
				},
				{
					Priority:                   scheduling.SystemCriticalPriority,
					ShutdownGracePeriodSeconds: 0,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := migrateConfig(tt.args.shutdownGracePeriodRequested, tt.args.shutdownGracePeriodCriticalPods); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("migrateConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_groupByPriority(t *testing.T) {
	type args struct {
		podPriorityShutdownGracePeriod []kubeletconfig.PodPriorityShutdownGracePeriod
		pods                           []*v1.Pod
	}
	tests := []struct {
		name string
		args args
		want []processShutdownGroup
	}{
		{
			args: args{
				podPriorityShutdownGracePeriod: migrateConfig(300*time.Second, 120*time.Second),
				pods: []*v1.Pod{
					makePod("normal-pod", scheduling.DefaultPriorityWhenNoDefaultClassExists, nil),
					makePod("highest-user-definable-pod", scheduling.HighestUserDefinablePriority, nil),
					makePod("critical-pod", scheduling.SystemCriticalPriority, nil),
				},
			},
			want: []processShutdownGroup{
				{
					PodPriorityShutdownGracePeriod: kubeletconfig.PodPriorityShutdownGracePeriod{
						Priority:                   scheduling.DefaultPriorityWhenNoDefaultClassExists,
						ShutdownGracePeriodSeconds: 180,
					},
					Pods: []*v1.Pod{
						makePod("normal-pod", scheduling.DefaultPriorityWhenNoDefaultClassExists, nil),
						makePod("highest-user-definable-pod", scheduling.HighestUserDefinablePriority, nil),
					},
				},
				{
					PodPriorityShutdownGracePeriod: kubeletconfig.PodPriorityShutdownGracePeriod{
						Priority:                   scheduling.SystemCriticalPriority,
						ShutdownGracePeriodSeconds: 120,
					},
					Pods: []*v1.Pod{
						makePod("critical-pod", scheduling.SystemCriticalPriority, nil),
					},
				},
			},
		},
		{
			args: args{
				podPriorityShutdownGracePeriod: []kubeletconfig.PodPriorityShutdownGracePeriod{
					{
						Priority:                   1,
						ShutdownGracePeriodSeconds: 10,
					},
					{
						Priority:                   2,
						ShutdownGracePeriodSeconds: 20,
					},
					{
						Priority:                   3,
						ShutdownGracePeriodSeconds: 30,
					},
					{
						Priority:                   4,
						ShutdownGracePeriodSeconds: 40,
					},
				},
				pods: []*v1.Pod{
					makePod("pod-0", 0, nil),
					makePod("pod-1", 1, nil),
					makePod("pod-2", 2, nil),
					makePod("pod-3", 3, nil),
					makePod("pod-4", 4, nil),
					makePod("pod-5", 5, nil),
				},
			},
			want: []processShutdownGroup{
				{
					PodPriorityShutdownGracePeriod: kubeletconfig.PodPriorityShutdownGracePeriod{
						Priority:                   1,
						ShutdownGracePeriodSeconds: 10,
					},
					Pods: []*v1.Pod{
						makePod("pod-0", 0, nil),
						makePod("pod-1", 1, nil),
					},
				},
				{
					PodPriorityShutdownGracePeriod: kubeletconfig.PodPriorityShutdownGracePeriod{
						Priority:                   2,
						ShutdownGracePeriodSeconds: 20,
					},
					Pods: []*v1.Pod{
						makePod("pod-2", 2, nil),
					},
				},
				{
					PodPriorityShutdownGracePeriod: kubeletconfig.PodPriorityShutdownGracePeriod{
						Priority:                   3,
						ShutdownGracePeriodSeconds: 30,
					},
					Pods: []*v1.Pod{
						makePod("pod-3", 3, nil),
					},
				},
				{
					PodPriorityShutdownGracePeriod: kubeletconfig.PodPriorityShutdownGracePeriod{
						Priority:                   4,
						ShutdownGracePeriodSeconds: 40,
					},
					Pods: []*v1.Pod{
						makePod("pod-4", 4, nil),
						makePod("pod-5", 5, nil),
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupByPriority(tt.args.podPriorityShutdownGracePeriod, tt.args.pods); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("groupByPriority() = %v, want %v", got, tt.want)
			}
		})
	}
}
