package resolver

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// MagicClient creates a resilient client-go that, in case of connection failures,
// tries to connect to all the available apiservers in the cluster.
func MagicClient(ctx context.Context, c *rest.Config) (*kubernetes.Clientset, error) {
	// create an apiserver resolver
	r, err := NewResolver(ctx, c)
	if err != nil {
		return nil, err
	}
	// inject the API server resolver into the client
	if c.Dial != nil {
		return nil, fmt.Errorf("APIServer resolver doesn't support custom dialers")
	}

	// create the clientset with our own resolver
	config := *c
	config.Resolver = r
	return kubernetes.NewForConfig(&config)
}
