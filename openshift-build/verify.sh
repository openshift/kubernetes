#!/usr/bin/env bash

source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

# `make verify` requires recent bash (>= 4.3). If the system bash is
# not recent (e.g openshift ci and macos), download and compile a
# newer bash and make it available in the path.
RECENT_BASH="$( os::recent_bash )"
WHICH_BASH="$( which bash )"
if [[ "${RECENT_BASH}" != "${WHICH_BASH}" ]]; then
  BASH_PATH="$( dirname "${RECENT_BASH}" )"
  export PATH="${BASH_PATH}:${PATH}"
fi

bash --version

hack/make-rules/verify.sh
