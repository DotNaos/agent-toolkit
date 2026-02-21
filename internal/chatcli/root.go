package chatcli

import "github.com/spf13/cobra"

var (
	serverURLFlag string
	dbPathFlag    string
)

var rootCmd = &cobra.Command{
	Use:           "agent-chat",
	Short:         "Agent-to-agent chat CLI with local realtime daemon",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&serverURLFlag, "server-url", "", "Daemon base URL (default http://127.0.0.1:45217)")
	rootCmd.PersistentFlags().StringVar(&dbPathFlag, "db", "", "SQLite path for daemon/autostart (default ~/.agent-toolkit/chat.db)")

	rootCmd.AddCommand(sendCmd)
	rootCmd.AddCommand(waitCmd)
	rootCmd.AddCommand(watchCmd)
	rootCmd.AddCommand(ackCmd)
	rootCmd.AddCommand(daemonCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
