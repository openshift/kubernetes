package main

import (
	"fmt"
	"os"

	"k8s.io/kubernetes/openshift-hack/e2e/label"
)

func main() {
	goPath := os.Getenv("GOPATH")
	externalBinaryPath := fmt.Sprintf("%s/bin/k8s-tests-ext", goPath)
	label.Run(externalBinaryPath)
}
