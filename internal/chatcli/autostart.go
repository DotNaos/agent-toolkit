package chatcli

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"agent-toolkit/internal/chatd"
)

func ensureDaemon(cfg ClientConfig) error {
	if err := getHealth(cfg.ServerURL); err == nil {
		return nil
	}

	if !isLocalServerURL(cfg.ServerURL) {
		return fmt.Errorf("daemon at %s is unreachable", cfg.ServerURL)
	}

	listenAddr, err := parseListenAddr(cfg.ServerURL)
	if err != nil {
		return err
	}

	if err := startDaemonProcess(listenAddr, cfg.DBPath); err != nil {
		return err
	}

	deadline := time.Now().Add(chatd.DefaultAutoStartWait)
	for time.Now().Before(deadline) {
		if err := getHealth(cfg.ServerURL); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("failed to start daemon within %s", chatd.DefaultAutoStartWait)
}

func startDaemonProcess(listenAddr, dbPath string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve executable path: %w", err)
	}

	if err := os.MkdirAll(defaultToolkitDir(), 0o755); err != nil {
		return fmt.Errorf("failed to create toolkit dir: %w", err)
	}

	logPath := filepath.Join(defaultToolkitDir(), "agent-chat-daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open daemon log file: %w", err)
	}

	cmd := exec.Command(exePath, "daemon", "--listen", listenAddr, "--db", dbPath)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("failed to start daemon process: %w", err)
	}

	return logFile.Close()
}

func isLocalServerURL(serverURL string) bool {
	u, err := url.Parse(serverURL)
	if err != nil {
		return false
	}

	host := u.Host
	if strings.Contains(host, ":") {
		if h, _, splitErr := net.SplitHostPort(host); splitErr == nil {
			host = h
		}
	}

	host = strings.Trim(host, "[]")
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}
