# Architecture: openshift/kubernetes

## Overview

`openshift/kubernetes` is OpenShift's fork of upstream Kubernetes (`k8s.io/kubernetes`), maintained through periodic rebases and selective carry patches. This is not a standalone project but rather a carefully managed fork that preserves OpenShift-specific functionality while tracking upstream releases.

**Core principle:** Minimize downstream divergence. Most changes belong upstream first; downstream patches exist only where OpenShift-specific integration is required.

## Fork Architecture

### Rebase Strategy

The repository follows a **periodic rebase model**:

1. **Upstream tracking**: Rebased against official Kubernetes release tags (e.g., `v1.31.0`)
2. **Carry patches**: OpenShift-specific changes retained across rebases using `UPSTREAM: <carry>:` commits
3. **Cherry-picks**: Critical upstream fixes backported with `UPSTREAM: <PR>:` commits
4. **Drop commits**: Generated code and temporary changes marked `UPSTREAM: <drop>:`

The rebase process is semi-automated via `openshift-hack/rebase.sh`, which creates new branches from upstream tags and reapplies carry patches.

### Commit Categorization

Every commit falls into one of three categories based on its `UPSTREAM:` prefix:

| Prefix | Purpose | Lifecycle | Examples |
|--------|---------|-----------|----------|
| `UPSTREAM: <PR>:` | Cherry-picked upstream fix | Dropped at next rebase (already in base) | Bug fixes, security patches |
| `UPSTREAM: <carry>:` | OpenShift-specific patch | Retained across rebases | Feature gates, authentication, cloud providers |
| `UPSTREAM: <drop>:` | Generated/temporary code | Dropped at next rebase | Dependency bumps, generated bindata |

## OpenShift-Specific Components

### 1. Enablement Layer (`openshift-kube-apiserver/enablement/`)

Runtime detection and configuration for OpenShift-specific behavior:

- **`IsOpenShift()`**: Boolean flag determining whether OpenShift extensions are active
- **`ForceOpenShift()`**: Bootstraps OpenShift mode with `KubeAPIServerConfig`
- **Post-start hooks**: Registry for OpenShift-specific initialization hooks

This layer allows the same Kubernetes codebase to run in both vanilla and OpenShift modes.

### 2. API Server Patches (`openshift-kube-apiserver/`)

Key subsystems:

- **`admission/`**: OpenShift-specific admission plugins:
  - `admissionenablement/` - Admission plugin registration
  - `namespaceconditions/` - Namespace lifecycle conditions
  - `storage/` - CSI inline volume security, performant security policy
  - `customresourcevalidation/` - OpenShift CR validation
  - `network/`, `route/`, `scheduler/`, `autoscaling/` - Domain-specific admission
- **`authorization/`**: OpenShift RBAC extensions
- **`filters/`**: Request filtering (API request counting for telemetry)
- **`openshiftkubeapiserver/`**: Integration glue:
  - `patch.go`: Applies OpenShift configuration to generic API server
  - `patch_handlerchain.go`: Injects OpenShift filters into request chain
  - `wellknown_oauth.go`: OAuth discovery endpoint support
  - `sdn_readyz_wait.go`: Delays readiness until SDN is configured

### 3. Controller Manager Extensions (`openshift-kube-controller-manager/`)

Additional controllers for OpenShift features:

- **`servicecacertpublisher/`**: Publishes service CA certificates to namespaces for service-to-service TLS

### 4. Feature Gates (`pkg/features/openshift_features.go`)

OpenShift-specific feature gates injected into the standard Kubernetes feature gate registry:

- `RouteExternalCertificate` (Alpha, 4.16+) - External certificate support for routes
- `MinimumKubeletVersion` (Alpha, 4.19+) - Enforce minimum kubelet version
- `StoragePerformantSecurityPolicy` (Alpha, 4.20+) - Performance-focused storage security policy

These control OpenShift-only functionality without upstreaming unused gates.

### 5. Cloud Provider Integration

Carry patches for OpenShift-supported cloud providers:
- IBM Cloud provider registration
- Azure Stack compatibility
- Platform detection integration with OpenShift Infrastructure CR

### 6. Downstream Binaries

In addition to standard Kubernetes binaries in `cmd/`, this fork adds:

- **`cmd/watch-termination`**: Monitors kubelet termination and logs events during graceful shutdown. Built into the hyperkube image alongside core components.

### 7. Authentication Extensions

- **Email claim validation**: Enforces `email_verified` when `email` is used in OIDC username expressions
- **Custom authentication plugins**: Integration with OpenShift OAuth server

## Staging Repositories

The `staging/` directory contains source for 40+ published Kubernetes libraries (e.g., `k8s.io/client-go`, `k8s.io/api`, `k8s.io/apiserver`). These are the authoritative source and are periodically published to standalone repositories. See [staging/README.md](staging/README.md) for the complete list and publishing process.

**Key staging repos:**
- `k8s.io/api` - API types
- `k8s.io/apimachinery` - Shared machinery
- `k8s.io/client-go` - Kubernetes client library (see [ARCHITECTURE.md](staging/src/k8s.io/client-go/ARCHITECTURE.md))
- `k8s.io/apiserver` - API server framework (see [ARCHITECTURE.md](staging/src/k8s.io/apiserver/ARCHITECTURE.md))
- `k8s.io/kubectl` - kubectl command

These are vendored by other OpenShift components and published as separate modules.

## Build System

### OpenShift Build Wrappers

The `openshift-hack/` directory provides OpenShift-specific tooling:

| Script | Purpose |
|--------|---------|
| `build-go.sh` | OpenShift build wrapper with version injection |
| `build-rpms.sh` | RPM package build |
| `test-go.sh` | Test harness for OpenShift CI |
| `test-integration.sh` | Integration tests with etcd setup |
| `test-kubernetes-e2e.sh` | E2E tests against an OpenShift cluster |
| `verify.sh` | Additional verification checks beyond upstream |
| `conformance-k8s.sh` | Kubernetes conformance test runner |
| `rebase.sh` | Semi-automated rebase orchestration (see [REBASE.openshift.md](REBASE.openshift.md)) |
| `create-or-update-rebase-branch.sh` | Rebase branch management |

### Container Images

Container image definitions live in `openshift-hack/images/`:

| Image | Purpose |
|-------|---------|
| `hyperkube` | Main image containing kube-apiserver, kube-controller-manager, kube-scheduler, kubelet, watch-termination, k8s-tests-ext, and kubensenter |
| `kube-proxy` | Kube-proxy image |
| `tests` | Test image for CI |
| `installer-kube-apiserver-artifacts` | Static assets consumed by the OpenShift installer |

Images are built from `Dockerfile.rhel` files using the OpenShift CI builder base image. The CI build root is configured via [`.ci-operator.yaml`](.ci-operator.yaml).

### Standard Kubernetes Makefile

The upstream `Makefile` is preserved with standard targets (see [hack/README.md](hack/README.md)):
- `make` / `make all` - Build binaries
- `make test` - Run unit tests
- `make verify` - Linting and generated code checks
- `make update` - Regenerate code (deepcopy, conversions, OpenAPI)

## Testing Architecture

### Test Categories

1. **Unit tests**: Standard Go tests in `*_test.go` files
2. **Integration tests**: `test/integration/` - requires etcd but not full cluster
3. **E2E tests**: `test/e2e/` - full Kubernetes e2e test suite (see [test/e2e/README.md](test/e2e/README.md))
4. **OpenShift conformance**: `openshift-hack/conformance-k8s.sh`

### Test Ownership

All e2e tests must be SIG-owned and live under `test/e2e/{sig}/` with proper OWNERS files. See [test/e2e/README.md](test/e2e/README.md) for ownership policies enforced by `hack/verify-e2e-test-ownership.sh`.

### Test Extension Framework (`openshift-hack/cmd/k8s-tests-ext`)

Filters which upstream e2e tests run in OpenShift CI:

- **Disabled tests**: Skips tests for alpha features, unimplemented features, and known incompatibilities with OpenShift
- **Environment selectors**: Matches tests to platform capabilities (cloud provider, network plugin, etc.)
- **Labels**: Categorizes tests for selective execution in CI jobs

Built into the hyperkube image and invoked by CI to generate the filtered test suite.

### Import Verification (`openshift-hack/cmd/go-imports-diff`)

Validates that e2e test packages under `test/e2e/` only import expected dependencies, preventing unintended coupling between test packages.

### OpenShift Conformance Suite

Added via carry patch (`UPSTREAM: <carry>: add kubernetes/conformance umbrella suite`):
- Validates OpenShift-specific features against Kubernetes conformance requirements
- Runs in OpenShift CI with disabled container security restrictions
- Located in `openshift-hack/e2e/`

## Key Design Decisions

| Decision | Rationale | Tradeoffs |
|----------|-----------|-----------|
| **Periodic rebases over continuous merge** | Reduces drift, maintains upstream compatibility | Rebase effort every release cycle |
| **Enablement flag over fork** | Single codebase for vanilla K8s and OpenShift | Conditional logic in core paths |
| **Carry patches minimized** | Easier rebases, simpler maintenance | Some features delayed until upstream accepts them |
| **Automated rebase tooling** | Reduces human error, speeds rebases | Complex tooling to maintain |
| **UPSTREAM commit prefixes required** | Clear provenance, automated rebase handling | Strict process discipline needed |
| **Vendored dependencies** | Reproducible builds, offline capability | Large vendor directory in git |
| **Staging repos in-tree** | Simplifies publishing, version coherence | Monorepo complexity |

## Dependency Flow

```
upstream k8s.io/kubernetes (tag: v1.31.0)
    ↓
openshift/kubernetes (periodic rebase)
    ↓
staging repos (k8s.io/client-go, etc.)
    ↓
OpenShift components:
  - openshift/api
  - openshift/apiserver-library-go  
  - openshift/library-go
  - cluster operators (image-registry, authentication, etc.)
    ↓
OpenShift distribution
```

## High-Risk Areas

### Rebase-Sensitive Code

These areas change frequently upstream and require careful conflict resolution:

- **Scheduler** (`pkg/scheduler/`): High upstream activity
- **API machinery** (`staging/src/k8s.io/apimachinery/`): Foundational changes
- **Authentication/authorization**: OpenShift-specific patches interact with evolving upstream code
- **Feature gates**: Divergence between upstream and OpenShift gates
- **Cloud providers**: Upstream provider refactoring vs OpenShift integrations

### Carry Patch Hotspots

Areas with persistent downstream patches:

- API server admission chain
- Feature gate registration
- TLS profile configuration (TLS 1.3 support)
- etcd health checking (retry monitors)
- Audit event annotations (readiness/termination phase marking)
- CPU resource handling (workload pinning, shared CPUs)
- Volume group snapshots

## Integration Points

### With OpenShift API

OpenShift-specific types from `github.com/openshift/api`:
- `kubecontrolplane/v1.KubeAPIServerConfig` - API server configuration
- Feature gate definitions
- Cloud provider configurations

### With library-go

Shared OpenShift libraries from `github.com/openshift/library-go`:
- Controller framework
- Certificate rotation
- Resource application helpers

### With Cluster Operators

This repository provides the Kubernetes control plane components consumed by:
- `cluster-kube-apiserver-operator` - Manages kube-apiserver deployment
- `cluster-kube-controller-manager-operator` - Manages kube-controller-manager
- `cluster-kube-scheduler-operator` - Manages kube-scheduler

## Observability

### Metrics

OpenShift-specific metrics added via carry patches:
- API request counting by deprecated API version
- Controller-specific performance metrics

### Logging

- Structured logging via `klog/v2`
- OpenShift extensions log only deprecated API requests (vs all usage)
- Audit event annotations for unready/terminating API servers

## Security Considerations

### Admission Control

OpenShift adds:
- Security Context Constraints (SCC) admission
- Namespace lifecycle enforcement
- Storage security policy plugin

### TLS Configuration

Carry patches support:
- TLS 1.3 explicit configuration
- OpenShift TLS security profiles (Old, Intermediate, Modern, Custom)

### Secret Mutation

Limited type mutation allowed for specific secret types (service account tokens) to support OpenShift secret management.

## Performance Optimizations

Carry patches include:

- **etcd client monitoring caching**: Reduces client recreation overhead during metrics scrapes
- **SELinux conflict caching**: Pre-parse labels, maintain reverse index for faster conflict detection
- **Extended test timeouts**: Accommodate parallel test load in CI

## Future Direction

### Upstream Contribution Goals

OpenShift team actively works to upstream carry patches:
- Feature gates → Kubernetes core features
- Cloud provider improvements → kubernetes/cloud-provider-* repos
- Authentication enhancements → Kubernetes SIGs

### Rebase Cadence

- **Target**: Rebase to new Kubernetes minor version within 2-4 weeks of upstream release
- **Process**: Coordinated freeze period on `openshift/kubernetes` during active rebase
- **Communication**: Email announcements to `aos-devel` mailing list

## References

- [README.openshift.md](README.openshift.md) - Fork overview and cherry-pick process
- [REBASE.openshift.md](REBASE.openshift.md) - Detailed rebase procedures for maintainers
- [AGENTS.md](AGENTS.md) - AI agent development guidelines
- [DOWNSTREAM_OWNERS](DOWNSTREAM_OWNERS) - Rebase team (approvers/reviewers)
- [staging/README.md](staging/README.md) - Staging repository structure and publishing
- Upstream Kubernetes docs: https://kubernetes.io/docs/
- OpenShift docs: https://docs.openshift.com/
