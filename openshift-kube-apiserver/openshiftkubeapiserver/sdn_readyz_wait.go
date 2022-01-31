package openshiftkubeapiserver

import (
	gocontext "context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"sync/atomic"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

func newOpenshiftAPIServiceReachabilityCheck(apiSeverIP string) *aggregatedAPIServiceAvailabilityCheck {
	return newAggregatedAPIServiceReachabilityCheck("openshift-apiserver", "api", apiSeverIP)
}

func newOAuthPIServiceReachabilityCheck(apiServerIP string) *aggregatedAPIServiceAvailabilityCheck {
	return newAggregatedAPIServiceReachabilityCheck("openshift-oauth-apiserver", "api", apiServerIP)
}

// if the API service is not found, then this check returns quickly.
func newAggregatedAPIServiceReachabilityCheck(namespace, service, apiServerIP string) *aggregatedAPIServiceAvailabilityCheck {
	check := &aggregatedAPIServiceAvailabilityCheck{
		namespace:   namespace,
		serviceName: service,
		ip: apiServerIP,
	}
	check.readyzErrorMessage.Store("waiting for endpoint verification")
	return check
}

type aggregatedAPIServiceAvailabilityCheck struct {
	// readyzErrorMessage is not empty string when api endpoint can't be reach
	readyzErrorMessage atomic.Value
	// namespace is the namespace hosting the service for the aggregated api
	namespace string
	// serviceName is used to get a list of endpoints to directly dial
	serviceName string
	// IP address of apiserver where check is run
	ip string
}

func (c *aggregatedAPIServiceAvailabilityCheck) Name() string {
	return fmt.Sprintf("%s-%s-available", c.serviceName, c.namespace)
}

func (c *aggregatedAPIServiceAvailabilityCheck) Check(req *http.Request) error {
	if errMsg := c.readyzErrorMessage.Load().(string); len(errMsg) > 0 {
		return fmt.Errorf(errMsg)
	}
	return nil
}

func (c *aggregatedAPIServiceAvailabilityCheck) checkForConnection(context genericapiserver.PostStartHookContext) {
	defer utilruntime.HandleCrash()
	kubeClient, err := kubernetes.NewForConfig(context.LoopbackClientConfig)
	if err != nil {
		// shouldn't happen.  this means the loopback config didn't work.
		panic(err)
	}
	go wait.Until(func() {
		client := http.Client{
			Transport: &http.Transport{
				// since any http return code satisfies us, we don't bother to send credentials.
				// we don't care about someone faking a response and we aren't sending credentials, so we don't check the server CA
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
			Timeout: 1 * time.Second, // these should all be very fast.  if none work, we continue anyway.
		}
		ctx := gocontext.TODO()
		kubeApiEndpoints, err := kubeClient.CoreV1().Endpoints("openshift-kube-apiserver").Get(ctx, "apiserver", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			// bootstrap case
			klog.Warningf("apiserver.openshift-kube-apiserver.svc endpoints were not found")
			c.readyzErrorMessage.Store("")
			return
		}
		otherEndpointExist := false
		for _, subset := range kubeApiEndpoints.Subsets {
			for _, address := range subset.Addresses {
				if address.IP != c.ip {
					otherEndpointExist = true
					break
				}
			}
		}
		if !otherEndpointExist {
			// bootstrap case
			klog.V(2).Infof("Only %s registered as apiserver endpoint, set ready for this check", c.ip)
			c.readyzErrorMessage.Store("")
			return
		}
		openshiftEndpoints, err := kubeClient.CoreV1().Endpoints(c.namespace).Get(ctx, c.serviceName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			// if we have no aggregated apiserver endpoint, we have no reason to wait
			klog.Warningf("%s.%s.svc endpoints were not found", c.serviceName, c.namespace)
			c.readyzErrorMessage.Store("")
			return
		}
		for _, subset := range openshiftEndpoints.Subsets {
			for _, address := range subset.Addresses {
				url := fmt.Sprintf("https://%v", net.JoinHostPort(address.IP, "8443"))
				resp, err := client.Get(url)
				if err == nil { // any http response is fine.  it means that we made contact
					response, dumpErr := httputil.DumpResponse(resp, true)
					klog.V(4).Infof("reached to connect to %q: %v\n%v", url, dumpErr, string(response))
					c.readyzErrorMessage.Store("")
					resp.Body.Close()
					return
				}
				klog.V(2).Infof("failed to connect to %q: %v", url, err)
			}
		}
		c.readyzErrorMessage.Store(fmt.Sprintf("%s not reachable", c.Name()))
	}, 10*time.Second, context.StopCh)
}
