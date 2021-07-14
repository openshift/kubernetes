package server

import (
	"context"
	"net/http"
	"strings"

	"k8s.io/apiserver/pkg/authorization/authorizer"
)

// WithNotFoundProtectorHandler will return 429 instead of 404 iff:
//  - server hasn't been ready (/readyz=false)
//  - the user is GC or the namespace lifecycle controller
//  - the path is for an aggregated API or CR
//
// This handler ensures that the system stays consistent even when requests are received before the server is ready.
// In particular it prevents child deletion in case of GC or/and orphaned content in case of the namespaces controller.
func WithNotFoundProtectorHandler(delegate http.Handler, hasBeenReadyCh <-chan struct{}, authorizerAttributesFunc func(ctx context.Context) (authorizer.Attributes, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-hasBeenReadyCh:
			delegate.ServeHTTP(w, r)
			return
		default:
		}

		ctx := r.Context()
		attribs, err := authorizerAttributesFunc(ctx)
		if err != nil {
			delegate.ServeHTTP(w, r)
			return
		}

		if patchMatches(r.URL.Path) && userMatches(attribs.GetUser().GetName()) {
			w.Header().Set("Retry-After", "3")
			http.Error(w, "The server hasn't been ready yet, please try again later.", http.StatusTooManyRequests)
			return
		}
		delegate.ServeHTTP(w, r)
	})
}

func patchMatches(path string) bool {
	// since discovery contains all groups, we have to block the discovery paths until CRDs and APIServices are synced
	return strings.HasPrefix(path, "/apis") || strings.HasPrefix(path, "/apis/")
}

func userMatches(user string) bool {
	return user == "system:serviceaccount:kube-system:generic-garbage-collector" || user == "system:serviceaccount:kube-system:namespace-controller"
}
