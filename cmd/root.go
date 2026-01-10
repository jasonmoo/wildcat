package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wildcat",
	Short: "Language-agnostic static analysis CLI for AI agents",
	Long: `Wildcat is a language-agnostic static analysis CLI optimized for AI agents.

Uses LSP (Language Server Protocol) to provide symbol-based queries with
structured JSON output. Supports multiple languages including Go, Python,
TypeScript, Rust, and C/C++.

Output is designed for AI tool integration with consistent JSON structure,
absolute paths, and actionable error messages.`,
}

var (
	globalOutput string
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringP("config", "c", "", "config file path")
	rootCmd.PersistentFlags().StringVarP(&globalOutput, "output", "o", "json", "Output format (json, yaml, dot, markdown, template:<path>, plugin:<name>)")
}
