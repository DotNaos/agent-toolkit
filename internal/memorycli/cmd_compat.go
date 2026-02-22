package memorycli

import (
	"agent-toolkit/internal/memoryd"
	"github.com/spf13/cobra"
)

var compatCmd = &cobra.Command{Use: "compat", Short: "Compatibility and proxy interception status"}

var compatStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show intercepted provider compatibility report",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveConfig(flagDBPath, flagListen, flagOllamaURL, flagEmbeddingModel, flagMemoryReposRoot)
		if err != nil {
			return err
		}
		store, err := openStore(cfg)
		if err != nil {
			return err
		}
		defer store.Close()
		items, err := store.CompatReport(memoryLimit)
		if err != nil {
			return err
		}
		return outputJSON(map[string]any{"status": "success", "action": "compat.status", "items": items})
	},
}

func init() {
	compatStatusCmd.Flags().IntVar(&memoryLimit, "limit", 100, "Rows to return")
	compatCmd.AddCommand(compatStatusCmd)
	_ = memoryd.DefaultListenAddr
}
