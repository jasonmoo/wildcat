package cmd

import (
	channels_cmd "github.com/jasonmoo/wildcat/internal/commands/channels"
	deadcode_cmd "github.com/jasonmoo/wildcat/internal/commands/deadcode"
	package_cmd "github.com/jasonmoo/wildcat/internal/commands/package"
	search_cmd "github.com/jasonmoo/wildcat/internal/commands/search"
	symbol_cmd "github.com/jasonmoo/wildcat/internal/commands/symbol"
	tree_cmd "github.com/jasonmoo/wildcat/internal/commands/tree"
	"github.com/spf13/cobra"
)

// globalOutput is used by GetWriter in server.go
var globalOutput string

func Execute() error {

	rootCmd := &cobra.Command{
		Use:   "wildcat",
		Short: "Go static analysis CLI for AI agents",
		Long: `Wildcat is a Go static analysis CLI optimized for AI agents.

Uses gopls to provide symbol-based code queries with structured JSON output.
Designed for AI tool integration with consistent structure, absolute paths,
and actionable error messages.`,
	}

	// Configure root command
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.PersistentFlags().StringVarP(&globalOutput, "output", "o", "markdown", "Output format (json, yaml, markdown, template:<path>, plugin:<name>)")

	// Custom usage template with ordered commands
	commandOrder := []string{
		"package",  // package-level analysis
		"symbol",   // symbol-level analysis
		"search",   // find symbols
		"tree",     // call graph traversal
		"channels", // concurrency analysis
		"deadcode", // dead code detection
		"readme",   // onboarding
		"version",  // meta
		"help",     // always last
	}

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

	// Add subcommands
	rootCmd.AddCommand(package_cmd.NewPackageCommand().Cmd())
	rootCmd.AddCommand(symbol_cmd.NewSymbolCommand().Cmd())
	rootCmd.AddCommand(search_cmd.NewSearchCommand().Cmd())
	rootCmd.AddCommand(tree_cmd.NewTreeCommand().Cmd())
	rootCmd.AddCommand(channels_cmd.NewChannelsCommand().Cmd())
	rootCmd.AddCommand(deadcode_cmd.NewDeadcodeCommand().Cmd())
	rootCmd.AddCommand(readmeCmd)
	rootCmd.AddCommand(versionCmd)

	return rootCmd.Execute()
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
