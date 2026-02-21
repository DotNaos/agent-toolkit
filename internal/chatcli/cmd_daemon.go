package chatcli

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"agent-toolkit/internal/chatd"
	"github.com/spf13/cobra"
)

var daemonListen string

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Run the local agent-chat daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveConfig(serverURLFlag, dbPathFlag)
		if err != nil {
			return err
		}

		listenAddr := strings.TrimSpace(daemonListen)
		if listenAddr == "" {
			listenAddr, err = parseListenAddr(cfg.ServerURL)
			if err != nil {
				return err
			}
		}

		store, err := chatd.NewSQLiteStore(cfg.DBPath, chatd.DefaultLeaseDuration)
		if err != nil {
			return err
		}

		server, err := chatd.NewServer(chatd.ServerConfig{
			ListenAddr:   listenAddr,
			DBPath:       cfg.DBPath,
			PollInterval: chatd.DefaultPollInterval,
			Store:        store,
		})
		if err != nil {
			_ = store.Close()
			return err
		}
		defer server.Close()

		if err := outputJSON(map[string]any{
			"status": "success",
			"action": "daemon",
			"listen": listenAddr,
			"db":     cfg.DBPath,
		}); err != nil {
			return err
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		go func() {
			<-sigCh
			_ = server.Close()
		}()

		if err := server.Run(); err != nil {
			return fmt.Errorf("daemon server failed: %w", err)
		}
		return nil
	},
}

func init() {
	daemonCmd.Flags().StringVar(&daemonListen, "listen", chatd.DefaultListenAddr, "Daemon listen address")
}
