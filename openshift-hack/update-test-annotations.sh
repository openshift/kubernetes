#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

KUBE_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
source "${KUBE_ROOT}/hack/lib/init.sh"

kube::golang::setup_env

# Build k8s-test-ext in order to generate the labels mapping
"${KUBE_ROOT}"/hack/make-rules/build.sh "openshift-hack/cmd/k8s-tests-ext"

# Update e2e test annotations and labels that indicate openshift compatibility
go generate -mod vendor ./openshift-hack/e2e
