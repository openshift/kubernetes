/*
Copyright 2021 The Kubernetes Authors.

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
	"time"

	"k8s.io/apimachinery/pkg/util/clock"
	kubeletconfig "k8s.io/kubernetes/pkg/kubelet/apis/config"
	"k8s.io/kubernetes/pkg/kubelet/eviction"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
)

// Manager interface provides methods for Kubelet to manager node shutdown.
type Manager interface {
	Admit(attrs *lifecycle.PodAdmitAttributes) lifecycle.PodAdmitResult
	Start() error
	ShutdownStatus() error
}

// Config represents Manager configuration
type Config struct {
	GetPodsFunc                     eviction.ActivePodsFunc
	KillPodFunc                     eviction.KillPodFunc
	SyncNodeStatus                  func()
	ShutdownGracePeriodRequested    time.Duration
	ShutdownGracePeriodCriticalPods time.Duration
	PodPriorityShutdownGracePeriod  []kubeletconfig.PodPriorityShutdownGracePeriod
	Clock                           clock.Clock
}

// managerStub is a fake node shutdown managerImpl .
type managerStub struct{}

// Admit returns a fake Pod admission which always returns true
func (managerStub) Admit(attrs *lifecycle.PodAdmitAttributes) lifecycle.PodAdmitResult {
	return lifecycle.PodAdmitResult{Admit: true}
}

// Start is a no-op always returning nil for non linux platforms.
func (managerStub) Start() error {
	return nil
}

// ShutdownStatus is a no-op always returning nil for non linux platforms.
func (managerStub) ShutdownStatus() error {
	return nil
}
