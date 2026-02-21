package chatcli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-toolkit/internal/chatd"
	"agent-toolkit/internal/shared/cliio"
)

const (
	envServerURL = "AGENT_CHAT_SERVER_URL"
	envDBPath    = "AGENT_CHAT_DB_PATH"
)

var httpClient = &http.Client{Timeout: 70 * time.Second}

type ClientConfig struct {
	ServerURL string
	DBPath    string
}

func defaultToolkitDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".agent-toolkit"
	}
	return filepath.Join(home, ".agent-toolkit")
}

func defaultDBPath() string {
	return filepath.Join(defaultToolkitDir(), "chat.db")
}

func defaultServerURL() string {
	return "http://" + chatd.DefaultListenAddr
}

func resolveConfig(serverURLFlag, dbPathFlag string) (ClientConfig, error) {
	serverURL := strings.TrimSpace(serverURLFlag)
	if serverURL == "" {
		serverURL = strings.TrimSpace(os.Getenv(envServerURL))
	}
	if serverURL == "" {
		serverURL = defaultServerURL()
	}

	dbPath := strings.TrimSpace(dbPathFlag)
	if dbPath == "" {
		dbPath = strings.TrimSpace(os.Getenv(envDBPath))
	}
	if dbPath == "" {
		dbPath = defaultDBPath()
	}

	expandedDBPath, err := expandPath(dbPath)
	if err != nil {
		return ClientConfig{}, err
	}

	return ClientConfig{
		ServerURL: strings.TrimRight(serverURL, "/"),
		DBPath:    expandedDBPath,
	}, nil
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

func parseListenAddr(serverURL string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", fmt.Errorf("invalid server url: %w", err)
	}
	if u.Scheme != "http" {
		return "", fmt.Errorf("server url must use http")
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", fmt.Errorf("server url missing host")
	}
	return u.Host, nil
}

func outputJSON(payload any) error {
	return cliio.OutputJSON(payload)
}

func FormatErrorJSON(err error) string {
	return cliio.FormatErrorJSON(err)
}

func postJSON(serverURL, endpoint string, requestPayload any, responsePayload any) (int, error) {
	body, err := json.Marshal(requestPayload)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, serverURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(responsePayload); err != nil {
		return resp.StatusCode, fmt.Errorf("failed to decode response: %w", err)
	}

	return resp.StatusCode, nil
}

func getHealth(serverURL string) error {
	req, err := http.NewRequest(http.MethodGet, serverURL+"/v1/health", nil)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}
