package traverse

import (
	"context"
	"testing"
	"time"

	"github.com/jasonmoo/wildcat/internal/lsp"
)

// TestStdlibPathDetection explores what stdlib paths look like from gopls
// so we can build a proper detection function.
func TestStdlibPathDetection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	config := lsp.ServerConfig{
		Command: "gopls",
		Args:    []string{"serve"},
		WorkDir: "/home/jason/go/src/github.com/jasonmoo/wildcat",
	}

	client, err := lsp.NewClient(ctx, config)
	if err != nil {
		t.Fatalf("creating LSP client: %v", err)
	}
	defer client.Close()

	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("initializing LSP: %v", err)
	}
	defer client.Shutdown(ctx)

	time.Sleep(500 * time.Millisecond)

	// Find BuildTree via workspace symbol search
	syms, err := client.WorkspaceSymbol(ctx, "BuildTree")
	if err != nil {
		t.Fatalf("workspace symbol search: %v", err)
	}
	if len(syms) == 0 {
		t.Fatal("no symbols found for BuildTree")
	}

	// Find the Traverser.BuildTree method (first one should be ours)
	var targetSym lsp.SymbolInformation
	for _, s := range syms {
		t.Logf("Found symbol: %s (container: %s) at %s", s.Name, s.ContainerName, s.Location.URI)
		if s.Name == "Traverser.BuildTree" || (s.Name == "BuildTree" && s.ContainerName == "github.com/jasonmoo/wildcat/internal/traverse") {
			targetSym = s
			break
		}
	}
	if targetSym.Name == "" {
		t.Fatal("could not find Traverser.BuildTree symbol")
	}

	t.Logf("")
	t.Logf("=== SELECTED TARGET ===")
	t.Logf("Name: %s", targetSym.Name)
	t.Logf("Container: %s", targetSym.ContainerName)
	t.Logf("URI: %s", targetSym.Location.URI)
	t.Logf("")

	// Prepare call hierarchy from the symbol location
	items, err := client.PrepareCallHierarchy(ctx, targetSym.Location.URI, targetSym.Location.Range.Start)
	if err != nil {
		t.Fatalf("preparing call hierarchy: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("no call hierarchy items found")
	}

	t.Logf("Target: %s", items[0].Name)
	t.Logf("Target URI: %s", items[0].URI)
	t.Logf("Target Path: %s", lsp.URIToPath(items[0].URI))
	t.Logf("")

	// Get outgoing calls (callees) - should include stdlib like fmt.Sprintf
	outgoing, err := client.OutgoingCalls(ctx, items[0])
	if err != nil {
		t.Fatalf("getting outgoing calls: %v", err)
	}

	t.Logf("=== OUTGOING CALLS (callees) ===")
	for _, call := range outgoing {
		path := lsp.URIToPath(call.To.URI)
		t.Logf("Name: %-30s URI: %s", call.To.Name, call.To.URI)
		t.Logf("  Path: %s", path)
		t.Logf("")
	}
}
