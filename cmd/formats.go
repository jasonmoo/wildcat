package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

var formatsCmd = &cobra.Command{
	Use:   "formats",
	Short: "List available output formats",
	Long: `List all available output formats.

Built-in formats:
  json       JSON output (default)
  yaml       YAML output
  dot        Graphviz DOT format (for call trees)
  markdown   Markdown tables and lists

Custom formats:
  template:<path>   Use a Go template file
  plugin:<name>     Use external plugin (wildcat-format-<name>)

Examples:
  wildcat formats
  wildcat callers main.main --output yaml
  wildcat tree main.main --output dot | dot -Tpng -o graph.png
  wildcat callers main.main --output template:./my.tmpl`,
	Run: runFormats,
}

func init() {
	rootCmd.AddCommand(formatsCmd)
}

func runFormats(cmd *cobra.Command, args []string) {
	formatters := output.DefaultRegistry.All()

	// Sort by name
	sort.Slice(formatters, func(i, j int) bool {
		return formatters[i].Name() < formatters[j].Name()
	})

	fmt.Fprintln(os.Stdout, "Available output formats:")
	fmt.Fprintln(os.Stdout)

	for _, f := range formatters {
		fmt.Fprintf(os.Stdout, "  %-12s  %s\n", f.Name(), f.Description())
	}

	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Custom formats:")
	fmt.Fprintln(os.Stdout, "  template:<path>  Use a Go template file")
	fmt.Fprintln(os.Stdout, "  plugin:<name>    External plugin (wildcat-format-<name>)")
}
