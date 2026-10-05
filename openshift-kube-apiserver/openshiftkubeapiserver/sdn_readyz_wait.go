package openshiftkubeapiserver

import (
	gocontext "context"
	"fmt"
	"net"
	"net/http"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
)

func newOpenshiftAPIServiceReachabilityCheck(ipForKubernetesDefaultService net.IP) *aggregatedAPIServiceAvailabilityCheck {
	return newAggregatedAPIServiceReachabilityCheck(ipForKubernetesDefaultService, "openshift-apiserver", "api", "/apis/route.openshift.io/v1")
}

func newOAuthPIServiceReachabilityCheck(ipForKubernetesDefaultService net.IP) *aggregatedAPIServiceAvailabilityCheck {
	return newAggregatedAPIServiceReachabilityCheck(ipForKubernetesDefaultService, "openshift-oauth-apiserver", "api", "/apis/oauth.openshift.io/v1")
}

// if the aggregated API is not reachable through the aggregator within 60 seconds, we report ready
// no matter what -- this avoids a rebootstrapping deadlock.
// otherwise, wait for up to 60 seconds until a request through the aggregation layer succeeds.
func newAggregatedAPIServiceReachabilityCheck(ipForKubernetesDefaultService net.IP, namespace, service, aggregatedAPIPath string) *aggregatedAPIServiceAvailabilityCheck {
	return &aggregatedAPIServiceAvailabilityCheck{
		done:                          make(chan struct{}),
		ipForKubernetesDefaultService: ipForKubernetesDefaultService,
		namespace:                     namespace,
		serviceName:                   service,
		aggregatedAPIPath:             aggregatedAPIPath,
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
	// serviceName is used to check for the existence of the aggregated apiserver's endpoints
	serviceName string
	// aggregatedAPIPath is the API group discovery path probed through the loopback to exercise the
	// aggregator's actual proxy transport to the backend.
	aggregatedAPIPath string
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
		close(waitUntilCh) // this stops the polling
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

	// if the kubernetes.default.svc needs an endpoint and this is the only apiserver that can fulfill it, then we don't
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

	// Probe the aggregated API through this kube-apiserver's own aggregation layer via the
	// loopback. This exercises the aggregator's actual http2 proxy transport to the backend,
	// so a success means the aggregator can serve requests right now. If the aggregator has a
	// dead connection pinned from the network convergence window, the request will fail until
	// http2 PING-based dead connection detection closes it and the aggregator reconnects.
	loopbackConfig := rest.CopyConfig(context.LoopbackClientConfig)
	loopbackConfig.Timeout = 5 * time.Second
	loopbackHTTPClient, err := rest.HTTPClientFor(loopbackConfig)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("%s failed to create loopback HTTP client: %v", c.Name(), err))
		return
	}

	go func() {
		defer utilruntime.HandleCrash()

		wait.PollImmediateUntil(1*time.Second, func() (bool, error) {
			loopbackURL := loopbackConfig.Host + c.aggregatedAPIPath
			req, err := http.NewRequest("GET", loopbackURL, nil)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("%s failed to create request: %v", c.Name(), err))
				return false, nil
			}
			resp, err := loopbackHTTPClient.Do(req)
			if err != nil {
				klog.V(2).Infof("%s not yet reachable via aggregator: %v", c.Name(), err)
				return false, nil
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound {
				klog.Warningf("%s not found via aggregator, no APIService registered", c.Name())
				close(noAggregatedAPIServer)
				return true, nil
			}
			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				klog.V(2).Infof("%s returned %d via aggregator", c.Name(), resp.StatusCode)
				return false, nil
			}

			close(reachedAggregatedAPIServer)
			return true, nil
		}, waitUntilCh)
	}()

	select {
	case <-time.After(60 * time.Second):
		utilruntime.HandleError(fmt.Errorf("%s never reached aggregated apiserver via aggregator", c.Name()))
		return
	case <-context.Done():
		utilruntime.HandleError(fmt.Errorf("%s interrupted", c.Name()))
		return
	case <-noAggregatedAPIServer:
		utilruntime.HandleError(fmt.Errorf("%s has no APIService registered", c.Name()))
		return
	case <-reachedAggregatedAPIServer:
		end := time.Now()
		klog.Infof("reached %s via aggregator after %v milliseconds", c.namespace, end.Sub(start).Milliseconds())
		return
	}
}
