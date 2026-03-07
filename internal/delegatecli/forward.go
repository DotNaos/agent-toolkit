package delegatecli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"agent-toolkit/internal/delegaterun"
	"agent-toolkit/internal/shared/cliio"
)

func FormatErrorJSON(err error) string {
	return cliio.FormatErrorJSON(err)
}

func Execute(args []string) error {
	if isCompletionRequest(args) {
		return emitCompletion(args)
	}

	toolDir, err := findToolDir()
	if err != nil {
		return err
	}

	forwardArgs, err := normalizeArgs(args)
	if err != nil {
		return err
	}
	callerDir, err := os.Getwd()
	if err != nil {
		return err
	}

	cmdArgs := append([]string{"run", "src/index.ts"}, forwardArgs...)
	cmd := exec.Command("bun", cmdArgs...)
	cmd.Dir = toolDir
	cmd.Env = append(os.Environ(), "AGENT_DELEGATE_CALLER_CWD="+callerDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("agent-delegate TS CLI failed: %w", err)
	}
	return nil
}

func findToolDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	toolDir := filepath.Join(cwd, "tools", "agent-delegate")
	if _, err := os.Stat(filepath.Join(toolDir, "src", "index.ts")); err != nil {
		return "", fmt.Errorf("agent-delegate TS CLI not found at %s", toolDir)
	}
	return toolDir, nil
}

func normalizeArgs(args []string) ([]string, error) {
	out := append([]string(nil), args...)
	for index := 0; index < len(out); index += 1 {
		if out[index] != "--config" {
			continue
		}
		if index+1 >= len(out) {
			return nil, fmt.Errorf("--config requires a value")
		}
		resolved, err := resolveConfigPath(out[index+1])
		if err != nil {
			return nil, err
		}
		out[index+1] = resolved
		return out, nil
	}

	resolved, err := resolveConfigPath("agent-delegate.json")
	if err != nil {
		return nil, err
	}
	return append(out, "--config", resolved), nil
}

func resolveConfigPath(rawPath string) (string, error) {
	if filepath.IsAbs(rawPath) {
		return rawPath, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, rawPath), nil
}

func isCompletionRequest(args []string) bool {
	return len(args) >= 1 && args[0] == "completion"
}

func emitCompletion(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("completion requires a shell argument")
	}
	switch strings.ToLower(strings.TrimSpace(args[1])) {
	case "zsh":
		_, err := os.Stdout.WriteString(buildZshCompletion())
		return err
	default:
		return fmt.Errorf("unsupported completion shell %q", args[1])
	}
}

func buildZshCompletion() string {
	modelChoices := strings.Join(loadModelChoices(), "\n    ")
	targets := strings.Join(completionTargets(), " ")
	return fmt.Sprintf(`#compdef agent-delegate

_agent-delegate() {
  local curcontext="$curcontext" state
  typeset -A opt_args

  local -a subcommands
  local -a model_choices
  local -a run_flags
  local -a remaining_run_flags
  subcommands=(
    'run:Run a one-shot delegated task'
    'list-adapters:List configured adapters and models'
    'validate-config:Validate the delegate config file'
    'completion:Emit shell completion script'
  )
  model_choices=(
    %s
  )
  run_flags=(
    '-p:Delegated prompt text'
    '--prompt:Delegated prompt text'
    '-m:Delegated model in adapter/model form'
    '--model:Delegated model in adapter/model form'
    '-r:Path to request JSON or - for stdin'
    '--request:Path to request JSON or - for stdin'
    '-c:Path to delegate config JSON'
    '--config:Path to delegate config JSON'
    '--mode:Execution mode'
    '--cwd:Working directory for relative context paths'
    '--adapter:Delegate adapter'
    '--timeout-sec:Request timeout in seconds'
    '--approval-granted:Bypass approval block for guarded_execution'
    '-j:Emit compact JSON instead of pretty-printed JSON'
    '--json:Emit compact JSON instead of pretty-printed JSON'
  )

  if (( CURRENT == 2 )); then
    _describe 'subcommand' subcommands
    return
  fi

  local subcommand="${words[2]}"
  case "$subcommand" in
    run)
      local prev_word=""
      local current_word="${words[CURRENT]}"
      local has_model=0
      local has_prompt=0
      local has_request=0
      local has_config=0
      local has_json=0
      local has_mode=0
      local has_cwd=0
      local has_adapter=0
      local has_timeout=0
      local has_approval=0
      local index=3
      if (( CURRENT > 1 )); then
        prev_word="${words[CURRENT-1]}"
      fi
      while (( index < CURRENT )); do
        case "${words[index]}" in
          -m|--model)
            has_model=1
            (( index += 2 ))
            continue
            ;;
          -p|--prompt)
            has_prompt=1
            (( index += 2 ))
            continue
            ;;
          -r|--request)
            has_request=1
            (( index += 2 ))
            continue
            ;;
          -c|--config)
            has_config=1
            (( index += 2 ))
            continue
            ;;
          -j|--json)
            has_json=1
            (( index += 1 ))
            continue
            ;;
          --mode)
            has_mode=1
            (( index += 2 ))
            continue
            ;;
          --cwd)
            has_cwd=1
            (( index += 2 ))
            continue
            ;;
          --adapter)
            has_adapter=1
            (( index += 2 ))
            continue
            ;;
          --timeout-sec)
            has_timeout=1
            (( index += 2 ))
            continue
            ;;
          --approval-granted)
            has_approval=1
            (( index += 1 ))
            continue
            ;;
        esac
        (( index += 1 ))
      done
      if [[ "${prev_word}" == "-m" || "${prev_word}" == "--model" ]]; then
        _describe 'model' model_choices
        return
      fi
      if [[ "${prev_word}" == "-p" || "${prev_word}" == "--prompt" ]]; then
        _message 'prompt text'
        return
      fi
      if [[ "${prev_word}" == "-r" || "${prev_word}" == "--request" ]]; then
        _files
        return
      fi
      if [[ "${prev_word}" == "-c" || "${prev_word}" == "--config" ]]; then
        _files
        return
      fi
      if [[ "${prev_word}" == "--cwd" ]]; then
        _directories
        return
      fi
      if [[ "${prev_word}" == "--adapter" ]]; then
        _describe 'adapter' '(claude codex copilot gemini)'
        return
      fi
      if [[ "${prev_word}" == "--mode" ]]; then
        _describe 'mode' '(advisory guarded_execution)'
        return
      fi
      remaining_run_flags=()
      if (( has_prompt == 0 )); then
        remaining_run_flags+=('-p:Delegated prompt text' '--prompt:Delegated prompt text')
      fi
      if (( has_model == 0 )); then
        remaining_run_flags+=('-m:Delegated model in adapter/model form' '--model:Delegated model in adapter/model form')
      fi
      if (( has_request == 0 )); then
        remaining_run_flags+=('-r:Path to request JSON or - for stdin' '--request:Path to request JSON or - for stdin')
      fi
      if (( has_config == 0 )); then
        remaining_run_flags+=('-c:Path to delegate config JSON' '--config:Path to delegate config JSON')
      fi
      if (( has_mode == 0 )); then
        remaining_run_flags+=('--mode:Execution mode')
      fi
      if (( has_cwd == 0 )); then
        remaining_run_flags+=('--cwd:Working directory for relative context paths')
      fi
      if (( has_adapter == 0 )); then
        remaining_run_flags+=('--adapter:Delegate adapter')
      fi
      if (( has_timeout == 0 )); then
        remaining_run_flags+=('--timeout-sec:Request timeout in seconds')
      fi
      if (( has_approval == 0 )); then
        remaining_run_flags+=('--approval-granted:Bypass approval block for guarded_execution')
      fi
      if (( has_json == 0 )); then
        remaining_run_flags+=('-j:Emit compact JSON instead of pretty-printed JSON' '--json:Emit compact JSON instead of pretty-printed JSON')
      fi
      if [[ -z "${current_word}" || "${current_word}" == "-" || "${current_word}" == "--" ]]; then
        _describe 'option' remaining_run_flags
        return
      fi
      if [[ "${current_word}" == -* ]]; then
        _describe 'option' remaining_run_flags
        return
      fi
      _arguments -C -s \
        '(-p --prompt)'{-p,--prompt}'[Delegated prompt text]:prompt:' \
        '(-m --model)'{-m,--model}'[Delegated model in adapter/model form]:model:->model' \
        '(-r --request)'{-r,--request}'[Path to request JSON or - for stdin]:request file:_files' \
        '(-c --config)'{-c,--config}'[Path to delegate config JSON]:config file:_files' \
        '--mode[Execution mode]:mode:(advisory guarded_execution)' \
        '--cwd[Working directory for relative context paths]:working directory:_directories' \
        '--adapter[Delegate adapter]:adapter:(claude codex copilot gemini)' \
        '--timeout-sec[Request timeout in seconds]:seconds:' \
        '--approval-granted[Bypass approval block for guarded_execution]' \
        '(-j --json)'{-j,--json}'[Emit compact JSON instead of pretty-printed JSON]' && return
      case "$state" in
        model)
          _describe 'model' model_choices
          return
          ;;
      esac
      ;;
    list-adapters|validate-config)
      _arguments -s \
        '(-c --config)'{-c,--config}'[Path to delegate config JSON]:config file:_files' \
        '(-j --json)'{-j,--json}'[Emit compact JSON instead of pretty-printed JSON]'
      ;;
    completion)
      _arguments '1:shell:(zsh)'
      ;;
    *)
      _describe 'subcommand' subcommands
      ;;
  esac
}
compdef _agent-delegate %s
`, modelChoices, targets)
}

func loadModelChoices() []string {
	cfg, err := delegaterun.LoadConfig("agent-delegate.json")
	if err != nil {
		return nil
	}

	choices := make([]string, 0)
	for _, adapter := range cfg.EnabledAdapters() {
		for _, model := range adapter.Models {
			value := adapter.ID + "/" + model.ID
			description := adapter.ID + " model"
			if model.Multiplier != nil {
				description = fmt.Sprintf("%s premium x%s", adapter.ID, strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", *model.Multiplier), "0"), "."))
			}
			choices = append(choices, fmt.Sprintf("'%s:%s'", escapeZsh(value), escapeZsh(description)))
		}
	}
	sort.Strings(choices)
	return choices
}

func escapeZsh(value string) string {
	return strings.ReplaceAll(value, "'", "'\\''")
}

func completionTargets() []string {
	targets := []string{
		"agent-delegate",
		"bin/agent-delegate",
		"./bin/agent-delegate",
		"/Users/oli/projects/active/agent-toolkit/bin/agent-delegate",
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		targets = append(targets, filepath.Join(home, ".local", "bin", "agent-delegate"))
	}
	return targets
}
