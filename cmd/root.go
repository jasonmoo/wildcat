package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wildcat",
	Short: "Go static analysis CLI for AI agents",
	Long: `Wildcat is a Go static analysis CLI optimized for AI agents.

Uses gopls to provide symbol-based code queries with structured JSON output.
Designed for AI tool integration with consistent structure, absolute paths,
and actionable error messages.`,
}

var globalOutput string

func Execute() error {
	return rootCmd.Execute()
}

// commandOrder defines the display order for commands in help output.
// Commands not in this list appear at the end alphabetically.
var commandOrder = []string{
	"package",  // package-level analysis
	"symbol",   // symbol-level analysis
	"search",   // find symbols
	"tree",     // call graph traversal
	"channels", // concurrency analysis
	"readme",   // onboarding
	"version",  // meta
	"help",     // always last
}

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.PersistentFlags().StringVarP(&globalOutput, "output", "o", "json", "Output format (json, yaml, markdown, template:<path>, plugin:<name>)")

	// Custom usage template with ordered commands
	cobra.AddTemplateFunc("orderedCommands", func(cmd *cobra.Command) []*cobra.Command {
		commands := cmd.Commands()
		orderMap := make(map[string]int)
		for i, name := range commandOrder {
			orderMap[name] = i
		}

		// Sort by our order, unknowns go to end
		result := make([]*cobra.Command, 0, len(commands))
		for _, c := range commands {
			if c.IsAvailableCommand() || c.Name() == "help" {
				result = append(result, c)
			}
		}

		// Stable sort preserving our order
		for i := 0; i < len(result)-1; i++ {
			for j := i + 1; j < len(result); j++ {
				oi, oki := orderMap[result[i].Name()]
				oj, okj := orderMap[result[j].Name()]
				if !oki {
					oi = 1000 // unknown commands go to end
				}
				if !okj {
					oj = 1000
				}
				if oi > oj {
					result[i], result[j] = result[j], result[i]
				}
			}
		}
		return result
	})

	rootCmd.SetUsageTemplate(usageTemplate)
}

var usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range orderedCommands .}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
