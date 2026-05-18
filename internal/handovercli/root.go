package handovercli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           commandName(),
	Short:         "Create and kick off Codex handover briefings",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(kickoffCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(resumeCmd)
}

func commandName() string {
	if len(os.Args) > 0 {
		name := filepath.Base(os.Args[0])
		if name != "" {
			return name
		}
	}
	return "codex-handover"
}
