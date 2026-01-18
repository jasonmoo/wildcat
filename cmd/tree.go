package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jasonmoo/wildcat/internal/errors"
	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/symbols"
	"github.com/jasonmoo/wildcat/internal/traverse"
	"github.com/spf13/cobra"
)

var treeCmd = &cobra.Command{
	Use:   "tree <symbol>",
	Short: "Build a call tree centered on a symbol",
	Long: `Build a call tree showing callers and callees of a symbol.

The symbol is the center point of the tree:
  --up N    Show N levels of callers (what calls this function)
  --down N  Show N levels of callees (what this function calls)

By default, shows 2 levels in each direction.

Scope:
  all     - Include everything (stdlib, dependencies)
  project - Project packages only (default)
  package - Same package as starting symbol only

Examples:
  wildcat tree main.main                              # 2 up, 2 down (default)
  wildcat tree db.Query --up 3 --down 1               # focus on callers
  wildcat tree Server.Start --up 0 --down 4           # callees only
  wildcat tree Handler.ServeHTTP --scope all          # include stdlib calls`,
	Args: cobra.ExactArgs(1),
	RunE: runTree,
}

var (
	treeUp           int
	treeDown         int
	treeExcludeTests bool
	treeScope        string
)

func init() {
	rootCmd.AddCommand(treeCmd)

	treeCmd.Flags().IntVar(&treeUp, "up", 2, "Depth of callers to show (0 to skip)")
	treeCmd.Flags().IntVar(&treeDown, "down", 2, "Depth of callees to show (0 to skip)")
	treeCmd.Flags().BoolVar(&treeExcludeTests, "exclude-tests", false, "Exclude test files")
	treeCmd.Flags().StringVar(&treeScope, "scope", "project", "Traversal scope: all, project, package")
}

func runTree(cmd *cobra.Command, args []string) error {
	symbolArg := args[0]
	writer, err := GetWriter(os.Stdout)
	if err != nil {
		return fmt.Errorf("invalid output format: %w", err)
	}

	// Parse symbol
	query, err := symbols.Parse(symbolArg)
	if err != nil {
		return writer.WriteError("parse_error", err.Error(), nil, nil)
	}

	// Validate depths
	if treeUp < 0 || treeDown < 0 {
		return writer.WriteError("invalid_argument", "--up and --down must be non-negative", nil, nil)
	}

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Start LSP client
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	config, err := GetServerConfig(workDir)
	if err != nil {
		return writer.WriteError(
			string(errors.CodeServerNotFound),
			err.Error(),
			nil,
			nil,
		)
	}

	client, err := lsp.NewClient(ctx, config)
	if err != nil {
		return writer.WriteError(
			string(errors.CodeServerNotFound),
			fmt.Sprintf("Failed to start language server: %v", err),
			nil,
			map[string]any{"server": config.Command},
		)
	}
	defer client.Close()

	if err := client.Initialize(ctx); err != nil {
		return writer.WriteError(
			string(errors.CodeLSPError),
			fmt.Sprintf("LSP initialization failed: %v", err),
			nil,
			nil,
		)
	}
	defer client.Shutdown(ctx)

	if err := client.WaitForReady(ctx); err != nil {
		return writer.WriteError(
			string(errors.CodeLSPError),
			fmt.Sprintf("LSP server not ready: %v", err),
			nil,
			nil,
		)
	}

	// Resolve symbol
	resolver := symbols.NewDefaultResolver(client)
	resolved, err := resolver.Resolve(ctx, query)
	if err != nil {
		if we, ok := err.(*errors.WildcatError); ok {
			return writer.WriteError(string(we.Code), we.Message, we.Suggestions, we.Context)
		}
		return writer.WriteError(string(errors.CodeSymbolNotFound), err.Error(), nil, nil)
	}

	// Prepare call hierarchy
	items, err := client.PrepareCallHierarchy(ctx, resolved.URI, resolved.Position)
	if err != nil {
		return writer.WriteError(
			string(errors.CodeLSPError),
			fmt.Sprintf("Failed to prepare call hierarchy: %v", err),
			nil,
			nil,
		)
	}

	if len(items) == 0 {
		return writer.WriteError(
			string(errors.CodeSymbolNotFound),
			fmt.Sprintf("No call hierarchy found for '%s'", query.Raw),
			nil,
			nil,
		)
	}

	// Validate and convert scope
	var scope traverse.Scope
	switch treeScope {
	case "all":
		scope = traverse.ScopeAll
	case "project":
		scope = traverse.ScopeProject
	case "package":
		scope = traverse.ScopePackage
	default:
		return writer.WriteError("invalid_argument", "scope must be 'all', 'project', or 'package'", nil, nil)
	}

	// Build tree
	traverser := traverse.NewTraverser(client)
	opts := traverse.Options{
		UpDepth:      treeUp,
		DownDepth:    treeDown,
		ExcludeTests: treeExcludeTests,
		Scope:        scope,
		StartFile:    lsp.URIToPath(resolved.URI),
	}

	tree, err := traverser.BuildTree(ctx, items[0], opts)
	if err != nil {
		return writer.WriteError(
			string(errors.CodeLSPError),
			fmt.Sprintf("Failed to build call tree: %v", err),
			nil,
			nil,
		)
	}

	return writer.Write(tree)
}
