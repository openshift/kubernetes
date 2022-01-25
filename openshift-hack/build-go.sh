#!/usr/bin/env bash

STARTTIME=$(date +%s)

# shellcheck source=openshift-hack/lib/init.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib/init.sh"

os::build::version::git_vars

# creating KUBE_GIT_VERSION_FILE will allow kube build scripts to
# pick envs we explicitly set it to
export KUBE_GIT_VERSION_FILE=${OS_ROOT}/.dockerized-kube-version-defs
cat <<EOF >"${KUBE_GIT_VERSION_FILE}"
KUBE_GIT_COMMIT='${OS_GIT_COMMIT}'
KUBE_GIT_TREE_STATE='${OS_GIT_TREE_STATE}'
KUBE_GIT_VERSION='${OS_GIT_VERSION}'
KUBE_GIT_MAJOR='${OS_GIT_MAJOR}'
KUBE_GIT_MINOR='${OS_GIT_MINOR}'
EOF

pushd "${OS_ROOT}" > /dev/null || exit 1
  make all WHAT='cmd/kube-apiserver cmd/kube-controller-manager cmd/kube-scheduler cmd/kubelet'
popd > /dev/null || exit 1

if [[ "${OS_GIT_TREE_STATE:-dirty}" == "clean"  ]]; then
  # only when we are building from a clean state can we claim to
  # have created a valid set of binaries that can resemble a release
  mkdir -p "${OS_OUTPUT_RELEASEPATH}"
  echo "${OS_GIT_COMMIT}" > "${OS_OUTPUT_RELEASEPATH}/.commit"
fi

ret=$?; ENDTIME=$(date +%s); echo "$0 took $((ENDTIME - STARTTIME)) seconds"; exit "$ret"
