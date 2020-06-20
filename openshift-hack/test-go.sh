#!/usr/bin/env bash

# shellcheck source=openshift-hack/lib/init.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/init.sh"

# Upstream testing requires recent bash (>= 4.3). If the system bash
# is not recent (e.g openshift ci and macos), download and compile a
# newer bash and make it available in the path.
PATH="$( os::deps::path_with_recent_bash )"
export PATH

/usr/bin/env bash --version

if [[ -n "${KUBE_JUNIT_REPORT_DIR}" ]]; then
  # junit output will only be created if this tool is available
  make WHAT=vendor/gotest.tools/gotestsum
fi

make test
