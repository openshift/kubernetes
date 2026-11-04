package openshiftkubeapiserver

import (
	gocontext "context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

func newOpenshiftAPIServiceReachabilityCheck(ipForKubernetesDefaultService net.IP) *aggregatedAPIServiceAvailabilityCheck {
	return newAggregatedAPIServiceReachabilityCheck(ipForKubernetesDefaultService, "openshift-apiserver", "api")
}

// requiredConsecutiveSuccesses is the number of consecutive polls on which every endpoint of the aggregated
// apiserver service must be reachable before the availability check reports complete.  A single successful
// connection is not enough: right after a node reboot the pod network may still be converging and connectivity
// flaps, so a lucky one-off connection must not mark this kube-apiserver as ready to serve aggregated APIs.
const requiredConsecutiveSuccesses = 3

// allEndpointsReachable returns true when the endpoints object lists at least one ready address and every
// listed ready address accepts a connection.  Any http response (including errors) counts as reachable,
// consistent with the anonymous probing done by this check.
func allEndpointsReachable(client *http.Client, endpoints *corev1.Endpoints, port string) bool {
	addressCount := 0
	for _, subset := range endpoints.Subsets {
		for _, address := range subset.Addresses {
			addressCount++
			url := fmt.Sprintf("https://%v", net.JoinHostPort(address.IP, port))
			resp, err := client.Get(url)
			if err != nil {
				klog.V(2).Infof("failed to connect to %q: %v", url, err)
				return false
			}
			response, dumpErr := httputil.DumpResponse(resp, true)
			klog.V(4).Infof("reached to connect to %q: %v\n%v", url, dumpErr, string(response))
			resp.Body.Close()
		}
	}
	return addressCount > 0
}

func newOAuthPIServiceReachabilityCheck(ipForKubernetesDefaultService net.IP) *aggregatedAPIServiceAvailabilityCheck {
	return newAggregatedAPIServiceReachabilityCheck(ipForKubernetesDefaultService, "openshift-oauth-apiserver", "api")
}

// if the API service is not found, then this check returns quickly.
// if the endpoints are not accessible within 60 seconds, we report ready no matter what
// otherwise, wait for up to 60 seconds until every endpoint is reachable on consecutive polls
func newAggregatedAPIServiceReachabilityCheck(ipForKubernetesDefaultService net.IP, namespace, service string) *aggregatedAPIServiceAvailabilityCheck {
	return &aggregatedAPIServiceAvailabilityCheck{
		done:                          make(chan struct{}),
		ipForKubernetesDefaultService: ipForKubernetesDefaultService,
		namespace:                     namespace,
		serviceName:                   service,
	}
}

type aggregatedAPIServiceAvailabilityCheck struct {
	// done indicates that this check is complete (success or failure) and the check should return true
	done chan struct{}

	// ipForKubernetesDefaultService is used to determine whether this endpoint is the only one for the kubernetes.default.svc
	// if so, it will report reachable immediately because honoring some requests is better than honoring no requests.
	ipForKubernetesDefaultService net.IP

	// namespace is the namespace hosting the service for the aggregated api
	namespace string
	// serviceName is used to get a list of endpoints to directly dial
	serviceName string
}

func (c *aggregatedAPIServiceAvailabilityCheck) Name() string {
	return fmt.Sprintf("%s-%s-available", c.serviceName, c.namespace)
}

func (c *aggregatedAPIServiceAvailabilityCheck) Check(req *http.Request) error {
	select {
	case <-c.done:
		return nil
	default:
		return fmt.Errorf("check is not yet complete")
	}
}

func (c *aggregatedAPIServiceAvailabilityCheck) checkForConnection(context genericapiserver.PostStartHookContext) {
	defer utilruntime.HandleCrash()

	reachedAggregatedAPIServer := make(chan struct{})
	noAggregatedAPIServer := make(chan struct{})
	waitUntilCh := make(chan struct{})
	defer func() {
		close(waitUntilCh) // this stops the endpoint check
		close(c.done)      // once this method is done, the ready check should return true
	}()
	start := time.Now()

	kubeClient, err := kubernetes.NewForConfig(context.LoopbackClientConfig)
	if err != nil {
		// shouldn't happen.  this means the loopback config didn't work.
		panic(err)
	}

	ctx, cancel := gocontext.WithTimeout(gocontext.TODO(), 30*time.Second)
	defer cancel()

	// if the kubernetes.default.svc needs an endpoint and this is the only apiserver than can fulfill it, then we don't
	// wait for reachability. We wait for other conditions, but unreachable apiservers correctly 503 for clients.
	kubeEndpoints, err := kubeClient.CoreV1().Endpoints("default").Get(ctx, "kubernetes", metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		utilruntime.HandleError(fmt.Errorf("%s did not find a kubernetes.default.svc endpoint", c.Name()))
		return
	case err != nil:
		utilruntime.HandleError(fmt.Errorf("%s unable to read a kubernetes.default.svc endpoint: %w", c.Name(), err))
		return
	case len(kubeEndpoints.Subsets) == 0:
		utilruntime.HandleError(fmt.Errorf("%s did not find any IPs for kubernetes.default.svc endpoint", c.Name()))
		return
	case len(kubeEndpoints.Subsets[0].Addresses) == 0:
		utilruntime.HandleError(fmt.Errorf("%s did not find any IPs for kubernetes.default.svc endpoint", c.Name()))
		return
	case len(kubeEndpoints.Subsets[0].Addresses) == 1:
		if kubeEndpoints.Subsets[0].Addresses[0].IP == c.ipForKubernetesDefaultService.String() {
			utilruntime.HandleError(fmt.Errorf("%s only found this kube-apiserver's IP (%v) in kubernetes.default.svc endpoint", c.Name(), c.ipForKubernetesDefaultService))
			return
		}
	}

	// Start a thread which repeatedly tries to connect to every aggregated apiserver endpoint.
	//  1. if the aggregated apiserver endpoint doesn't exist, logs a warning and reports ready
	//  2. if the connections cannot be made, after 60 seconds logs an error and reports ready -- this avoids a rebootstrapping cycle
	//  3. as soon as every listed endpoint is reachable on requiredConsecutiveSuccesses consecutive polls, logs a time to be
	//     ready and reports ready.  Requiring all endpoints (instead of any one) on consecutive polls avoids latching ready
	//     during a window where pod-network connectivity from this node is still flapping (for instance while OVN is still
	//     converging right after a node reboot).  Connections established during such a window get pinned by the aggregator's
	//     http2 transport and produce 503 "http2: client connection lost" errors for tens of seconds after the network heals.
	go func() {
		defer utilruntime.HandleCrash()

		client := http.Client{
			Transport: &http.Transport{
				// since any http return code satisfies us, we don't bother to send credentials.
				// we don't care about someone faking a response and we aren't sending credentials, so we don't check the server CA
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			Timeout: 1 * time.Second, // these should all be very fast.  if none work, we continue anyway.
		}

		consecutiveSuccesses := 0
		wait.PollImmediateUntil(1*time.Second, func() (bool, error) {
			ctx := gocontext.TODO()
			openshiftEndpoints, err := kubeClient.CoreV1().Endpoints(c.namespace).Get(ctx, c.serviceName, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				// if we have no aggregated apiserver endpoint, we have no reason to wait
				klog.Warningf("%s.%s.svc endpoints were not found", c.serviceName, c.namespace)
				close(noAggregatedAPIServer)
				return true, nil
			}
			if err != nil {
				utilruntime.HandleError(err)
				consecutiveSuccesses = 0
				return false, nil
			}
			if !allEndpointsReachable(&client, openshiftEndpoints, "8443") {
				consecutiveSuccesses = 0
				return false, nil
			}
			consecutiveSuccesses++
			if consecutiveSuccesses < requiredConsecutiveSuccesses {
				return false, nil
			}
			close(reachedAggregatedAPIServer)
			return true, nil
		}, waitUntilCh)
	}()

	select {
	case <-time.After(60 * time.Second):
		// if we timeout, always return ok so that we can start from a case where all kube-apiservers are down and the SDN isn't coming up
		utilruntime.HandleError(fmt.Errorf("%s never reached apiserver", c.Name()))
		return
	case <-context.Done():
		utilruntime.HandleError(fmt.Errorf("%s interrupted", c.Name()))
		return
	case <-noAggregatedAPIServer:
		utilruntime.HandleError(fmt.Errorf("%s did not find an %s endpoint", c.Name(), c.namespace))
		return

	case <-reachedAggregatedAPIServer:
		end := time.Now()
		klog.Infof("reached all %s endpoints via SDN after %v milliseconds", c.namespace, end.Sub(start).Milliseconds())
		return
	}
}
