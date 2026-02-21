package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-toolkit/internal/shared/cliio"
)

type Bounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type MappedElement struct {
	ID          string  `json:"id"`
	Path        string  `json:"path"`
	Role        string  `json:"role,omitempty"`
	Subrole     string  `json:"subrole,omitempty"`
	Title       string  `json:"title,omitempty"`
	Value       string  `json:"value,omitempty"`
	Description string  `json:"description,omitempty"`
	Identifier  string  `json:"identifier,omitempty"`
	Bounds      *Bounds `json:"bounds,omitempty"`
}

type ViewState struct {
	Status      string          `json:"status"`
	Action      string          `json:"action"`
	AppName     string          `json:"app_name,omitempty"`
	PID         int             `json:"pid,omitempty"`
	GeneratedAt string          `json:"generated_at"`
	Screenshot  string          `json:"screenshot"`
	StateFile   string          `json:"state_file"`
	Elements    []MappedElement `json:"elements"`
}

type axDump struct {
	Status      string  `json:"status"`
	AppName     string  `json:"appName"`
	PID         int     `json:"pid"`
	GeneratedAt string  `json:"generatedAt"`
	Root        *axNode `json:"root"`
	Message     string  `json:"message,omitempty"`
}

type axNode struct {
	Role        string    `json:"role,omitempty"`
	Subrole     string    `json:"subrole,omitempty"`
	Title       string    `json:"title,omitempty"`
	Value       string    `json:"value,omitempty"`
	Description string    `json:"description,omitempty"`
	Identifier  string    `json:"identifier,omitempty"`
	Bounds      *Bounds   `json:"bounds,omitempty"`
	Children    []*axNode `json:"children,omitempty"`
}

func FormatErrorJSON(err error) string {
	return cliio.FormatErrorJSON(err)
}

func outputJSON(v any) error {
	return cliio.OutputJSON(v)
}

func toolkitDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agent-toolkit"
	}
	return filepath.Join(home, ".agent-toolkit")
}

func defaultStatePath() string {
	return filepath.Join(toolkitDir(), "current_view.json")
}

func defaultScreenshotPath() string {
	return filepath.Join(toolkitDir(), "current_view.png")
}

func defaultAXDumpBinaryPath() string {
	return filepath.Join(toolkitDir(), "bin", "axdump")
}

func ensureParentDir(path string) error {
	parent := filepath.Dir(path)
	if parent == "." || parent == "" {
		return nil
	}
	return os.MkdirAll(parent, 0o755)
}

func expandPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path cannot be empty")
	}
	if path == "~" {
		return os.UserHomeDir()
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to resolve home directory: %w", err)
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return path, nil
}

func saveState(path string, state *ViewState) error {
	if err := ensureParentDir(path); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}
	return nil
}

func loadState(path string) (*ViewState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file %s: %w", path, err)
	}

	var state ViewState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to parse state file %s: %w", path, err)
	}
	return &state, nil
}

func flattenAXTree(root *axNode) []MappedElement {
	if root == nil {
		return nil
	}

	elements := make([]MappedElement, 0, 256)
	counter := 1

	var walk func(node *axNode, path string)
	walk = func(node *axNode, path string) {
		if node == nil {
			return
		}

		id := fmt.Sprintf("X%03d", counter)
		counter++

		elements = append(elements, MappedElement{
			ID:          id,
			Path:        path,
			Role:        node.Role,
			Subrole:     node.Subrole,
			Title:       node.Title,
			Value:       node.Value,
			Description: node.Description,
			Identifier:  node.Identifier,
			Bounds:      node.Bounds,
		})

		for i, child := range node.Children {
			walk(child, fmt.Sprintf("%s.%d", path, i))
		}
	}

	walk(root, "0")
	return elements
}

func findElementByID(elements []MappedElement, id string) (*MappedElement, bool) {
	for i := range elements {
		if strings.EqualFold(elements[i].ID, id) {
			return &elements[i], true
		}
	}
	return nil, false
}
