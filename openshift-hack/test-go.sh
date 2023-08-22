#!/usr/bin/env bash

# shellcheck source=openshift-hack/lib/init.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/init.sh"

ulimit -c unlimited

ARTIFACTS="${ARTIFACTS:-/tmp/artifacts}"
mkdir -p "${ARTIFACTS}"

save_core() {
  echo "Checking for core files in $(pwd)..."
  for i in $(find ./ -type f -iname core); do
    if [ -f "$i" ] ; then
        echo "Found core file $i..."
        cp "$i" "${ARTIFACTS}"
    fi
  done
}
trap "save_core" EXIT SIGINT

export KUBERNETES_SERVICE_HOST=
export KUBE_JUNIT_REPORT_DIR="${ARTIFACTS}"
export KUBE_KEEP_VERBOSE_TEST_OUTPUT=y
export KUBE_RACE=-race
export KUBE_TEST_ARGS='-p 8'
export KUBE_TIMEOUT='--timeout=360s'

make test
