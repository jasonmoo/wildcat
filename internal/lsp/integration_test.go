package lsp_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonmoo/wildcat/internal/lsp"
)

// TestGoplsIntegration tests the LSP client against a real gopls server.
// Skip if gopls is not available.
func TestGoplsIntegration(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not found in PATH")
	}

	// Create a temporary Go project for testing
	tmpDir, err := os.MkdirTemp("", "wildcat-lsp-test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write a simple Go module
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module testmod\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	// Write a simple Go file with a function
	mainGo := `package main

func main() {
	hello()
}

func hello() {
	println("Hello, World!")
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	// Create LSP client
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := lsp.ServerConfig{
		Command: "gopls",
		Args:    []string{"serve"},
		WorkDir: tmpDir,
	}

	client, err := lsp.NewClient(ctx, config)
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	defer client.Close()

	// Initialize
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// Give gopls a moment to index
	time.Sleep(500 * time.Millisecond)

	// Test workspace/symbol
	t.Run("WorkspaceSymbol", func(t *testing.T) {
		symbols, err := client.WorkspaceSymbol(ctx, "hello")
		if err != nil {
			t.Fatalf("workspace/symbol: %v", err)
		}

		if len(symbols) == 0 {
			t.Error("expected at least one symbol, got none")
		}

		found := false
		for _, sym := range symbols {
			if sym.Name == "hello" {
				found = true
				if sym.Kind != lsp.SymbolKindFunction {
					t.Errorf("expected function kind, got %d", sym.Kind)
				}
			}
		}
		if !found {
			t.Error("hello function not found in symbols")
		}
	})

	// Test call hierarchy
	t.Run("CallHierarchy", func(t *testing.T) {
		uri := lsp.FileURI(filepath.Join(tmpDir, "main.go"))

		// Position of "hello" function definition (line 6, character 5)
		pos := lsp.Position{Line: 6, Character: 5}

		items, err := client.PrepareCallHierarchy(ctx, uri, pos)
		if err != nil {
			t.Fatalf("prepareCallHierarchy: %v", err)
		}

		if len(items) == 0 {
			t.Fatal("expected at least one call hierarchy item")
		}

		// Get incoming calls (callers of hello)
		incoming, err := client.IncomingCalls(ctx, items[0])
		if err != nil {
			t.Fatalf("incomingCalls: %v", err)
		}

		if len(incoming) == 0 {
			t.Error("expected at least one incoming call (from main)")
		}

		callerFound := false
		for _, call := range incoming {
			if call.From.Name == "main" {
				callerFound = true
			}
		}
		if !callerFound {
			t.Error("main function should be a caller of hello")
		}
	})

	// Test references
	t.Run("References", func(t *testing.T) {
		uri := lsp.FileURI(filepath.Join(tmpDir, "main.go"))

		// Position of "hello" function call in main (line 3, character 1)
		pos := lsp.Position{Line: 3, Character: 1}

		refs, err := client.References(ctx, uri, pos, true)
		if err != nil {
			t.Fatalf("references: %v", err)
		}

		if len(refs) < 2 {
			t.Errorf("expected at least 2 references (definition + call), got %d", len(refs))
		}
	})

	// Shutdown gracefully
	if err := client.Shutdown(ctx); err != nil {
		t.Errorf("shutdown: %v", err)
	}
}
