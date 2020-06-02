#!/usr/bin/env bash

# os::deps::path_with_recent_bash returns a path that includes a
# recent bash (~ 5.x).
#
# Globals:
#  None
# Arguments:
#  None
# Returns:
#  A path that includes a recent bash.
function os::deps::path_with_recent_bash() {
  local path_with_bash="${PATH}"
  local bash_version
  bash_version="$( bash --version | head -n 1 | awk '{print $4}' )"
  if [[ ! "${bash_version}" =~ 5.* ]]; then
    recent_bash_path="${BASETMPDIR}/recent-bash"
    mkdir -p "${recent_bash_path}"
    if [[ ! -f "${recent_bash_path}/bash" ]]; then
      pushd "${recent_bash_path}" > /dev/null || exit 1
        curl -LO https://ftp.gnu.org/gnu/bash/bash-5.0.tar.gz
        tar xf bash-5.0.tar.gz
        pushd bash-5.0 > /dev/null || exit 1
          ./configure > configure.log
          make > make.log
          cp bash ../
        popd > /dev/null || exit 1
      popd > /dev/null || exit 1
    fi
    path_with_bash="${recent_bash_path}:${path_with_bash}"
  fi
  echo "${path_with_bash}"
}
readonly -f os::deps::path_with_recent_bash

# os::deps::protoc returns a path that includes protoc.
#
# Globals:
#  None
# Arguments:
#  None
# Returns:
#  The path that includes protoc.
function os::deps::path_with_protoc() {
  local path="${PATH}"
  if ! which protoc &> /dev/null; then
    local protoc_path="${BASETMPDIR}/protoc"
    mkdir -p "${protoc_path}"
    if [[ ! -f "${protoc_path}/bin/protoc" ]]; then
      pushd "${protoc_path}" > /dev/null || exit 1
        curl -LO https://github.com/protocolbuffers/protobuf/releases/download/v3.12.3/protoc-3.12.3-linux-x86_64.zip
        unzip protoc-3.12.3-linux-x86_64.zip
      popd > /dev/null || exit 1
    fi
    path="${PATH}:${protoc_path}/bin"
  fi
  echo "${path}"
}
readonly -f os::deps::path_with_protoc
