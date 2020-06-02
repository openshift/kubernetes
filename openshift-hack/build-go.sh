#!/usr/bin/env bash

STARTTIME=$(date +%s)
source "$(dirname "${BASH_SOURCE}")/lib/init.sh"

pushd "${OS_ROOT}" > /dev/null
  make all WHAT='cmd/kube-apiserver cmd/kube-controller-manager cmd/kube-scheduler cmd/kubelet'
popd > /dev/null

os::build::version::git_vars

if [[ "${OS_GIT_TREE_STATE:-dirty}" == "clean"  ]]; then
  # only when we are building from a clean state can we claim to
  # have created a valid set of binaries that can resemble a release
  mkdir -p "${OS_OUTPUT_RELEASEPATH}"
  echo "${OS_GIT_COMMIT}" > "${OS_OUTPUT_RELEASEPATH}/.commit"
fi

ret=$?; ENDTIME=$(date +%s); echo "$0 took $(($ENDTIME - $STARTTIME)) seconds"; exit "$ret"
