#!/usr/bin/env bash

source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

# Upstream verify requires recent bash (>= 4.3). If the system bash is
# not recent (e.g openshift ci and macos), download and compile a
# newer bash and make it available in the path.
export PATH="$( os::deps::path_with_recent_bash )"

/usr/bin/env bash --version

# Upstream verify requires protoc (>= 3.0.0). If not present, download
# a recent version and make it available in the path.
export PATH="$( os::deps::path_with_protoc )"

# Attempt to verify without docker if it is not available.
export OS_RUN_WITHOUT_DOCKER=
if ! which docker &> /dev/null; then
  os::log::warning "docker not available, attempting to run verify without it"
  export OS_RUN_WITHOUT_DOCKER=y
fi

hack/make-rules/verify.sh
