/*
Copyright The Kubernetes Authors.

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

// Package quotadiagnostics captures a bounded snapshot of the state behind a failed
// wait for custom resource usage in the ResourceQuota lifecycle test. That test fails
// the moment the wait times out, and the deferred CRD cleanup plus the namespace
// teardown then delete the CustomResourceDefinition and the temporary quota before any
// artifact gathering runs, so by the time anyone looks the evidence is gone.
//
// These helpers live in a sub-package on purpose: do not move them back into
// test/e2e/apimachinery, and do not add test files to that package.
// test/e2e/apimachinery imports the internal apiextensions API, which
// staging/src/k8s.io/apiextensions-apiserver/pkg/apis/.import-restrictions allows only
// under the prefix "k8s.io/kubernetes/test/e2e/apimachinery". import-boss renames any
// package that has test files to "<path> ((tests:<name>))", and that renamed path no
// longer matches the allowed prefix, so the pre-existing internal import would start
// failing hack/verify-import-boss.sh.
package quotadiagnostics

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	restclient "k8s.io/client-go/rest"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/utils/crd"
)

const (
	// stepCount is the number of serial requests a collection
	// issues. The budget is derived from it so that every step can still spend its full
	// per request timeout.
	stepCount = 3
	// defaultRequestTimeout bounds every individual diagnostic
	// request.
	defaultRequestTimeout = 10 * time.Second
	// defaultBudget bounds the total wall clock time spent on
	// diagnostics so a slow API server cannot extend the test run.
	defaultBudget = stepCount * defaultRequestTimeout
	// logPrefix prefixes every diagnostic line so the whole
	// snapshot can be grepped out of a CI log.
	logPrefix = "resourcequota diagnostics:"
)

// reportedConditions are the only CustomResourceDefinition
// conditions reported. They decide whether the custom resource is servable and
// discoverable, which is exactly what the resourcequota controller depends on.
var reportedConditions = []apiextensionsv1.CustomResourceDefinitionConditionType{
	apiextensionsv1.NamesAccepted,
	apiextensionsv1.Established,
}

// customResourceDefinitionGetter reads a single CustomResourceDefinition. It is the
// subset of the generated apiextensions client that the diagnostics need.
type customResourceDefinitionGetter interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*apiextensionsv1.CustomResourceDefinition, error)
}

// resourceQuotaGetter reads a single ResourceQuota. It is the subset of the typed core
// client that the diagnostics need.
type resourceQuotaGetter interface {
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.ResourceQuota, error)
}

// apiResourceGetter reads the discovery document of a single group/version under the
// caller's context.
type apiResourceGetter interface {
	GetAPIResources(ctx context.Context, groupVersion string) (*metav1.APIResourceList, error)
}

// restAPIResourceGetter reads discovery through a REST client, so the request honours a
// context and a retry ceiling. discovery.DiscoveryInterface cannot be used here:
// ServerResourcesForGroupVersion issues its request with context.TODO and retries
// Retry-After responses up to ten times without a ceiling, which would let a throttling
// API server stretch the collection far past its budget and hold up the cleanup that
// runs after the failed test.
type restAPIResourceGetter struct {
	client restclient.Interface
}

func (g restAPIResourceGetter) GetAPIResources(ctx context.Context, groupVersion string) (*metav1.APIResourceList, error) {
	resources := &metav1.APIResourceList{}
	if err := g.client.Get().AbsPath("/apis/" + groupVersion).MaxRetries(1).Do(ctx).Into(resources); err != nil {
		return nil, err
	}
	return resources, nil
}

// diagnosticsTarget names the exact objects that may be read. The
// diagnostics never list or dump anything beyond these.
type diagnosticsTarget struct {
	crdName        string
	groupVersion   string
	resourcePlural string
	quotaNamespace string
	quotaName      string
	resourceName   v1.ResourceName
}

// newDiagnosticsTarget derives the read targets from the custom
// resource definition the test created.
func newDiagnosticsTarget(definition *apiextensionsv1.CustomResourceDefinition, quotaNamespace, quotaName string, resourceName v1.ResourceName) diagnosticsTarget {
	return diagnosticsTarget{
		crdName:        definition.Name,
		groupVersion:   firstServedCustomResourceVersion(definition),
		resourcePlural: definition.Spec.Names.Plural,
		quotaNamespace: quotaNamespace,
		quotaName:      quotaName,
		resourceName:   resourceName,
	}
}

// collector gathers a bounded, read-only snapshot explaining why the resourcequota
// controller never reported usage for a custom resource. All fields are required;
// RecordUsageFailure builds the production value.
type collector struct {
	logf           func(format string, args ...interface{})
	crds           customResourceDefinitionGetter
	quotas         resourceQuotaGetter
	apiResources   apiResourceGetter
	budget         time.Duration
	requestTimeout time.Duration
}

// log emits one diagnostic line. Every line carries the shared prefix so the snapshot
// can be grepped out of a CI log as a unit.
func (d collector) log(format string, args ...interface{}) {
	d.logf("%s %s", logPrefix, fmt.Sprintf(format, args...))
}

// recordFailure returns cause unchanged so the caller reports the original failure.
// Nothing is collected when cause is nil, which keeps the diagnostics failure only.
func (d collector) recordFailure(ctx context.Context, target diagnosticsTarget, cause error) error {
	if cause == nil {
		return nil
	}
	d.log("ResourceQuota %s/%s never reported usage for %q: %v; collecting diagnostics before cleanup",
		target.quotaNamespace, target.quotaName, target.resourceName, cause)

	// The wait that just failed timed out on its own sub-context, but the caller's
	// context may be done as well, for instance when the spec itself ran out of time or
	// the run was interrupted. Detach cancellation so the reads below stay usable in
	// that case, and give the whole collection its own bound.
	budgetCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.budget)
	defer cancel()
	d.collect(budgetCtx, target)

	return cause
}

// collect runs the diagnostic steps serially, in a fixed order, within the budget that
// ctx carries. It is best effort: every failure is logged instead of returned.
func (d collector) collect(ctx context.Context, target diagnosticsTarget) {
	d.logCustomResourceDefinition(ctx, target.crdName)
	d.logResourceQuota(ctx, target.quotaNamespace, target.quotaName, target.resourceName)
	d.logGroupVersionDiscovery(ctx, target.groupVersion, target.resourcePlural)
}

// requestContext derives a per-request context from the remaining budget. ok is false
// when the budget is already spent, so collection stops instead of issuing more
// requests.
func (d collector) requestContext(ctx context.Context, what string) (context.Context, context.CancelFunc, bool) {
	if err := ctx.Err(); err != nil {
		d.log("skipping %s, diagnostics budget is spent: %v", what, err)
		return ctx, func() {}, false
	}
	// The budget is the parent, so a request can never outlive what is left of it.
	requestCtx, cancel := context.WithTimeout(ctx, d.requestTimeout)
	return requestCtx, cancel, true
}

// logCustomResourceDefinition reports the identity of the custom resource and the two
// conditions that decide whether it can be served and discovered at all.
func (d collector) logCustomResourceDefinition(ctx context.Context, name string) {
	what := fmt.Sprintf("CustomResourceDefinition %q", name)
	requestCtx, cancel, ok := d.requestContext(ctx, what)
	if !ok {
		return
	}
	defer cancel()

	definition, err := d.crds.Get(requestCtx, name, metav1.GetOptions{})
	if err != nil {
		d.log("failed to read %s: %v", what, err)
		return
	}
	d.log("%s identity: %s group=%s plural=%s kind=%s scope=%s acceptedPlural=%s versions=%s",
		what, formatObjectIdentity(definition),
		definition.Spec.Group, definition.Spec.Names.Plural, definition.Spec.Names.Kind, definition.Spec.Scope,
		definition.Status.AcceptedNames.Plural,
		formatCustomResourceDefinitionVersions(definition.Spec.Versions))
	d.log("%s conditions: %s", what, formatCustomResourceDefinitionConditions(definition.Status.Conditions))
}

// logResourceQuota reports the temporary quota the test keeps updating: the hard limits
// it asked for, the status the controller wrote back, and the metadata needed to tell
// how far the update loop got before the deadline.
func (d collector) logResourceQuota(ctx context.Context, namespace, name string, resourceName v1.ResourceName) {
	what := fmt.Sprintf("ResourceQuota %s/%s", namespace, name)
	requestCtx, cancel, ok := d.requestContext(ctx, what)
	if !ok {
		return
	}
	defer cancel()

	rq, err := d.quotas.Get(requestCtx, name, metav1.GetOptions{})
	if err != nil {
		d.log("failed to read %s: %v", what, err)
		return
	}
	_, inSpecHard := rq.Spec.Hard[resourceName]
	_, inStatusHard := rq.Status.Hard[resourceName]
	_, inStatusUsed := rq.Status.Used[resourceName]
	d.log("%s identity: %s", what, formatObjectIdentity(rq))
	d.log("%s spec.hard=%s status.hard=%s status.used=%s; %q present in spec.hard=%t status.hard=%t status.used=%t",
		what,
		formatResourceList(rq.Spec.Hard), formatResourceList(rq.Status.Hard), formatResourceList(rq.Status.Used),
		resourceName, inSpecHard, inStatusHard, inStatusUsed)
}

// logGroupVersionDiscovery reports whether the custom resource is visible in discovery,
// which is how the resourcequota controller learns that the resource exists.
func (d collector) logGroupVersionDiscovery(ctx context.Context, groupVersion, plural string) {
	if groupVersion == "" {
		d.log("skipping discovery, the CustomResourceDefinition has no served version")
		return
	}
	what := fmt.Sprintf("discovery for %q", groupVersion)
	requestCtx, cancel, ok := d.requestContext(ctx, what)
	if !ok {
		return
	}
	defer cancel()

	resources, err := d.apiResources.GetAPIResources(requestCtx, groupVersion)
	if err != nil {
		d.log("%s failed: %v", what, err)
		return
	}
	for _, resource := range resources.APIResources {
		if resource.Name != plural {
			continue
		}
		d.log("%s lists %q: kind=%s namespaced=%t verbs=[%s]",
			what, resource.Name, resource.Kind, resource.Namespaced, strings.Join(resource.Verbs, ","))
		return
	}
	d.log("%s does not list %q (%d resources served)", what, plural, len(resources.APIResources))
}

// RecordUsageFailure captures diagnostics for a failed wait on the temporary quota and
// returns cause unchanged, so the test still reports the original failure and keeps its
// own timeouts and assertions. It does nothing when cause is nil.
func RecordUsageFailure(ctx context.Context, f *framework.Framework, testcrd *crd.TestCrd, quotaName string, resourceName v1.ResourceName, cause error) error {
	diagnostics := collector{
		logf:           framework.Logf,
		crds:           testcrd.APIExtensionClient.ApiextensionsV1().CustomResourceDefinitions(),
		quotas:         f.ClientSet.CoreV1().ResourceQuotas(f.Namespace.Name),
		apiResources:   restAPIResourceGetter{client: f.ClientSet.Discovery().RESTClient()},
		budget:         defaultBudget,
		requestTimeout: defaultRequestTimeout,
	}
	target := newDiagnosticsTarget(testcrd.Crd, f.Namespace.Name, quotaName, resourceName)

	return diagnostics.recordFailure(ctx, target, cause)
}

// firstServedCustomResourceVersion returns the group/version the custom resource is
// expected to be discoverable under, or an empty string when no version is served.
func firstServedCustomResourceVersion(definition *apiextensionsv1.CustomResourceDefinition) string {
	for _, version := range definition.Spec.Versions {
		if version.Served {
			return schema.GroupVersion{Group: definition.Spec.Group, Version: version.Name}.String()
		}
	}
	return ""
}

// formatObjectIdentity renders the identity and revision metadata shared by every
// object the diagnostics read. Nothing else from the object metadata is logged.
func formatObjectIdentity(object metav1.Object) string {
	creationTimestamp := object.GetCreationTimestamp()
	return fmt.Sprintf("uid=%s resourceVersion=%s generation=%d creationTimestamp=%s",
		object.GetUID(), object.GetResourceVersion(), object.GetGeneration(), formatDiagnosticTime(creationTimestamp))
}

// formatCustomResourceDefinitionVersions renders the served and storage flags of every
// declared version.
func formatCustomResourceDefinitionVersions(versions []apiextensionsv1.CustomResourceDefinitionVersion) string {
	parts := make([]string, 0, len(versions))
	for _, version := range versions {
		parts = append(parts, fmt.Sprintf("%s(served=%t,storage=%t)", version.Name, version.Served, version.Storage))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// formatCustomResourceDefinitionConditions renders only the conditions listed in
// reportedConditions, reporting the ones that are absent as such.
func formatCustomResourceDefinitionConditions(conditions []apiextensionsv1.CustomResourceDefinitionCondition) string {
	parts := make([]string, 0, len(reportedConditions))
	for _, wanted := range reportedConditions {
		part := fmt.Sprintf("%s=<absent>", wanted)
		for _, condition := range conditions {
			if condition.Type == wanted {
				part = fmt.Sprintf("%s=%s(reason=%q,message=%q,lastTransitionTime=%s)",
					condition.Type, condition.Status, condition.Reason, condition.Message,
					formatDiagnosticTime(condition.LastTransitionTime))
				break
			}
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, " ")
}

// formatResourceList renders a ResourceList deterministically. Only resource names and
// quantities are rendered, never labels, annotations or any other object content.
func formatResourceList(list v1.ResourceList) string {
	names := make([]string, 0, len(list))
	for name := range list {
		names = append(names, string(name))
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		quantity := list[v1.ResourceName(name)]
		parts = append(parts, name+"="+quantity.String())
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// formatDiagnosticTime renders a timestamp in UTC, so timestamps collected from
// different objects can be compared with each other without a zone conversion.
func formatDiagnosticTime(timestamp metav1.Time) string {
	if timestamp.IsZero() {
		return "<none>"
	}
	return timestamp.UTC().Format(time.RFC3339)
}
