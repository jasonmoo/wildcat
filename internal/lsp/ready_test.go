package lsp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitForReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Find project root (go up from internal/lsp to repo root)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	workDir := filepath.Join(wd, "..", "..")

	client, err := NewClient(ctx, ServerConfig{
		Command: "gopls",
		Args:    []string{"serve"},
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Shutdown(ctx)

	t.Log("Initializing...")
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	t.Log("Waiting for ready...")
	if err := client.WaitForReady(ctx); err != nil {
		t.Fatalf("WaitForReady: %v", err)
	}

	t.Log("Server ready! Doing a test query...")

	// Quick sanity check - query for a symbol we know exists
	symbols, err := client.WorkspaceSymbol(ctx, "Client")
	if err != nil {
		t.Fatalf("WorkspaceSymbol: %v", err)
	}

	t.Logf("Found %d symbols matching 'Client'", len(symbols))
	if len(symbols) == 0 {
		t.Fatal("Expected at least one symbol")
	}
	// Show first few results
	for i, s := range symbols {
		if i >= 3 {
			t.Logf("  ... and %d more", len(symbols)-3)
			break
		}
		t.Logf("  %s (%s)", s.Name, s.Kind)
	}
}
