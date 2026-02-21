package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           commandName(),
	Short:         "Toolkit for mapping and interacting with macOS UI elements",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(mapUICmd)
	rootCmd.AddCommand(clickCmd)
	rootCmd.AddCommand(typeCmd)
}

func commandName() string {
	if len(os.Args) > 0 {
		name := filepath.Base(os.Args[0])
		if name != "" {
			return name
		}
	}
	return "tool"
}
