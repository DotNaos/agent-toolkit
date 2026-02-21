package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"agent-toolkit/internal/hubapi"
	"agent-toolkit/internal/shared/cliio"
	"github.com/spf13/cobra"
)

func main() {
	var listen string
	var dbPath string
	var webDir string

	cmd := &cobra.Command{
		Use:           "agent-hub",
		Short:         "Web/PWA group chat hub with human-in-the-loop approvals",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedDB, err := expandPath(dbPath)
			if err != nil {
				return err
			}

			resolvedWebDir, err := resolveWebDir(webDir)
			if err != nil {
				return err
			}

			server, err := hubapi.NewServer(hubapi.Config{
				ListenAddr: listen,
				DBPath:     resolvedDB,
				WebDir:     resolvedWebDir,
			})
			if err != nil {
				return err
			}

			if err := cliio.OutputJSON(map[string]any{
				"status":  "success",
				"action":  "agent-hub",
				"listen":  listen,
				"db":      resolvedDB,
				"web_dir": resolvedWebDir,
			}); err != nil {
				return err
			}

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			defer signal.Stop(sigCh)

			errCh := make(chan error, 1)
			go func() {
				errCh <- server.Run()
			}()

			select {
			case sig := <-sigCh:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := server.Close(ctx); err != nil {
					return fmt.Errorf("shutdown after signal %s failed: %w", sig, err)
				}
				return nil
			case err := <-errCh:
				if err != nil {
					return err
				}
				return nil
			}
		},
	}

	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:46001", "Hub listen address")
	cmd.Flags().StringVar(&dbPath, "db", "~/.agent-toolkit/hub.db", "Path to hub sqlite database")
	cmd.Flags().StringVar(&webDir, "web-dir", "web/agent-hub/dist", "Path to web UI assets")

	if err := cmd.Execute(); err != nil {
		fmt.Println(cliio.FormatErrorJSON(err))
		os.Exit(1)
	}
}

func expandPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	if path == "~" {
		return os.UserHomeDir()
	}
	if len(path) > 2 && path[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func resolveWebDir(path string) (string, error) {
	expanded, err := expandPath(path)
	if err != nil {
		return "", err
	}
	resolved := expanded
	if !filepath.IsAbs(expanded) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		resolved = filepath.Join(cwd, expanded)
	}

	indexPath := filepath.Join(resolved, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		return "", fmt.Errorf("web assets not found at %s (missing index.html). Build UI first with: cd web/agent-hub && bun install && bun run build", resolved)
	}

	return resolved, nil
}
