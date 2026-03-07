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

type ModelConfig struct {
	ID         string   `json:"id"`
	Label      string   `json:"label,omitempty"`
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
}

type Config struct {
	Defaults DefaultsConfig           `json:"defaults"`
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
		for _, model := range adapter.Models {
			if strings.TrimSpace(model.ID) == "" {
				return fmt.Errorf("adapter %q contains model with empty id", id)
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
		})
	}
	return items
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
