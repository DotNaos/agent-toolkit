package delegaterun

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type DefaultsConfig struct {
	TimeoutSec    int `json:"timeout_sec"`
	MaxTimeoutSec int `json:"max_timeout_sec"`
}

type PolicyConfig struct {
	DefaultCapabilities    []string `json:"default_capabilities"`
	ApprovalRequiredFor    []string `json:"approval_required_for"`
	AllowHeuristicFallback bool     `json:"allow_heuristic_fallback"`
}

type ModelConfig struct {
	ID         string   `json:"id"`
	Label      string   `json:"label,omitempty"`
	Aliases    []string `json:"aliases,omitempty"`
	Multiplier *float64 `json:"multiplier,omitempty"`
}

type AdapterConfig struct {
	Enabled                  bool          `json:"enabled"`
	Command                  string        `json:"command"`
	Args                     []string      `json:"args"`
	TimeoutSec               int           `json:"timeout_sec"`
	SupportsGuardedExecution bool          `json:"supports_guarded_execution"`
	DefaultModel             string        `json:"default_model,omitempty"`
	Models                   []ModelConfig `json:"models,omitempty"`
	SupportedCapabilities    []string      `json:"supported_capabilities,omitempty"`
}

type Config struct {
	Defaults DefaultsConfig           `json:"defaults"`
	Policy   PolicyConfig             `json:"policy"`
	Adapters map[string]AdapterConfig `json:"adapters"`
}

type AdapterInfo struct {
	ID                       string        `json:"id"`
	Command                  string        `json:"command"`
	Args                     []string      `json:"args,omitempty"`
	TimeoutSec               int           `json:"timeout_sec"`
	SupportsGuardedExecution bool          `json:"supports_guarded_execution"`
	DefaultModel             string        `json:"default_model,omitempty"`
	Models                   []ModelConfig `json:"models,omitempty"`
	SupportedCapabilities    []string      `json:"supported_capabilities,omitempty"`
}

func LoadConfig(path string) (Config, error) {
	resolved := strings.TrimSpace(path)
	if resolved == "" {
		resolved = "agent-delegate.json"
	}
	if !filepath.IsAbs(resolved) {
		cwd, err := os.Getwd()
		if err != nil {
			return Config{}, err
		}
		resolved = filepath.Join(cwd, resolved)
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Defaults.TimeoutSec <= 0 {
		c.Defaults.TimeoutSec = 120
	}
	if c.Defaults.MaxTimeoutSec <= 0 {
		c.Defaults.MaxTimeoutSec = 600
	}
	if c.Adapters == nil {
		c.Adapters = map[string]AdapterConfig{}
	}
	if len(c.Policy.DefaultCapabilities) == 0 {
		c.Policy.DefaultCapabilities = []string{"read"}
	}
	if len(c.Policy.ApprovalRequiredFor) == 0 {
		c.Policy.ApprovalRequiredFor = []string{"write", "exec", "network", "git"}
	}
	if !c.Policy.AllowHeuristicFallback {
		// false is a meaningful value only when explicitly set later during decode.
		// Default to true here for backwards compatible config behavior.
		c.Policy.AllowHeuristicFallback = true
	}
	for id, adapter := range c.Adapters {
		if len(adapter.SupportedCapabilities) == 0 {
			adapter.SupportedCapabilities = []string{"read", "write", "exec", "network", "git"}
		} else {
			adapter.SupportedCapabilities = normalizeCapabilities(adapter.SupportedCapabilities)
		}
		c.Adapters[id] = adapter
	}
}

func (c Config) Validate() error {
	c.applyDefaults()
	if len(c.Adapters) == 0 {
		return fmt.Errorf("config must define at least one adapter")
	}
	for id, adapter := range c.Adapters {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("adapter id cannot be empty")
		}
		if !adapter.Enabled {
			continue
		}
		if strings.TrimSpace(adapter.Command) == "" {
			return fmt.Errorf("adapter %q missing command", id)
		}
		if adapter.TimeoutSec < 0 {
			return fmt.Errorf("adapter %q has invalid timeout", id)
		}
		if len(adapter.SupportedCapabilities) == 0 {
			adapter.SupportedCapabilities = []string{"read", "write", "exec", "network", "git"}
		}
		for _, capability := range normalizeStringList(adapter.SupportedCapabilities) {
			if _, ok := allowedCapabilities[capability]; !ok {
				return fmt.Errorf("adapter %q has unsupported capability %q", id, capability)
			}
		}
		for _, model := range adapter.Models {
			if strings.TrimSpace(model.ID) == "" {
				return fmt.Errorf("adapter %q contains model with empty id", id)
			}
			for _, alias := range normalizeStringList(model.Aliases) {
				if alias == strings.TrimSpace(strings.ToLower(model.ID)) {
					return fmt.Errorf("adapter %q model %q alias duplicates id", id, model.ID)
				}
			}
		}
	}
	return nil
}

func (c Config) EnabledAdapters() []AdapterInfo {
	ids := make([]string, 0, len(c.Adapters))
	for id, adapter := range c.Adapters {
		if adapter.Enabled {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	items := make([]AdapterInfo, 0, len(ids))
	for _, id := range ids {
		adapter := c.Adapters[id]
		items = append(items, AdapterInfo{
			ID:                       id,
			Command:                  resolvedCommand(id, adapter),
			Args:                     append([]string(nil), adapter.Args...),
			TimeoutSec:               adapter.TimeoutSec,
			SupportsGuardedExecution: adapter.SupportsGuardedExecution,
			DefaultModel:             adapter.DefaultModel,
			Models:                   append([]ModelConfig(nil), adapter.Models...),
			SupportedCapabilities:    append([]string(nil), normalizeCapabilities(adapter.SupportedCapabilities)...),
		})
	}
	return items
}

func normalizeCapabilities(items []string) []string {
	normalized := normalizeStringList(items)
	if len(normalized) == 0 {
		return []string{"read", "write", "exec", "network", "git"}
	}
	return normalized
}

func resolvedCommand(adapterID string, adapter AdapterConfig) string {
	envName := "AGENT_DELEGATE_" + strings.ToUpper(strings.ReplaceAll(adapterID, "-", "_")) + "_COMMAND"
	if value := strings.TrimSpace(os.Getenv(envName)); value != "" {
		return value
	}
	return strings.TrimSpace(adapter.Command)
}

func resolveCommandPath(adapterID string, adapter AdapterConfig) (string, error) {
	command := resolvedCommand(adapterID, adapter)
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("adapter %q command is empty", adapterID)
	}
	if strings.ContainsRune(command, os.PathSeparator) {
		if _, err := os.Stat(command); err != nil {
			return "", fmt.Errorf("adapter %q command not found: %w", adapterID, err)
		}
		return command, nil
	}
	resolved, err := exec.LookPath(command)
	if err != nil {
		return "", fmt.Errorf("adapter %q command %q not found in PATH", adapterID, command)
	}
	return resolved, nil
}
