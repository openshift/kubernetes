package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

func TestWithNotFoundProtectorHandler(t *testing.T) {
	// test data
	var currentUser, currentPath string
	hasBeenReadyCh := make(chan struct{})
	delegate := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	authorizerAttributesTestFunc := func(ctx context.Context) (authorizer.Attributes, error) {
		return authorizer.AttributesRecord{
			User: &user.DefaultInfo{
				Name: currentUser,
			},
			Path: currentPath,
		}, nil
	}
	testServer := httptest.NewServer(WithNotFoundProtectorHandler(delegate, hasBeenReadyCh, authorizerAttributesTestFunc))
	defer testServer.Close()

	// scenario 1: server hasn't been ready and the user doesn't match
	currentUser = "bob"
	currentPath = "/apis/operators.coreos.com/v1alpha1/namespaces/abc/clusterserviceversions"
	expect(t, testServer, currentPath, http.StatusOK)

	// scenario 2: hasn't been ready and the path doesn't match
	currentUser = "system:serviceaccount:kube-system:generic-garbage-collector"
	currentPath = "/api/v1/namespaces/abc/endpoints"
	expect(t, testServer, currentPath, http.StatusOK)

	// scenario 3: hasn't been ready for GC
	currentUser = "system:serviceaccount:kube-system:generic-garbage-collector"
	currentPath = "/apis/operators.coreos.com/v1alpha1/namespaces/abc/clusterserviceversions"
	expect(t, testServer, currentPath, http.StatusTooManyRequests)

	// scenario 4: hasn't been ready for ns controller
	currentUser = "system:serviceaccount:kube-system:namespace-controller"
	currentPath = "/apis/operators.coreos.com/v1alpha1/namespaces/abc/clusterserviceversions"
	expect(t, testServer, currentPath, http.StatusTooManyRequests)

	close(hasBeenReadyCh)

	// scenario 5: has been ready for GC
	currentUser = "system:serviceaccount:kube-system:generic-garbage-collector"
	currentPath = "/apis/operators.coreos.com/v1alpha1/namespaces/abc/clusterserviceversions"
	expect(t, testServer, currentPath, http.StatusOK)

	// scenario 6: has been ready for ns controller
	currentUser = "system:serviceaccount:kube-system:namespace-controller"
	currentPath = "/apis/operators.coreos.com/v1alpha1/namespaces/abc/clusterserviceversions"
	expect(t, testServer, currentPath, http.StatusOK)
}

func expect(t *testing.T, testServer *httptest.Server, currentPath string, expectedStatusCode int) {
	t.Helper()

	response, err := testServer.Client().Get(testServer.URL + currentPath)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expectedStatusCode {
		t.Fatalf("expected %d, got %d", expectedStatusCode, response.StatusCode)
	}
}
