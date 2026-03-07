package delegaterun

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Runner struct {
	ConfigPath string
}

func New(configPath string) *Runner {
	return &Runner{ConfigPath: configPath}
}

func (r *Runner) LoadConfig() (Config, error) {
	return LoadConfig(r.ConfigPath)
}

func (r *Runner) ListEnabledAdapters() ([]AdapterInfo, error) {
	cfg, err := r.LoadConfig()
	if err != nil {
		return nil, err
	}
	return cfg.EnabledAdapters(), nil
}

func (r *Runner) Run(ctx context.Context, req Request, opts RunOptions) (Result, error) {
	req.Normalize()
	if err := req.Validate(); err != nil {
		return Result{}, err
	}
	toolDir, err := findToolDir()
	if err != nil {
		return Result{}, err
	}

	payload := map[string]any{
		"request":          req,
		"approval_granted": opts.ApprovalGranted,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Result{}, err
	}

	configPath, err := resolveConfigPath(r.ConfigPath)
	if err != nil {
		return Result{}, err
	}

	args := []string{"run", "src/index.ts", "run", "--request", "-", "--json", "--config", configPath}
	cmd := exec.CommandContext(ctx, "bun", args...)
	cmd.Dir = toolDir
	cmd.Stdin = strings.NewReader(string(body))
	if callerDir, cwdErr := os.Getwd(); cwdErr == nil {
		cmd.Env = append(os.Environ(), "AGENT_DELEGATE_CALLER_CWD="+callerDir)
	}

	output, err := cmd.CombinedOutput()
	if err != nil && len(output) == 0 {
		return Result{}, fmt.Errorf("agent-delegate TS CLI failed: %w", err)
	}

	var result Result
	if decodeErr := json.Unmarshal(output, &result); decodeErr != nil {
		return Result{}, fmt.Errorf("invalid delegate response: %w", decodeErr)
	}
	return result, nil
}

func ParseTimeout(raw any) int {
	switch value := raw.(type) {
	case float64:
		return int(value)
	case int:
		return value
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(value))
		return n
	default:
		return 0
	}
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

func resolveConfigPath(rawPath string) (string, error) {
	if strings.TrimSpace(rawPath) == "" {
		rawPath = "agent-delegate.json"
	}
	if filepath.IsAbs(rawPath) {
		return rawPath, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, rawPath), nil
}
