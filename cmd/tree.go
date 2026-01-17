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
	Short: "Build a call tree from a starting point",
	Long: `Build a call tree from a starting point.

Direction:
  up    - Show callers (what calls this function)
  down  - Show callees (what this function calls)

Scope:
  all     - Include everything (stdlib, dependencies)
  project - Project packages only (default)
  package - Same package as starting symbol only

Examples:
  wildcat tree main.main --depth 3 --direction down
  wildcat tree db.Query --depth 2 --direction up
  wildcat tree Server.Start --scope package            # stay within package
  wildcat tree Handler.ServeHTTP --scope all           # include stdlib calls`,
	Args: cobra.ExactArgs(1),
	RunE: runTree,
}

var (
	treeDepth        int
	treeDirection    string
	treeExcludeTests bool
	treeScope        string
)

func init() {
	rootCmd.AddCommand(treeCmd)

	treeCmd.Flags().IntVar(&treeDepth, "depth", 3, "Maximum tree depth")
	treeCmd.Flags().StringVar(&treeDirection, "direction", "down", "Traversal direction: up or down")
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

	// Validate direction
	var direction traverse.Direction
	switch treeDirection {
	case "up":
		direction = traverse.Up
	case "down":
		direction = traverse.Down
	default:
		return writer.WriteError("invalid_argument", "direction must be 'up' or 'down'", nil, nil)
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
		Direction:    direction,
		MaxDepth:     treeDepth,
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
