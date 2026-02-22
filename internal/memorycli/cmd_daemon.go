package memorycli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"agent-toolkit/internal/memoryd"
	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the local agent-memory daemon/API",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveConfig(flagDBPath, flagListen, flagOllamaURL, flagEmbeddingModel, flagMemoryReposRoot)
		if err != nil {
			return err
		}
		srv, err := memoryd.NewServer(memoryd.ServerConfig{
			ListenAddr:      cfg.ListenAddr,
			DBPath:          cfg.DBPath,
			OllamaURL:       cfg.OllamaURL,
			EmbeddingModel:  cfg.EmbeddingModel,
			MemoryReposRoot: cfg.MemoryReposRoot,
		})
		if err != nil {
			return err
		}
		defer srv.Close(context.Background())

		if err := outputJSON(map[string]any{"status": "success", "action": "daemon", "listen": cfg.ListenAddr, "db": cfg.DBPath, "ollama_url": cfg.OllamaURL, "embedding_model": cfg.EmbeddingModel}); err != nil {
			return err
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)
		errCh := make(chan error, 1)
		go func() { errCh <- srv.Run() }()
		select {
		case sig := <-sigCh:
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := srv.Close(ctx); err != nil {
				return fmt.Errorf("shutdown after %s failed: %w", sig, err)
			}
			return nil
		case err := <-errCh:
			return err
		}
	},
}
