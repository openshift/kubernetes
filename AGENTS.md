# AI Agent Instructions for openshift/kubernetes

## What This Repo Is
OpenShift's fork of upstream Kubernetes (`k8s.io/kubernetes`) with downstream patches applied. This is a periodically-rebased fork, not an independent codebase. Changes land in three ways: periodic rebases from upstream, cherry-picked upstream commits, or downstream carry patches.

## Critical Rules

**Always:**
- Run `make verify` before completing changes - checks formatting, linting, and generated code
- Run `make test` for unit tests, especially after modifying core functionality
- Use `UPSTREAM:` commit message prefixes (see [Commit Messages](#commit-messages))
- When discussing code, use file:line notation for precise references (e.g., `pkg/kubelet/kuberuntime/kuberuntime_manager.go:345`)
- Check if your change should go upstream first - most features belong in upstream Kubernetes

**Ask First:**
- Adding new downstream-only features (should be rare; justify why not upstream)
- Modifying rebase/cherry-pick tooling in `openshift-hack/`
- Changes to OpenShift-specific cloud provider integrations
- Structural changes to the build system or vendoring

**Never:**
- Modify `vendor/` directly - use `go mod tidy && go mod vendor`
- Commit without proper `UPSTREAM:` prefixes in commit messages
- Add OpenShift-specific imports to upstream code without strong justification
- Skip `make update` after modifying generated code or bindata

## Commit Messages

All commits to `openshift/kubernetes` must use one of these prefixes:

- `UPSTREAM: <PR number>:` - Cherry-pick of upstream Kubernetes PR (e.g., `UPSTREAM: 138075:`)
- `UPSTREAM: <carry>:` - Downstream patches that maintain OpenShift-specific behavior across rebases
- `UPSTREAM: <drop>:` - Generated code or temporary changes omitted in next rebase

Examples from git log:
```
UPSTREAM: 138075: apiserver: cache etcd storage monitors
UPSTREAM: <carry>: add kubernetes/conformance umbrella suite
UPSTREAM: <drop>: bump library-go
```

## Repository Structure

**Core Directories:**
- `cmd/` - Kubernetes binaries (kubelet, kube-apiserver, etc.)
- `pkg/` - Core Kubernetes packages
- `staging/` - Staging repos for client-go, apimachinery, etc.
- `test/e2e/` - End-to-end test suites
- `openshift-hack/` - OpenShift-specific build and rebase tooling
- `vendor/` - Vendored dependencies

## Key Build Commands

Standard Kubernetes commands (see [hack/README.md](hack/README.md)):
```bash
make                  # Build all binaries
make test             # Run unit tests
make verify           # Run formatting, linting, dependency checks (runs hack/verify-all.sh)
make update           # Update generated code and bindata (runs hack/update-all.sh)
make test-integration # Run integration tests
```

OpenShift-specific:
```bash
openshift-hack/build-go.sh              # OpenShift build wrapper with version injection
openshift-hack/build-rpms.sh            # RPM package build
openshift-hack/test-go.sh               # OpenShift test wrapper
openshift-hack/test-integration.sh      # Integration tests with etcd setup
openshift-hack/test-kubernetes-e2e.sh   # E2E tests against an OpenShift cluster
openshift-hack/verify.sh                # OpenShift verification checks
openshift-hack/conformance-k8s.sh       # Kubernetes conformance test runner
```

## Testing

**Test Types:**
- Unit tests: `make test` or `go test ./pkg/...`
- Integration tests: `make test-integration`
- E2E tests: `test/e2e/` (requires cluster; see [test/e2e/README.md](test/e2e/README.md))
- OpenShift conformance: `openshift-hack/conformance-k8s.sh`

E2E tests use the upstream Kubernetes e2e framework (see [test/e2e/README.md](test/e2e/README.md)). All tests must be SIG-owned and follow ownership policies. OpenShift adds conformance suite coverage via `test/e2e/`.

## Common Patterns

**Cherry-picking upstream commits:**
1. Find the upstream PR number
2. Cherry-pick to downstream: `git cherry-pick <commit>`
3. Prefix commit message with `UPSTREAM: <PR>:`
4. Check if code has changed significantly since last rebase (see [README.openshift.md](README.openshift.md))

**Carry patches:**
- Used for OpenShift-specific behavior (cloud providers, authentication, etc.)
- Retained across rebases
- Should be minimal - prefer upstreaming when possible
- Examples: `git log --grep="UPSTREAM: <carry>"`

## What NOT to Do

- Add features that belong upstream to this fork first
- Create downstream APIs when upstream APIs exist
- Modify OWNERS files without team approval
- Cherry-pick uncommitted or WIP upstream changes
- Make breaking changes to existing carry patches without rebase team coordination

## Documentation

- [README.openshift.md](README.openshift.md) - OpenShift fork overview and cherry-pick guidance
- [REBASE.openshift.md](REBASE.openshift.md) - Rebase procedures for maintainers
- [ARCHITECTURE.md](ARCHITECTURE.md) - Architecture and design patterns
- [staging/README.md](staging/README.md) - Staging repository publishing process
- Upstream docs: https://kubernetes.io/docs/home/
- Community: https://git.k8s.io/community

## High-Risk Areas

**Rebase-sensitive code:**
- Authentication/authorization integrations
- Cloud provider implementations
- OpenShift-specific API extensions
- Vendored library-go dependencies

These areas change frequently and require careful conflict resolution during rebases.

## Getting Help

- OpenShift Kubernetes team: See [DOWNSTREAM_OWNERS](DOWNSTREAM_OWNERS) for reviewers and approvers
- Approvers manage rebases and carry patches - consult before adding new downstream-only features
- Upstream Kubernetes: https://kubernetes.io/community/
- Support channels: https://kubernetes.io/docs/tasks/debug/ (troubleshooting)
