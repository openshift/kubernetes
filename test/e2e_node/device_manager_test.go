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

package e2enode

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	kubeletpodresourcesv1 "k8s.io/kubelet/pkg/apis/podresources/v1"
	"k8s.io/kubernetes/pkg/kubelet/apis/podresources"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"
	"k8s.io/kubernetes/pkg/kubelet/cm/devicemanager/checkpoint"
	"k8s.io/kubernetes/pkg/kubelet/util"
	admissionapi "k8s.io/pod-security-admission/api"

	"k8s.io/kubernetes/test/e2e/framework"
	e2enode "k8s.io/kubernetes/test/e2e/framework/node"
	e2epod "k8s.io/kubernetes/test/e2e/framework/pod"
	e2eskipper "k8s.io/kubernetes/test/e2e/framework/skipper"
	e2etestfiles "k8s.io/kubernetes/test/e2e/framework/testfiles"
	testutils "k8s.io/kubernetes/test/utils"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

const (
	devicePluginDir = "/var/lib/kubelet/device-plugins"
	checkpointName  = "kubelet_internal_checkpoint"
)

// Serial because the test updates kubelet configuration.
var _ = SIGDescribe("Device Manager  [Serial] [Feature:DeviceManager][NodeFeature:DeviceManager]", func() {
	checkpointFullPath := filepath.Join(devicePluginDir, checkpointName)
	f := framework.NewDefaultFramework("devicemanager-test")
	f.NamespacePodSecurityEnforceLevel = admissionapi.LevelPrivileged

	ginkgo.Context("With SRIOV devices in the system", func() {
		// this test wants to reproduce what happened in https://github.com/kubernetes/kubernetes/issues/102880
		ginkgo.It("should be able to recover V1 (aka pre-1.20) checkpoint data and reject pods before device re-registration", func() {
			if sriovdevCount, err := countSRIOVDevices(); err != nil || sriovdevCount == 0 {
				e2eskipper.Skipf("this test is meant to run on a system with at least one configured VF from SRIOV device")
			}

			configMap := getSRIOVDevicePluginConfigMap(framework.TestContext.SriovdpConfigMapFile)
			sd := setupSRIOVConfigOrFail(f, configMap)

			waitForSRIOVResources(f, sd)

			cntName := "gu-container"
			// we create and delete a pod to make sure the internal device manager state contains a pod allocation
			ginkgo.By(fmt.Sprintf("Successfully admit one guaranteed pod with 1 core, 1 %s device", sd.resourceName))
			var initCtnAttrs []tmCtnAttribute
			ctnAttrs := []tmCtnAttribute{
				{
					ctnName:       cntName,
					cpuRequest:    "1000m",
					cpuLimit:      "1000m",
					deviceName:    sd.resourceName,
					deviceRequest: "1",
					deviceLimit:   "1",
				},
			}

			podName := "gu-pod-rec-pre-1"
			framework.Logf("creating pod %s attrs %v", podName, ctnAttrs)
			pod := makeTopologyManagerTestPod(podName, ctnAttrs, initCtnAttrs)
			pod = f.PodClient().CreateSync(pod)

			// now we need to simulate a node drain, so we remove all the pods, including the sriov device plugin.

			ginkgo.By("deleting the pod")
			// note we delete right now because we know the current implementation of devicemanager will NOT
			// clean up on pod deletion. When this changes, the deletion needs to be done after the test is done.
			deletePodSyncByName(f, pod.Name)
			waitForAllContainerRemoval(pod.Name, pod.Namespace)

			ginkgo.By("teardown the sriov device plugin")
			// since we will NOT be recreating the plugin, we clean up everything now
			teardownSRIOVConfigOrFail(f, sd)

			ginkgo.By("stopping the kubelet")
			killKubelet("SIGSTOP")

			ginkgo.By("rewriting the kubelet checkpoint file as v1")
			err := rewriteCheckpointAsV1(devicePluginDir, checkpointName)
			// make sure we remove any leftovers
			defer os.Remove(checkpointFullPath)
			framework.ExpectNoError(err)

			// this mimics a kubelet restart after the upgrade
			// TODO: is SIGTERM (less brutal) good enough?
			ginkgo.By("killing the kubelet")
			killKubelet("SIGKILL")

			ginkgo.By("waiting for the kubelet to be ready again")
			// Wait for the Kubelet to be ready.
			gomega.Eventually(func() bool {
				nodes, err := e2enode.TotalReady(f.ClientSet)
				framework.ExpectNoError(err)
				return nodes == 1
			}, time.Minute, time.Second).Should(gomega.BeTrue())

			// note we DO NOT start the sriov device plugin. This is intentional.
			// issue#102880 reproduces because of a race on startup caused by corrupted device manager
			// state which leads to v1.Node object not updated on apiserver.
			// So to hit the issue we need to receive the pod *before* the device plugin registers itself.
			// The simplest and safest way to reproduce is just avoid to run the device plugin again

			podName = "gu-pod-rec-post-2"
			framework.Logf("creating pod %s attrs %v", podName, ctnAttrs)
			pod = makeTopologyManagerTestPod(podName, ctnAttrs, initCtnAttrs)

			pod = f.PodClient().Create(pod)
			err = e2epod.WaitForPodCondition(f.ClientSet, f.Namespace.Name, pod.Name, "Failed", 30*time.Second, func(pod *v1.Pod) (bool, error) {
				if pod.Status.Phase != v1.PodPending {
					return true, nil
				}
				return false, nil
			})
			framework.ExpectNoError(err)
			pod, err = f.PodClient().Get(context.TODO(), pod.Name, metav1.GetOptions{})
			framework.ExpectNoError(err)

			if pod.Status.Phase != v1.PodFailed {
				framework.Failf("pod %s not failed: %v", pod.Name, pod.Status)
			}

			framework.Logf("checking pod %s status reason (%s)", pod.Name, pod.Status.Reason)
			if !isUnexpectedAdmissionError(pod) {
				framework.Failf("pod %s failed for wrong reason: %q", pod.Name, pod.Status.Reason)
			}

			deletePodSyncByName(f, pod.Name)
		})

		ginkgo.It("should be able to recover V1 (aka pre-1.20) checkpoint data and update topology info on device re-registration", func() {
			if sriovdevCount, err := countSRIOVDevices(); err != nil || sriovdevCount == 0 {
				e2eskipper.Skipf("this test is meant to run on a system with at least one configured VF from SRIOV device")
			}

			endpoint, err := util.LocalEndpoint(defaultPodResourcesPath, podresources.Socket)
			framework.ExpectNoError(err)

			configMap := getSRIOVDevicePluginConfigMap(framework.TestContext.SriovdpConfigMapFile)

			sd := setupSRIOVConfigOrFail(f, configMap)
			waitForSRIOVResources(f, sd)

			cli, conn, err := podresources.GetV1Client(endpoint, defaultPodResourcesTimeout, defaultPodResourcesMaxSize)
			framework.ExpectNoError(err)

			resp, err := cli.GetAllocatableResources(context.TODO(), &kubeletpodresourcesv1.AllocatableResourcesRequest{})
			conn.Close()
			framework.ExpectNoError(err)

			suitableDevs := 0
			for _, dev := range resp.GetDevices() {
				for _, node := range dev.GetTopology().GetNodes() {
					if node.GetID() != 0 {
						suitableDevs++
					}
				}
			}
			if suitableDevs == 0 {
				teardownSRIOVConfigOrFail(f, sd)
				e2eskipper.Skipf("no devices found on NUMA Cell other than 0")
			}

			cntName := "gu-container"
			// we create and delete a pod to make sure the internal device manager state contains a pod allocation
			ginkgo.By(fmt.Sprintf("Successfully admit one guaranteed pod with 1 core, 1 %s device", sd.resourceName))
			var initCtnAttrs []tmCtnAttribute
			ctnAttrs := []tmCtnAttribute{
				{
					ctnName:       cntName,
					cpuRequest:    "1000m",
					cpuLimit:      "1000m",
					deviceName:    sd.resourceName,
					deviceRequest: "1",
					deviceLimit:   "1",
				},
			}

			podName := "gu-pod-rec-pre-1"
			framework.Logf("creating pod %s attrs %v", podName, ctnAttrs)
			pod := makeTopologyManagerTestPod(podName, ctnAttrs, initCtnAttrs)
			pod = f.PodClient().CreateSync(pod)

			// now we need to simulate a node drain, so we remove all the pods, including the sriov device plugin.

			ginkgo.By("deleting the pod")
			// note we delete right now because we know the current implementation of devicemanager will NOT
			// clean up on pod deletion. When this changes, the deletion needs to be done after the test is done.
			deletePodSyncByName(f, pod.Name)
			waitForAllContainerRemoval(pod.Name, pod.Namespace)

			ginkgo.By("teardown the sriov device plugin")
			// no need to delete the config now (speed up later)
			deleteSRIOVPodOrFail(f, sd)

			ginkgo.By("stopping the kubelet")
			killKubelet("SIGSTOP")

			ginkgo.By("rewriting the kubelet checkpoint file as v1")
			err = rewriteCheckpointAsV1(devicePluginDir, checkpointName)
			// make sure we remove any leftovers
			defer os.Remove(checkpointFullPath)
			framework.ExpectNoError(err)

			// this mimics a kubelet restart after the upgrade
			// TODO: is SIGTERM (less brutal) good enough?
			ginkgo.By("killing the kubelet")
			killKubelet("SIGKILL")

			ginkgo.By("waiting for the kubelet to be ready again")
			// Wait for the Kubelet to be ready.
			gomega.Eventually(func() bool {
				nodes, err := e2enode.TotalReady(f.ClientSet)
				framework.ExpectNoError(err)
				return nodes == 1
			}, time.Minute, time.Second).Should(gomega.BeTrue())

			sd2 := &sriovData{
				configMap:      sd.configMap,
				serviceAccount: sd.serviceAccount,
			}
			sd2.pod = createSRIOVPodOrFail(f)
			defer teardownSRIOVConfigOrFail(f, sd2)
			waitForSRIOVResources(f, sd2)

			compareSRIOVResources(sd, sd2)

			cli, conn, err = podresources.GetV1Client(endpoint, defaultPodResourcesTimeout, defaultPodResourcesMaxSize)
			framework.ExpectNoError(err)
			defer conn.Close()

			resp2, err := cli.GetAllocatableResources(context.TODO(), &kubeletpodresourcesv1.AllocatableResourcesRequest{})
			framework.ExpectNoError(err)

			cntDevs := stringifyContainerDevices(resp.GetDevices())
			cntDevs2 := stringifyContainerDevices(resp2.GetDevices())
			if cntDevs != cntDevs2 {
				framework.Failf("different allocatable resources expected %v got %v", cntDevs, cntDevs2)
			}
		})

	})

	/*
		This end to end test is to simulate a scenario where after kubelet restart/node
		reboot application pods requesting devices appear before the device plugin
		pod exposing those devices as resources.

		The happy path is where after node reboot/ kubelet restart, the device plugin pod
		appears before the application pod. This PR and this e2e test
		aims to tackle the scenario where device plugin either does not appear first
		or doesn't get the chance to re-register itself.

		Since there is no way of controlling the order in which the pods appear after
		kubelet restart/node reboot, we can't guarantee that the application pod
		recovers before device plugin pod (the scenario we want to exercise here).
		If the device plugin pod is recovered before the test pod, we still can
		meaningfully reproduce the scenario by NOT sending the registration command.
		To do so sample device plugin is enhanced. For implementation details, refer to:
		`test/images/sample-device-plugin/sampledeviceplugin.go`. This enhancement
		allows auto-registration of the plugin to be controlled with the help of an environment
		variable: AUTO_REGISTER. By default AUTO_REGISTER=true allowing the device plugin
		to autoregister to kubelet. For this e2e test, we use sample device plugin
		spec with AUTO_REGISTER=false to first manually register the plugin to allow
		an application pod (requesting devices) to successfully run on the node
		followed by kubelet restart where device plugin doesn't register and the
		application pod fails with admission error.

		   Breakdown of the steps implemented as part of this e2e test is as follows:
		   1. Create sample device plugin with AUTO_REGISTER= false which means it
		      waits for a client to connect to a unix socket exposed at
		       `/var/lib/kubelet/device-plugins/registered` path.
		   2. Trigger plugin registeration by connecting to the abovementioned unix socket.
		   3. Create a test pod requesting devices exposed by the device plugin.
		   4. Stop kubelet.
		   5. Remove pods using CRI to ensure new pods are created after kubelet restart.
		   6. Restart kubelet.
		   7. Wait for the sample device plugin pod to be running. In this case,
		      the registration is not triggered.
		   8. Ensure that resource capacity/allocatable exported by the device plugin is zero.
		   9. The test pod should fail with `UnexpectedAdmissionError`
		   10. Delete the test pod.
		   11. Delete the sample device plugin pod.
	*/
	ginkgo.Context("With sample device plugin", func() {
		var deviceCount int = 2
		var devicePluginPod *v1.Pod

		// this test wants to reproduce what happened in https://github.com/kubernetes/kubernetes/issues/109595
		ginkgo.BeforeEach(func(ctx context.Context) {
			ginkgo.By("Wait for node to be ready")
			gomega.Eventually(func() bool {
				nodes, err := e2enode.TotalReady(ctx, f.ClientSet)
				framework.ExpectNoError(err)
				return nodes == 1
			}, time.Minute, time.Second).Should(gomega.BeTrue())

			ginkgo.By("Scheduling a sample device plugin pod")
			data, err := e2etestfiles.Read(SampleDevicePluginDS2YAML)
			if err != nil {
				framework.Fail(err.Error())
			}
			ds := readDaemonSetV1OrDie(data)

			dp := &v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: sampleDevicePluginName,
				},
				Spec: ds.Spec.Template.Spec,
			}

			devicePluginPod = e2epod.NewPodClient(f).CreateSync(ctx, dp)

			go func() {
				// Since autoregistration is disabled for the device plugin (as AUTO_REGISTER=false),
				// device plugin registration needs to be triggerred manually.
				// This is done by writing to the Unix socket exposed at:
				// `/var/lib/kubelet/device-plugins/registered`.

				triggerPath := devicePluginDir + "/registered"
				conn, err := net.Dial("unix", triggerPath)
				framework.ExpectNoError(err)
				defer conn.Close()

				_, err = conn.Write([]byte("registering"))
				framework.ExpectNoError(err)
			}()

			ginkgo.By("Waiting for devices to become available on the local node")
			gomega.Eventually(func() bool {
				node, ready := getLocalTestNode(ctx, f)
				return ready && numberOfSampleResources(node) > 0
			}, 5*time.Minute, framework.Poll).Should(gomega.BeTrue())
			framework.Logf("Successfully created device plugin pod")

			devsLen := int64(deviceCount) // shortcut
			ginkgo.By("Waiting for the resource exported by the sample device plugin to become available on the local node")
			gomega.Eventually(func() bool {
				node, ready := getLocalTestNode(ctx, f)
				return ready &&
					numberOfDevicesCapacity(node, resourceName) == devsLen &&
					numberOfDevicesAllocatable(node, resourceName) == devsLen
			}, 30*time.Second, framework.Poll).Should(gomega.BeTrue())
		})

		ginkgo.It("should deploy pod consuming devices first but fail with admission error after kubelet restart in case device plugin hasn't re-registered", func(ctx context.Context) {
			var err error
			podCMD := "while true; do sleep 1000; done;"

			ginkgo.By(fmt.Sprintf("creating a pods requiring %d %q", deviceCount, resourceName))

			pod := makeBusyboxDeviceRequiringPod(resourceName, podCMD)
			testPod := e2epod.NewPodClient(f).CreateSync(ctx, pod)

			ginkgo.By("making sure all the pods are ready")

			err = e2epod.WaitForPodCondition(ctx, f.ClientSet, testPod.Namespace, testPod.Name, "Ready", 120*time.Second, testutils.PodRunningReady)
			framework.ExpectNoError(err, "pod %s/%s did not go running", testPod.Namespace, testPod.Name)
			framework.Logf("pod %s/%s running", testPod.Namespace, testPod.Name)

			ginkgo.By("stopping the kubelet")
			startKubelet := stopKubelet()

			ginkgo.By("stopping all the local containers - using CRI")
			rs, _, err := getCRIClient()
			framework.ExpectNoError(err)
			sandboxes, err := rs.ListPodSandbox(ctx, &runtimeapi.PodSandboxFilter{})
			framework.ExpectNoError(err)
			for _, sandbox := range sandboxes {
				gomega.Expect(sandbox.Metadata).ToNot(gomega.BeNil())
				ginkgo.By(fmt.Sprintf("deleting pod using CRI: %s/%s -> %s", sandbox.Metadata.Namespace, sandbox.Metadata.Name, sandbox.Id))

				err := rs.RemovePodSandbox(ctx, sandbox.Id)
				framework.ExpectNoError(err)
			}

			ginkgo.By("restarting the kubelet")
			startKubelet()

			ginkgo.By("waiting for the kubelet to be ready again")
			// Wait for the Kubelet to be ready.
			gomega.Eventually(func() bool {
				nodes, err := e2enode.TotalReady(ctx, f.ClientSet)
				framework.ExpectNoError(err)
				return nodes == 1
			}, 2*time.Minute, time.Second).Should(gomega.BeTrue())

			ginkgo.By("making sure all the pods are ready after the recovery")

			var devicePluginPodAfterRestart, tmpPod *v1.Pod

			devicePluginPodAfterRestart, err = e2epod.NewPodClient(f).Get(ctx, devicePluginPod.Name, metav1.GetOptions{})
			framework.ExpectNoError(err)

			err = e2epod.WaitForPodCondition(ctx, f.ClientSet, devicePluginPodAfterRestart.Namespace, devicePluginPodAfterRestart.Name, "Ready", 120*time.Second, testutils.PodRunningReady)
			framework.ExpectNoError(err, "pod %s/%s did not go running", devicePluginPodAfterRestart.Namespace, devicePluginPodAfterRestart.Name)
			framework.Logf("pod %s/%s running", devicePluginPodAfterRestart.Namespace, devicePluginPodAfterRestart.Name)

			ginkgo.By("Waiting for the resource capacity/allocatable exported by the sample device plugin to become zero")

			// The device plugin pod has restarted but has not re-registered to kubelet (as AUTO_REGISTER= false)
			// and registration wasn't triggered manually (by writing to the unix socket exposed at
			// `/var/lib/kubelet/device-plugins/registered`). Because of this, the capacity and allocatable corresponding
			// to the resource exposed by the device plugin should be zero.
			gomega.Eventually(func() bool {
				node, ready := getLocalTestNode(ctx, f)
				return ready &&
					numberOfDevicesCapacity(node, resourceName) == 0 &&
					numberOfDevicesAllocatable(node, resourceName) == 0
			}, 30*time.Second, framework.Poll).Should(gomega.BeTrue())

			ginkgo.By("Checking that pod requesting devices failed to start because of admission error")

			// NOTE: The device plugin won't re-register again and this is intentional.
			// Because of this, the testpod (requesting a device) should fail with an admission error.

			gomega.Eventually(ctx, func() bool {
				tmpPod, err = e2epod.NewPodClient(f).Get(ctx, testPod.Name, metav1.GetOptions{})
				framework.ExpectNoError(err)

				if tmpPod.Status.Phase != v1.PodFailed {
					return false
				}

				if tmpPod.Status.Reason != "UnexpectedAdmissionError" {
					return false
				}

				if !strings.Contains(tmpPod.Status.Message, "Allocate failed due to can't allocate unhealthy devices") {
					return false
				}

				return true
			}, time.Minute, 5*time.Second).Should(
				gomega.Equal(true),
				"the pod succeeded to start, when it should fail with the admission error",
			)

			ginkgo.By("removing application pods")
			e2epod.NewPodClient(f).DeleteSync(ctx, tmpPod.Name, metav1.DeleteOptions{}, 2*time.Minute)
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			ginkgo.By("Deleting the device plugin pod")
			e2epod.NewPodClient(f).DeleteSync(ctx, devicePluginPod.Name, metav1.DeleteOptions{}, time.Minute)

			ginkgo.By("Deleting any Pods created by the test")
			l, err := e2epod.NewPodClient(f).List(context.TODO(), metav1.ListOptions{})
			framework.ExpectNoError(err)
			for _, p := range l.Items {
				if p.Namespace != f.Namespace.Name {
					continue
				}

				framework.Logf("Deleting pod: %s", p.Name)
				e2epod.NewPodClient(f).DeleteSync(ctx, p.Name, metav1.DeleteOptions{}, 2*time.Minute)
			}

			ginkgo.By("Waiting for devices to become unavailable on the local node")
			gomega.Eventually(func() bool {
				node, ready := getLocalTestNode(ctx, f)
				return ready && numberOfSampleResources(node) <= 0
			}, 5*time.Minute, framework.Poll).Should(gomega.BeTrue())
		})

	})

})

func compareSRIOVResources(expected, got *sriovData) {
	if expected.resourceName != got.resourceName {
		framework.Failf("different SRIOV resource name: expected %q got %q", expected.resourceName, got.resourceName)
	}
	if expected.resourceAmount != got.resourceAmount {
		framework.Failf("different SRIOV resource amount: expected %d got %d", expected.resourceAmount, got.resourceAmount)
	}
}

func isUnexpectedAdmissionError(pod *v1.Pod) bool {
	re := regexp.MustCompile(`Unexpected.*Admission.*Error`)
	return re.MatchString(pod.Status.Reason)
}

func rewriteCheckpointAsV1(dir, name string) error {
	ginkgo.By(fmt.Sprintf("Creating temporary checkpoint manager (dir=%q)", dir))
	checkpointManager, err := checkpointmanager.NewCheckpointManager(dir)
	if err != nil {
		return err
	}
	cp := checkpoint.New(make([]checkpoint.PodDevicesEntry, 0), make(map[string][]string))
	err = checkpointManager.GetCheckpoint(name, cp)
	if err != nil {
		return err
	}

	ginkgo.By(fmt.Sprintf("Read checkpoint %q %#v", name, cp))

	podDevices, registeredDevs := cp.GetDataInLatestFormat()
	podDevicesV1 := convertPodDeviceEntriesToV1(podDevices)
	cpV1 := checkpoint.NewV1(podDevicesV1, registeredDevs)

	blob, err := cpV1.MarshalCheckpoint()
	if err != nil {
		return err
	}

	// TODO: why `checkpointManager.CreateCheckpoint(name, cpV1)` doesn't seem to work?
	ckPath := filepath.Join(dir, name)
	os.WriteFile(filepath.Join("/tmp", name), blob, 0600)
	return os.WriteFile(ckPath, blob, 0600)
}

func convertPodDeviceEntriesToV1(entries []checkpoint.PodDevicesEntry) []checkpoint.PodDevicesEntryV1 {
	entriesv1 := []checkpoint.PodDevicesEntryV1{}
	for _, entry := range entries {
		deviceIDs := []string{}
		for _, perNUMANodeDevIDs := range entry.DeviceIDs {
			deviceIDs = append(deviceIDs, perNUMANodeDevIDs...)
		}
		entriesv1 = append(entriesv1, checkpoint.PodDevicesEntryV1{
			PodUID:        entry.PodUID,
			ContainerName: entry.ContainerName,
			ResourceName:  entry.ResourceName,
			DeviceIDs:     deviceIDs,
			AllocResp:     entry.AllocResp,
		})
	}
	return entriesv1
}

func stringifyContainerDevices(devs []*kubeletpodresourcesv1.ContainerDevices) string {
	entries := []string{}
	for _, dev := range devs {
		devIDs := dev.GetDeviceIds()
		if devIDs != nil {
			for _, devID := range dev.DeviceIds {
				nodes := dev.GetTopology().GetNodes()
				if nodes != nil {
					for _, node := range nodes {
						entries = append(entries, fmt.Sprintf("%s[%s]@NUMA=%d", dev.ResourceName, devID, node.GetID()))
					}
				} else {
					entries = append(entries, fmt.Sprintf("%s[%s]@NUMA=none", dev.ResourceName, devID))
				}
			}
		} else {
			entries = append(entries, dev.ResourceName)
		}
	}
	sort.Strings(entries)
	return strings.Join(entries, ", ")
}

func makeBusyboxDeviceRequiringPod(resourceName, cmd string) *v1.Pod {
	podName := "device-manager-test-" + string(uuid.NewUUID())
	rl := v1.ResourceList{
		v1.ResourceName(resourceName): *resource.NewQuantity(2, resource.DecimalSI),
	}
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: podName,
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{{
				Image: busyboxImage,
				Name:  podName,
				// Runs the specified command in the test pod.
				Command: []string{"sh", "-c", cmd},
				Resources: v1.ResourceRequirements{
					Limits:   rl,
					Requests: rl,
				},
			}},
		},
	}
}
