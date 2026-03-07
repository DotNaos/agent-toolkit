#!/bin/zsh
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPLETIONS_DIR="${HOME}/.zsh/completions"
BIN_DIR="${REPO_ROOT}/bin"

mkdir -p "$COMPLETIONS_DIR" "$BIN_DIR"

go build -o "${BIN_DIR}/agent-chat" "${REPO_ROOT}/cmd/agent-chat"
go build -o "${BIN_DIR}/agent-memory" "${REPO_ROOT}/cmd/agent-memory"
go build -o "${BIN_DIR}/ui-loop" "${REPO_ROOT}/cmd/ui-loop"
go build -o "${BIN_DIR}/agent-hub" "${REPO_ROOT}/cmd/agent-hub"
go build -o "${BIN_DIR}/agent-delegate" "${REPO_ROOT}/cmd/agent-delegate"

install_completion() {
  local command_name="$1"
  local binary_path="$2"
  local function_name="${3:-_${command_name}}"
  local append_compdef="${4:-true}"
  local output_path="${COMPLETIONS_DIR}/_${command_name}"
  local repo_bin_path="bin/${command_name}"
  local local_path="./bin/${command_name}"
  local absolute_path="${BIN_DIR}/${command_name}"
  local user_local_path="${HOME}/.local/bin/${command_name}"

  "${binary_path}" completion zsh > "${output_path}"
  if [[ "${append_compdef}" == "true" ]]; then
    printf '\ncompdef %s %s %s %s %s %s\n' "${function_name}" "${command_name}" "${repo_bin_path}" "${local_path}" "${absolute_path}" "${user_local_path}" >> "${output_path}"
  fi
}

install_completion "agent-chat" "${BIN_DIR}/agent-chat"
install_completion "agent-memory" "${BIN_DIR}/agent-memory"
install_completion "ui-loop" "${BIN_DIR}/ui-loop"
install_completion "agent-hub" "${BIN_DIR}/agent-hub"
install_completion "agent-delegate" "${BIN_DIR}/agent-delegate" "_agent_delegate" "false"

echo "Installed zsh completions to ${COMPLETIONS_DIR}"
