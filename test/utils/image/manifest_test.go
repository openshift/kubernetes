/*
Copyright 2019 The Kubernetes Authors.

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

package image

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func BenchmarkReplaceRegistryInImageURL(b *testing.B) {
	registryTests := []struct {
		in  string
		out string
	}{
		{
			in:  "docker.io/library/test:123",
			out: "test.io/library/test:123",
		}, {
			in:  "docker.io/library/test",
			out: "test.io/library/test",
		}, {
			in:  "test",
			out: "test.io/library/test",
		}, {
			in:  "registry.k8s.io/test:123",
			out: "test.io/test:123",
		}, {
			in:  "gcr.io/k8s-authenticated-test/test:123",
			out: "test.io/k8s-authenticated-test/test:123",
		}, {
			in:  "registry.k8s.io/sig-storage/test:latest",
			out: "test.io/sig-storage/test:latest",
		}, {
			in:  "invalid.registry.k8s.io/invalid/test:latest",
			out: "test.io/invalid/test:latest",
		}, {
			in:  "registry.k8s.io/e2e-test-images/test:latest",
			out: "test.io/promoter/test:latest",
		}, {
			in:  "registry.k8s.io/build-image/test:latest",
			out: "test.io/build/test:latest",
		}, {
			in:  "gcr.io/authenticated-image-pulling/test:latest",
			out: "test.io/gcAuth/test:latest",
		},
	}
	reg := RegistryList{
		DockerLibraryRegistry:   "test.io/library",
		GcRegistry:              "test.io",
		PrivateRegistry:         "test.io/k8s-authenticated-test",
		SigStorageRegistry:      "test.io/sig-storage",
		InvalidRegistry:         "test.io/invalid",
		PromoterE2eRegistry:     "test.io/promoter",
		BuildImageRegistry:      "test.io/build",
		GcAuthenticatedRegistry: "test.io/gcAuth",
	}
	for i := 0; i < b.N; i++ {
		tt := registryTests[i%len(registryTests)]
		s, _ := replaceRegistryInImageURLWithList(tt.in, reg)
		if s != tt.out {
			b.Errorf("got %q, want %q", s, tt.out)
		}
	}
}

func TestReplaceRegistryInImageURL(t *testing.T) {
	registryTests := []struct {
		in        string
		out       string
		expectErr error
	}{
		{
			in:  "docker.io/library/test:123",
			out: "test.io/library/test:123",
		}, {
			in:  "docker.io/library/test",
			out: "test.io/library/test",
		}, {
			in:  "test",
			out: "test.io/library/test",
		}, {
			in:  "registry.k8s.io/test:123",
			out: "test.io/test:123",
		}, {
			in:  "gcr.io/k8s-authenticated-test/test:123",
			out: "test.io/k8s-authenticated-test/test:123",
		}, {
			in:  "registry.k8s.io/sig-storage/test:latest",
			out: "test.io/sig-storage/test:latest",
		}, {
			in:  "invalid.registry.k8s.io/invalid/test:latest",
			out: "test.io/invalid/test:latest",
		}, {
			in:  "registry.k8s.io/e2e-test-images/test:latest",
			out: "test.io/promoter/test:latest",
		}, {
			in:  "registry.k8s.io/build-image/test:latest",
			out: "test.io/build/test:latest",
		}, {
			in:  "gcr.io/authenticated-image-pulling/test:latest",
			out: "test.io/gcAuth/test:latest",
		}, {
			in:        "unknwon.io/google-samples/test:latest",
			expectErr: fmt.Errorf("Registry: unknwon.io/google-samples is missing in test/utils/image/manifest.go, please add the registry, otherwise the test will fail on air-gapped clusters"),
		},
	}

	// Set custom registries
	reg := RegistryList{
		DockerLibraryRegistry:   "test.io/library",
		GcRegistry:              "test.io",
		PrivateRegistry:         "test.io/k8s-authenticated-test",
		SigStorageRegistry:      "test.io/sig-storage",
		InvalidRegistry:         "test.io/invalid",
		PromoterE2eRegistry:     "test.io/promoter",
		BuildImageRegistry:      "test.io/build",
		GcAuthenticatedRegistry: "test.io/gcAuth",
	}

	for _, tt := range registryTests {
		t.Run(tt.in, func(t *testing.T) {
			s, err := replaceRegistryInImageURLWithList(tt.in, reg)

			if err != nil && err.Error() != tt.expectErr.Error() {
				t.Errorf("got %q, want %q", err, tt.expectErr)
			}
			if s != tt.out {
				t.Errorf("got %q, want %q", s, tt.out)
			}
		})
	}
}

func TestGetOriginalImageConfigs(t *testing.T) {
	if len(GetOriginalImageConfigs()) == 0 {
		t.Fatalf("original map should not be empty")
	}
}

func TestGetMappedImageConfigs(t *testing.T) {
	originals := map[ImageID]Config{
		10: {registry: "docker.io", name: "source/repo", version: "1.0"},
	}
	mapping := GetMappedImageConfigs(originals, "quay.io/repo/for-test")

	actual := make(map[string]string)
	for i, mapping := range mapping {
		source := originals[i]
		actual[source.GetE2EImage()] = mapping.GetE2EImage()
	}
	expected := map[string]string{
		"docker.io/source/repo:1.0": "quay.io/repo/for-test:e2e-10-docker-io-source-repo-1-0-72R4aXm7YnxQ4_ek",
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatal(cmp.Diff(expected, actual))
	}
}

func TestGetImageConfigsWithMappedImage(t *testing.T) {
	tests := []struct {
		name                 string
		originalImageConfigs map[ImageID]Config
		repo                 string
		specialImages        []ImageID // Images that should not be mapped
	}{
		{
			name: "maps normal images to new repository",
			originalImageConfigs: map[ImageID]Config{
				Agnhost: {registry: "registry.k8s.io/e2e-test-images", name: "agnhost", version: "2.47"},
				BusyBox: {registry: "registry.k8s.io/e2e-test-images", name: "busybox", version: "1.36.1-1"},
			},
			repo:          "quay.io/test/repo",
			specialImages: []ImageID{},
		},
		{
			name: "does not map special images",
			originalImageConfigs: map[ImageID]Config{
				InvalidRegistryImage:           {registry: "invalid.registry.k8s.io/invalid", name: "alpine", version: "3.1"},
				AuthenticatedAlpine:            {registry: "gcr.io/authenticated-image-pulling", name: "alpine", version: "3.7"},
				AuthenticatedWindowsNanoServer: {registry: "gcr.io/authenticated-image-pulling", name: "windows-nanoserver", version: "v1"},
				AgnhostPrivate:                 {registry: "gcr.io/k8s-authenticated-test", name: "agnhost", version: "2.6"},
			},
			repo:          "quay.io/test/repo",
			specialImages: []ImageID{InvalidRegistryImage, AuthenticatedAlpine, AuthenticatedWindowsNanoServer, AgnhostPrivate},
		},
		{
			name: "handles mixed normal and special images",
			originalImageConfigs: map[ImageID]Config{
				Nginx:                {registry: "registry.k8s.io/e2e-test-images", name: "nginx", version: "1.14-4"},
				AuthenticatedAlpine:  {registry: "gcr.io/authenticated-image-pulling", name: "alpine", version: "3.7"},
				Pause:                {registry: "registry.k8s.io", name: "pause", version: "3.9"},
				InvalidRegistryImage: {registry: "invalid.registry.k8s.io/invalid", name: "alpine", version: "3.1"},
			},
			repo:          "my-registry.io/my-repo",
			specialImages: []ImageID{AuthenticatedAlpine, InvalidRegistryImage},
		},
		{
			name:                 "handles empty input",
			originalImageConfigs: map[ImageID]Config{},
			repo:                 "quay.io/test/repo",
			specialImages:        []ImageID{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetImageConfigsWithMappedImage(tt.originalImageConfigs, tt.repo)

			if len(result) != len(tt.originalImageConfigs) {
				t.Errorf("expected %d mapped configs, got %d", len(tt.originalImageConfigs), len(result))
			}

			for imageID, originalConfig := range tt.originalImageConfigs {
				actualConfigWithMapped, exists := result[imageID]
				if !exists {
					t.Errorf("expected imageID %v to be present in result", imageID)
					continue
				}

				// Check that original config is preserved
				actualOriginalImage := actualConfigWithMapped.Config.GetE2EImage()
				expectedOriginalImage := originalConfig.GetE2EImage()
				if actualOriginalImage != expectedOriginalImage {
					t.Errorf("original config mismatch for imageID %v: expected %q, got %q", imageID, expectedOriginalImage, actualOriginalImage)
				}

				// Check if this is a special image that should not be mapped
				isSpecial := false
				for _, specialID := range tt.specialImages {
					if imageID == specialID {
						isSpecial = true
						break
					}
				}

				actualMappedImage := actualConfigWithMapped.mapped.GetE2EImage()
				if isSpecial {
					// Special images should have empty mapped config (which results in "/:")
					if actualMappedImage != "/:" {
						t.Errorf("special image %v should have empty mapped config (resulting in '/:'), got %q", imageID, actualMappedImage)
					}
				} else {
					// Normal images should have a mapped config that's different from original
					if actualMappedImage == "" || actualMappedImage == "/:" {
						t.Errorf("normal image %v should have non-empty mapped config, got %q", imageID, actualMappedImage)
					}
					if actualMappedImage == actualOriginalImage {
						t.Errorf("mapped image should be different from original for imageID %v", imageID)
					}
					// Verify the mapped image uses the new repository
					expectedRepoPrefix := strings.Split(tt.repo, "/")[0]
					if !strings.HasPrefix(actualMappedImage, expectedRepoPrefix) {
						t.Errorf("mapped image %q should start with repository %q", actualMappedImage, expectedRepoPrefix)
					}
				}
			}
		})
	}
}
