# AGENTS.md - Guidance for AI Tools

This document provides essential information for AI coding assistants working with the OpenShift Kubernetes repository.

## Critical Context: This is a Fork

**This repository is OpenShift's fork of upstream Kubernetes (`kubernetes/kubernetes`).** The maintainers work to minimize the diff between upstream and this fork. AI tools must understand the special constraints and workflows that govern changes to this codebase.

## Why This Matters for AI Tools

Unlike most repositories where you can freely suggest changes anywhere in the codebase, this fork:

1. **Gets rebased regularly** against upstream Kubernetes (every minor version release)
2. **Must maintain minimal divergence** from upstream
3. **Has strict commit message conventions** that affect how changes survive rebases
4. **Requires changes to be categorized** as either upstream cherry-picks or downstream carries

**Before suggesting any code changes, AI tools MUST understand the carry patch process documented below.**

## Understanding the Rebase Process

Every time a new minor Kubernetes version is released (e.g., v1.30.0 → v1.31.0), the maintainers perform a rebase with these steps:

1. **Remove all carry patches** from the current branch
2. **Cherry-pick all new upstream commits** added between the two minor releases
3. **Reapply all carry patches** on top of the new upstream base

This means that **carry patches are a manual maintenance burden**. Each carry patch must be:
- Manually reapplied during every rebase
- Reviewed for conflicts with new upstream changes
- Updated if upstream code has changed in conflicting ways
- Maintained by the OpenShift team indefinitely

**Carry patches should only be added if a change absolutely cannot be accepted upstream.** Every carry patch represents ongoing manual work for the maintainers through every Kubernetes minor release.

For complete rebase procedures, see [REBASE.openshift.md](REBASE.openshift.md).

## Repository Structure

### Core Kubernetes Code (Upstream)
- `pkg/` - Core Kubernetes packages
- `cmd/` - Kubernetes binaries (kubelet, kube-apiserver, etc.)
- `staging/` - Staged Kubernetes libraries
- `api/` - API definitions
- `plugin/` - Kubernetes plugins
- `hack/` - Upstream build and development scripts

### OpenShift-Specific Code (Downstream)
- `openshift-hack/` - OpenShift-specific build and development scripts
- `openshift-kube-apiserver/` - OpenShift API server customizations
- `openshift-kube-controller-manager/` - OpenShift controller manager customizations
- `README.openshift.md` - OpenShift fork documentation
- `REBASE.openshift.md` - Rebase process documentation

## Commit Message Conventions (CRITICAL)

All commits to this repository MUST use one of these prefixes:

### `UPSTREAM: <carry>:`
Changes that should be reapplied in future rebases.
- **When to use**: Any change that needs to persist across Kubernetes version bumps
- **Lifecycle**: Manually reapplied during every rebase
- **Examples**:
  - `UPSTREAM: <carry>: Add support for OpenShift authentication`
  - `UPSTREAM: <carry>: Skip CPU resource status for workload-pinned pods`
- **Important**: These create ongoing maintenance burden - only use when the change cannot go upstream

### `UPSTREAM: <drop>:`
Changes that should NOT be included in future rebases.
- **When to use**: Any temporary or regeneratable change specific to this branch
- **Lifecycle**: Dropped during rebases
- **Common cases**:
  - Generated files that will be regenerated (`make update`)
  - Vendored dependencies that will be re-vendored
  - Behavior changes specific to just this minor release
  - Branch-specific fixes that won't apply to future versions
- **Examples**:
  - `UPSTREAM: <drop>: make update`
  - `UPSTREAM: <drop>: hack/update-vendor.sh`

### `UPSTREAM: 12345:`
Cherry-picks from upstream Kubernetes PRs.
- **When to use**: Backporting an upstream fix before the next rebase
- **Format**: The number is the upstream PR ID from `kubernetes/kubernetes`
- **Lifecycle**: Only picked if not yet in the new upstream base
- **Example**: `UPSTREAM: 134442: Fix ResourceQuota test for CRDs with long names`

### Direct upstream commits (no prefix)
Commits cherry-picked directly from `kubernetes/kubernetes` with their original commit messages.
- **When to use**: ONLY during the rebase process itself
- **AI tools should NEVER suggest these** in regular pull requests
- These are added by maintainers when rebasing to a new Kubernetes version

### Important Squashing Rules
- **OpenShift-specific files** (`openshift-hack/`, OpenShift READMEs) should be squashed into a single commit: `UPSTREAM: <carry>: Add OpenShift specific files`
- **Generated changes** must NEVER be mixed with code changes
- **Related carries** should be squashed together to simplify future rebases

### Enforcement
**Pull requests that do not follow these commit message conventions will be rejected by maintainers.** Every commit must use one of the `UPSTREAM:` prefixes listed above (except for direct upstream commits added during rebase).

## Rules for AI Tools Making Changes

### DO NOT Freely Modify Files
Unlike typical repositories, you cannot simply suggest changes anywhere. You must:

1. **Understand the intent**: Is this fixing an OpenShift-specific issue or an upstream bug?
2. **Choose the right approach**:
   - If it's an upstream bug → Should be fixed in `kubernetes/kubernetes` first, then cherry-picked
   - If it's OpenShift-specific → Use `UPSTREAM: <carry>:` prefix
   - If it's generated code → Will be handled by `make update` with `UPSTREAM: <drop>:`

3. **Use the correct commit prefix** based on the rules above

### Prefer Upstream Fixes
If you identify a bug that affects Kubernetes generally (not just OpenShift):
1. The fix should ideally go to upstream `kubernetes/kubernetes` first
2. Then cherry-pick to OpenShift using `UPSTREAM: <PR number>:` format
3. Only use `UPSTREAM: <carry>:` for truly OpenShift-specific behavior

### Generated Files Require Special Handling
These files are generated and must not be hand-edited:
- API code in `staging/`
- Generated clientsets
- OpenAPI specs
- Protobuf files

To update generated files:
```bash
make update
```
Commit with: `UPSTREAM: <drop>: make update`

### Dependency Updates
When updating Go dependencies:
```bash
hack/update-vendor.sh
```
Commit with: `UPSTREAM: <drop>: hack/update-vendor.sh`

## Build and Test Commands

AI tools should recommend these commands:

### Build
```bash
make
```

### Run Unit Tests
```bash
make test
```

### Run Integration Tests
```bash
make test-integration
```

### Verify Code Quality
```bash
make verify
```
This runs linting, generated file checks, and other verification.

### Update Generated Files
```bash
make update
```
Requires etcd installed (`hack/install-etcd.sh` or run in container).

## Key Directories to Avoid Modifying Carelessly

### High-Risk Areas (Upstream Code)
Changes here create rebase burden and should be minimized:
- `pkg/kubelet/`
- `pkg/scheduler/`
- `pkg/controller/`
- `pkg/apiserver/`
- `staging/src/k8s.io/*/`

If changes are needed here, strongly consider:
1. Can this be fixed upstream first?
2. Is there a way to achieve this with less invasive changes?
3. Can this be done via a plugin or extension point?

### Safe Areas (Downstream Code)
Changes here are expected:
- `openshift-hack/`
- `openshift-kube-apiserver/`
- `openshift-kube-controller-manager/`
- OpenShift-specific documentation

## Best Practices for AI Tools

1. **Read before writing**: Always examine existing code and commit history before suggesting changes
2. **Understand the context**: Is this addressing an upstream or downstream concern?
3. **Minimize divergence**: Prefer smaller, more targeted changes
4. **Use correct prefixes**: Every commit must have an `UPSTREAM:` prefix
5. **Check for upstream fixes**: Before implementing a fix, search if it exists upstream
6. **Suggest testing**: Always recommend running `make test` and `make verify`
7. **Consider rebase impact**: How will this change survive future rebases?

## Common Pitfalls to Avoid

1. **Mixing generated and code changes** in the same commit
2. **Using wrong UPSTREAM prefix** (causes rebase issues)
3. **Modifying core Kubernetes code** without understanding if it should be upstream
4. **Skipping `make update`** after API or dependency changes
5. **Not running `make verify`** before suggesting the change is complete
6. **Suggesting changes to vendored code** in `vendor/` (should use dependency updates)

## Examples of Good vs. Bad Suggestions

### ❌ Bad: Suggesting direct changes to core code without context
```
"I'll fix this bug in pkg/scheduler/core.go by changing line 123..."
```
**Problem**: Doesn't consider if this should be an upstream fix.

### ✅ Good: Understanding the context first
```
"This appears to be a general Kubernetes issue. I recommend:
1. Check if this is already fixed upstream in kubernetes/kubernetes
2. If not, consider opening an upstream PR first
3. If OpenShift needs it urgently, cherry-pick with: UPSTREAM: <PR>: <description>
4. If it's truly OpenShift-specific, use: UPSTREAM: <carry>: <description>"
```

### ❌ Bad: Mixing concerns
```
UPSTREAM: <carry>: Fix authentication and run make update

Changes:
- Modified pkg/apiserver/auth.go
- Regenerated API files
- Updated vendor/
```
**Problem**: Mixes code changes with generated file updates.

### ✅ Good: Separate commits
```
Commit 1:
UPSTREAM: <carry>: Fix OpenShift authentication integration

Changes:
- Modified pkg/apiserver/auth.go

Commit 2:
UPSTREAM: <drop>: make update

Changes:
- Regenerated API files

Commit 3:
UPSTREAM: <drop>: hack/update-vendor.sh

Changes:
- Updated vendor/
```

## Questions AI Tools Should Ask

When a user requests a change, consider:

1. **Is this fixing a Kubernetes bug or adding OpenShift-specific behavior?**
2. **Does this change already exist in a newer version of upstream Kubernetes?**
3. **Will this change need to survive future rebases?**
4. **Are there generated files that need updating after this change?**
5. **Does this change require updates to dependencies?**
6. **What tests should be added or updated?**

## Resources for AI Tools

- [README.openshift.md](README.openshift.md) - Fork overview and cherry-pick process
- [REBASE.openshift.md](REBASE.openshift.md) - Detailed rebase procedures
- [CONTRIBUTING.md](CONTRIBUTING.md) - Kubernetes contribution guidelines
- [upstream k8s.io/community](https://github.com/kubernetes/community) - Kubernetes community docs

## Testing Your Changes

Before suggesting a change is complete, ensure:

```bash
# Build succeeds
make

# Tests pass
make test

# Verification passes
make verify

# Integration tests pass (when applicable)
make test-integration
```

## Summary for AI Tools

**The most important thing to remember**: This is not a normal repository. It's a carefully maintained fork that gets regularly rebased. Every change must be made with the understanding of:
- Whether it belongs upstream or downstream
- How it will survive future rebases
- What commit message prefix it requires

When in doubt, ask the user or suggest looking at recent commit history for similar changes to understand the expected pattern.
