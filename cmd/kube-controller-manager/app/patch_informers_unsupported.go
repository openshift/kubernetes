// +build !openshift

package app

import (
	"fmt"

	"k8s.io/client-go/rest"
)

func patchInformers(clientConfig *rest.Config) error {
	return fmt.Errorf("unsupported without build tag 'openshift'")
}
