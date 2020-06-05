#!/usr/bin/env bash

source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

# Upstream testing requires recent bash (>= 4.3). If the system bash
# is not recent (e.g openshift ci and macos), download and compile a
# newer bash and make it available in the path.
export PATH="$( os::deps::path_with_recent_bash )"

/usr/bin/env bash --version

mkdir -p "${BASETMPDIR}/test-artifacts"
KUBE_JUNIT_REPORT_DIR="${BASETMPDIR}/test-artifacts" hack/make-rules/test.sh
