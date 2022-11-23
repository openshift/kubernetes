package main

import (
	"fmt"
	"os"

	"github.com/openshift/origin/test/extended/util/image"
	k8simage "k8s.io/kubernetes/test/utils/image"
)

func verifyImages() error {
	if len(os.Getenv("KUBE_TEST_REPO")) > 0 {
		return fmt.Errorf("KUBE_TEST_REPO may not be specified when this command is run")
	}
	return verifyImagesWithoutEnv()
}

func verifyImagesWithoutEnv() error {
	defaults := k8simage.GetOriginalImageConfigs()

	for originalPullSpec, index := range image.OriginalImages() {
		if index == -1 {
			continue
		}
		existing, ok := defaults[index]
		if !ok {
			return fmt.Errorf("image %q not found in upstream images, must be moved to test/extended/util/image.  Upstream mappings are:\n%v", originalPullSpec, defaults)
		}
		if existing.GetE2EImage() != originalPullSpec {
			return fmt.Errorf("image %q defines index %d but is defined upstream as %q, must be fixed in test/extended/util/image.  Upstream mappings are:\n%v", originalPullSpec, index, existing.GetE2EImage(), defaults)
		}
		mirror := image.LocationFor(originalPullSpec)
		upstreamMirror := k8simage.GetE2EImage(index)
		if mirror != upstreamMirror {
			return fmt.Errorf("image %q defines index %d and mirror %q but is mirrored upstream as %q, must be fixed in test/extended/util/image.  Upstream mappings are:\n%v", originalPullSpec, index, mirror, upstreamMirror, defaults)
		}
	}

	return nil
}
