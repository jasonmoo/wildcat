package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wildcat",
	Short: "Static analysis CLI for AI agents",
	Long: `Wildcat is a static analysis CLI optimized for AI agents.

Uses gopls (Go Language Server) to provide symbol-based queries with
structured JSON output. Built on LSP for future multi-language support.

Output is designed for AI tool integration with consistent JSON structure,
absolute paths, and actionable error messages.`,
}

var (
	globalOutput string
	globalDebug  bool
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file path")
	rootCmd.PersistentFlags().StringVarP(&globalOutput, "output", "o", "json", "Output format (json, yaml, markdown, template:<path>, plugin:<name>)")
	rootCmd.PersistentFlags().BoolVar(&globalDebug, "debug", false, "Enable LSP debug logging (dumps on 0 results)")
}
