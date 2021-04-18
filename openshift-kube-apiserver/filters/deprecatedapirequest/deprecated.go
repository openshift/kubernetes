package deprecatedapirequest

//go:generate go run ./deprecated_generator.go

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// removedRelease of a specified resource.version.group.
func removedRelease(resource schema.GroupVersionResource) string {
	return deprecatedApiRemovedRelease[resource]
}
