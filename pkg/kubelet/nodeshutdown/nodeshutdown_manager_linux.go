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

// Package nodeshutdown can watch for node level shutdown events and trigger graceful termination of pods running on the node prior to a system shutdown.
package nodeshutdown

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/clock"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/apis/scheduling"
	"k8s.io/kubernetes/pkg/features"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/pkg/kubelet/eviction"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/kubernetes/pkg/kubelet/nodeshutdown/systemd"
)

const (
	nodeShutdownReason          = "Shutdown"
	nodeShutdownMessage         = "Node is shutting, evicting pods"
	nodeShutdownNotAdmitMessage = "Node is in progress of shutting down, not admitting any new pods"
	dbusReconnectPeriod         = 1 * time.Second
)

var systemDbus = func() (dbusInhibiter, error) {
	bus, err := dbus.SystemBus()
	if err != nil {
		return nil, err
	}
	return &systemd.DBusCon{SystemBus: bus}, nil
}

type dbusInhibiter interface {
	CurrentInhibitDelay() (time.Duration, error)
	InhibitShutdown() (systemd.InhibitLock, error)
	ReleaseInhibitLock(lock systemd.InhibitLock) error
	ReloadLogindConf() error
	MonitorShutdown() (<-chan bool, error)
	OverrideInhibitDelay(inhibitDelayMax time.Duration) error
}

// Manager has functions that can be used to interact with the Node Shutdown Manager.
type Manager struct {
	podPriorityShutdownGracePeriod []kubeletconfig.PodPriorityShutdownGracePeriod

	getPods        eviction.ActivePodsFunc
	killPod        eviction.KillPodFunc
	syncNodeStatus func()

	dbusCon     dbusInhibiter
	inhibitLock systemd.InhibitLock

	nodeShuttingDownMutex sync.Mutex
	nodeShuttingDownNow   bool

	clock clock.Clock
}

// NewManager returns a new node shutdown manager.
func NewManager(getPodsFunc eviction.ActivePodsFunc, killPodFunc eviction.KillPodFunc, syncNodeStatus func(),
	shutdownGracePeriodRequested, shutdownGracePeriodCriticalPods time.Duration,
	podPriorityShutdownGracePeriod []kubeletconfig.PodPriorityShutdownGracePeriod,
) (*Manager, lifecycle.PodAdmitHandler) {

	// Migration from the original configures
	if !utilfeature.DefaultFeatureGate.Enabled(features.PodPriorityBasedGracefulShutdown) ||
		len(podPriorityShutdownGracePeriod) == 0 {
		podPriorityShutdownGracePeriod = migrateConfig(shutdownGracePeriodRequested, shutdownGracePeriodCriticalPods)
	}

	// Sort by priority from low to high
	sort.Slice(podPriorityShutdownGracePeriod, func(i, j int) bool {
		return podPriorityShutdownGracePeriod[i].Priority < podPriorityShutdownGracePeriod[j].Priority
	})

	manager := &Manager{
		getPods:                        getPodsFunc,
		killPod:                        killPodFunc,
		syncNodeStatus:                 syncNodeStatus,
		podPriorityShutdownGracePeriod: podPriorityShutdownGracePeriod,
		clock:                          clock.RealClock{},
	}
	return manager, manager
}

// Admit rejects all pods if node is shutting
func (m *Manager) Admit(attrs *lifecycle.PodAdmitAttributes) lifecycle.PodAdmitResult {
	nodeShuttingDown := m.ShutdownStatus() != nil

	if nodeShuttingDown {
		return lifecycle.PodAdmitResult{
			Admit:   false,
			Reason:  nodeShutdownReason,
			Message: nodeShutdownNotAdmitMessage,
		}
	}
	return lifecycle.PodAdmitResult{Admit: true}
}

// Start starts the node shutdown manager and will start watching the node for shutdown events.
func (m *Manager) Start() error {
	if !m.isFeatureEnabled() {
		return nil
	}

	systemBus, err := systemDbus()
	if err != nil {
		return err
	}
	m.dbusCon = systemBus

	currentInhibitDelay, err := m.dbusCon.CurrentInhibitDelay()
	if err != nil {
		return err
	}

	// If the logind's InhibitDelayMaxUSec as configured in (logind.conf) is less than shutdownGracePeriodRequested, attempt to update the value to shutdownGracePeriodRequested.
	if periodRequested := m.periodRequested(); periodRequested > currentInhibitDelay {
		err := m.dbusCon.OverrideInhibitDelay(periodRequested)
		if err != nil {
			return fmt.Errorf("unable to override inhibit delay by shutdown manager: %v", err)
		}

		err = m.dbusCon.ReloadLogindConf()
		if err != nil {
			return err
		}

		// Read the current inhibitDelay again, if the override was successful, currentInhibitDelay will be equal to shutdownGracePeriodRequested.
		updatedInhibitDelay, err := m.dbusCon.CurrentInhibitDelay()
		if err != nil {
			return err
		}

		if updatedInhibitDelay != periodRequested {
			return fmt.Errorf("node shutdown manager was unable to update logind InhibitDelayMaxSec to %v (ShutdownGracePeriod), current value of InhibitDelayMaxSec (%v) is less than requested ShutdownGracePeriod", periodRequested, updatedInhibitDelay)
		}
	}

	err = m.aquireInhibitLock()
	if err != nil {
		return err
	}

	events, err := m.dbusCon.MonitorShutdown()
	if err != nil {
		releaseErr := m.dbusCon.ReleaseInhibitLock(m.inhibitLock)
		if releaseErr != nil {
			return fmt.Errorf("failed releasing inhibitLock: %v and failed monitoring shutdown: %v", releaseErr, err)
		}
		return fmt.Errorf("failed to monitor shutdown: %v", err)
	}

	go func() {
		// Monitor for shutdown events. This follows the logind Inhibit Delay pattern described on https://www.freedesktop.org/wiki/Software/systemd/inhibit/
		// 1. When shutdown manager starts, an inhibit lock is taken.
		// 2. When shutdown(true) event is received, process the shutdown and release the inhibit lock.
		// 3. When shutdown(false) event is received, this indicates a previous shutdown was cancelled. In this case, acquire the inhibit lock again.
		for {
			select {
			case isShuttingDown := <-events:
				klog.V(1).InfoS("Shutdown manager detected new shutdown event, isNodeShuttingDownNow", "event", isShuttingDown)

				m.nodeShuttingDownMutex.Lock()
				m.nodeShuttingDownNow = isShuttingDown
				m.nodeShuttingDownMutex.Unlock()

				if isShuttingDown {
					// Update node status and ready condition
					go m.syncNodeStatus()

					m.processShutdownEvent()
				} else {
					m.aquireInhibitLock()
				}
			}
		}
	}()
	return nil
}

func (m *Manager) aquireInhibitLock() error {
	lock, err := m.dbusCon.InhibitShutdown()
	if err != nil {
		return err
	}
	if m.inhibitLock != 0 {
		m.dbusCon.ReleaseInhibitLock(m.inhibitLock)
	}
	m.inhibitLock = lock
	return nil
}

// Returns if the feature is enabled
func (m *Manager) isFeatureEnabled() bool {
	return utilfeature.DefaultFeatureGate.Enabled(features.GracefulNodeShutdown) &&
		len(m.podPriorityShutdownGracePeriod) != 0
}

// ShutdownStatus will return an error if the node is currently shutting down.
func (m *Manager) ShutdownStatus() error {
	if !m.isFeatureEnabled() {
		return nil
	}

	m.nodeShuttingDownMutex.Lock()
	defer m.nodeShuttingDownMutex.Unlock()

	if m.nodeShuttingDownNow {
		return fmt.Errorf("node is shutting down")
	}
	return nil
}

func (m *Manager) processShutdownEvent() error {
	klog.V(1).InfoS("Shutdown manager processing shutdown event")
	activePods := m.getPods()

	groups := groupByPriority(m.podPriorityShutdownGracePeriod, activePods)
	for _, group := range groups {
		// If there are no pods in a particular range,
		// then does not wait for pods in that priority range.
		if len(group.Pods) == 0 {
			continue
		}

		var wg sync.WaitGroup
		wg.Add(len(group.Pods))
		for _, pod := range group.Pods {
			go func(pod *v1.Pod, group processShutdownGroup) {
				defer wg.Done()

				gracePeriodOverride := group.ShutdownGracePeriodSeconds

				// If the pod's spec specifies a termination gracePeriod which is less than the gracePeriodOverride calculated, use the pod spec termination gracePeriod.
				if pod.Spec.TerminationGracePeriodSeconds != nil && *pod.Spec.TerminationGracePeriodSeconds <= gracePeriodOverride {
					gracePeriodOverride = *pod.Spec.TerminationGracePeriodSeconds
				}

				klog.V(1).InfoS("Shutdown manager killing pod with gracePeriod", "pod", klog.KObj(pod), "gracePeriod", gracePeriodOverride)

				status := v1.PodStatus{
					Phase:   v1.PodFailed,
					Reason:  nodeShutdownReason,
					Message: nodeShutdownMessage,
				}

				err := m.killPod(pod, status, &gracePeriodOverride)
				if err != nil {
					klog.V(1).InfoS("Shutdown manager failed killing pod", "pod", klog.KObj(pod), "err", err)
				} else {
					klog.V(1).InfoS("Shutdown manager finished killing pod", "pod", klog.KObj(pod))
				}
			}(pod, group)
		}

		c := make(chan struct{})
		go func() {
			defer close(c)
			wg.Wait()
		}()

		select {
		case <-c:
		case <-time.After(time.Duration(group.ShutdownGracePeriodSeconds) * time.Second):
			klog.V(1).InfoS("Shutdown manager pod killing time out", "gracePeriod", group.ShutdownGracePeriodSeconds, "priority", group.Priority)
		}
	}

	m.dbusCon.ReleaseInhibitLock(m.inhibitLock)
	klog.V(1).InfoS("Shutdown manager completed processing shutdown event, node will shutdown shortly")

	return nil
}

func (m *Manager) periodRequested() time.Duration {
	var sum int64
	for _, period := range m.podPriorityShutdownGracePeriod {
		sum += period.ShutdownGracePeriodSeconds
	}
	return time.Duration(sum) * time.Second
}

func migrateConfig(shutdownGracePeriodRequested, shutdownGracePeriodCriticalPods time.Duration) []kubeletconfig.PodPriorityShutdownGracePeriod {
	if shutdownGracePeriodRequested == 0 {
		return nil
	}
	defaultPriority := shutdownGracePeriodRequested - shutdownGracePeriodCriticalPods
	criticalPriority := shutdownGracePeriodRequested - defaultPriority
	return []kubeletconfig.PodPriorityShutdownGracePeriod{
		{
			Priority:                   scheduling.DefaultPriorityWhenNoDefaultClassExists,
			ShutdownGracePeriodSeconds: int64(defaultPriority / time.Second),
		},
		{
			Priority:                   scheduling.SystemCriticalPriority,
			ShutdownGracePeriodSeconds: int64(criticalPriority / time.Second),
		},
	}
}

func groupByPriority(podPriorityShutdownGracePeriod []kubeletconfig.PodPriorityShutdownGracePeriod, pods []*v1.Pod) []processShutdownGroup {
	groups := make([]processShutdownGroup, 0, len(podPriorityShutdownGracePeriod))
	for _, period := range podPriorityShutdownGracePeriod {
		groups = append(groups, processShutdownGroup{
			PodPriorityShutdownGracePeriod: period,
		})
	}

	for _, pod := range pods {
		var priority int32
		if pod.Spec.Priority != nil {
			priority = *pod.Spec.Priority
		}

		// Find the group index according to the priority.
		index := sort.Search(len(groups), func(i int) bool {
			return groups[i].Priority >= priority
		})

		// 1. Those higher than the highest priority default to the highest priority
		// 2. Those lower than the lowest priority default to the lowest priority
		// 3. Those boundary priority default to the lower priority
		if index == len(groups) {
			index = len(groups) - 1
		} else if index < 0 {
			index = 0
		} else if index > 0 && groups[index].Priority > priority {
			index--
		}

		groups[index].Pods = append(groups[index].Pods, pod)
	}
	return groups
}

type processShutdownGroup struct {
	kubeletconfig.PodPriorityShutdownGracePeriod
	Pods []*v1.Pod
}
