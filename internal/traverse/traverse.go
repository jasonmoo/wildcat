// Package traverse provides call hierarchy traversal for Wildcat.
package traverse

import (
	"context"
	"strings"

	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
)

// Direction indicates traversal direction.
type Direction int

const (
	Up   Direction = iota // Callers (incoming calls)
	Down                  // Callees (outgoing calls)
)

// Options configures traversal behavior.
type Options struct {
	Direction     Direction
	MaxDepth      int
	ExcludeTests  bool
	ExcludeStdlib bool
}

// CallInfo contains information about a call site.
type CallInfo struct {
	Symbol     string
	File       string
	Line       int
	LineEnd    int
	CallRanges []lsp.Range // Where the calls happen
	InTest     bool
}

// Traverser walks the call hierarchy.
type Traverser struct {
	client    *lsp.Client
	extractor *output.SnippetExtractor
}

// NewTraverser creates a new call hierarchy traverser.
func NewTraverser(client *lsp.Client) *Traverser {
	return &Traverser{
		client:    client,
		extractor: output.NewSnippetExtractor(),
	}
}

// GetCallers returns all callers of a call hierarchy item.
func (t *Traverser) GetCallers(ctx context.Context, item lsp.CallHierarchyItem, opts Options) ([]CallInfo, error) {
	return t.traverse(ctx, item, opts, 0, make(map[string]bool))
}

// GetCallees returns all callees of a call hierarchy item.
func (t *Traverser) GetCallees(ctx context.Context, item lsp.CallHierarchyItem, opts Options) ([]CallInfo, error) {
	opts.Direction = Down
	return t.traverse(ctx, item, opts, 0, make(map[string]bool))
}

// traverse recursively walks the call hierarchy.
func (t *Traverser) traverse(ctx context.Context, item lsp.CallHierarchyItem, opts Options, depth int, visited map[string]bool) ([]CallInfo, error) {
	// Check depth limit
	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		return nil, nil
	}

	// Prevent cycles
	key := item.URI + ":" + item.Name
	if visited[key] {
		return nil, nil
	}
	visited[key] = true

	var results []CallInfo

	if opts.Direction == Up {
		// Get incoming calls (callers)
		calls, err := t.client.IncomingCalls(ctx, item)
		if err != nil {
			return nil, err
		}

		for _, call := range calls {
			info := t.callInfoFromIncoming(call)

			// Apply filters
			if opts.ExcludeTests && info.InTest {
				continue
			}
			if opts.ExcludeStdlib && t.isStdlib(call.From.URI) {
				continue
			}

			results = append(results, info)

			// Recurse
			if opts.MaxDepth == 0 || depth+1 < opts.MaxDepth {
				sub, err := t.traverse(ctx, call.From, opts, depth+1, visited)
				if err != nil {
					return nil, err
				}
				results = append(results, sub...)
			}
		}
	} else {
		// Get outgoing calls (callees)
		calls, err := t.client.OutgoingCalls(ctx, item)
		if err != nil {
			return nil, err
		}

		for _, call := range calls {
			info := t.callInfoFromOutgoing(call)

			// Apply filters
			if opts.ExcludeTests && info.InTest {
				continue
			}
			if opts.ExcludeStdlib && t.isStdlib(call.To.URI) {
				continue
			}

			results = append(results, info)

			// Recurse
			if opts.MaxDepth == 0 || depth+1 < opts.MaxDepth {
				sub, err := t.traverse(ctx, call.To, opts, depth+1, visited)
				if err != nil {
					return nil, err
				}
				results = append(results, sub...)
			}
		}
	}

	return results, nil
}

// callInfoFromIncoming creates CallInfo from an incoming call.
func (t *Traverser) callInfoFromIncoming(call lsp.CallHierarchyIncomingCall) CallInfo {
	file := lsp.URIToPath(call.From.URI)
	return CallInfo{
		Symbol:     call.From.Name,
		File:       file,
		Line:       call.From.Range.Start.Line + 1, // LSP is 0-indexed
		LineEnd:    call.From.Range.End.Line + 1,
		CallRanges: call.FromRanges,
		InTest:     output.IsTestFile(file),
	}
}

// callInfoFromOutgoing creates CallInfo from an outgoing call.
func (t *Traverser) callInfoFromOutgoing(call lsp.CallHierarchyOutgoingCall) CallInfo {
	file := lsp.URIToPath(call.To.URI)
	return CallInfo{
		Symbol:     call.To.Name,
		File:       file,
		Line:       call.To.Range.Start.Line + 1,
		LineEnd:    call.To.Range.End.Line + 1,
		CallRanges: call.FromRanges,
		InTest:     output.IsTestFile(file),
	}
}

// isStdlib checks if a URI is from the standard library.
func (t *Traverser) isStdlib(uri string) bool {
	path := lsp.URIToPath(uri)
	// Standard library is typically in GOROOT
	return strings.Contains(path, "/go/src/") ||
		strings.Contains(path, "/golang.org/") ||
		(!strings.Contains(path, "/") && !strings.HasPrefix(path, "."))
}

// BuildTree builds a call tree structure.
func (t *Traverser) BuildTree(ctx context.Context, item lsp.CallHierarchyItem, opts Options) (*output.TreeResponse, error) {
	nodes := make(map[string]output.TreeNode)
	var edges []output.TreeEdge

	visited := make(map[string]bool)
	maxDepth := 0

	err := t.buildTreeRecursive(ctx, item, opts, 0, visited, nodes, &edges, &maxDepth)
	if err != nil {
		return nil, err
	}

	direction := "down"
	if opts.Direction == Up {
		direction = "up"
	}

	return &output.TreeResponse{
		Query: output.TreeQuery{
			Command:   "tree",
			Root:      item.Name,
			Depth:     opts.MaxDepth,
			Direction: direction,
		},
		Nodes: nodes,
		Edges: edges,
		Summary: output.TreeSummary{
			NodeCount:       len(nodes),
			EdgeCount:       len(edges),
			MaxDepthReached: maxDepth,
			Truncated:       opts.MaxDepth > 0 && maxDepth >= opts.MaxDepth,
		},
	}, nil
}

// buildTreeRecursive builds the tree structure recursively.
func (t *Traverser) buildTreeRecursive(ctx context.Context, item lsp.CallHierarchyItem, opts Options, depth int, visited map[string]bool, nodes map[string]output.TreeNode, edges *[]output.TreeEdge, maxDepth *int) error {
	if depth > *maxDepth {
		*maxDepth = depth
	}

	if opts.MaxDepth > 0 && depth >= opts.MaxDepth {
		return nil
	}

	key := item.URI + ":" + item.Name
	if visited[key] {
		return nil
	}
	visited[key] = true

	file := lsp.URIToPath(item.URI)

	// Add node if not exists
	if _, exists := nodes[item.Name]; !exists {
		nodes[item.Name] = output.TreeNode{
			File: file,
			Line: item.Range.Start.Line + 1,
		}
	}

	if opts.Direction == Up {
		calls, err := t.client.IncomingCalls(ctx, item)
		if err != nil {
			return err
		}

		node := nodes[item.Name]
		for _, call := range calls {
			if opts.ExcludeTests && output.IsTestFile(lsp.URIToPath(call.From.URI)) {
				continue
			}
			if opts.ExcludeStdlib && t.isStdlib(call.From.URI) {
				continue
			}

			node.CalledBy = append(node.CalledBy, call.From.Name)

			// Add edge
			for _, r := range call.FromRanges {
				*edges = append(*edges, output.TreeEdge{
					From: call.From.Name,
					To:   item.Name,
					File: lsp.URIToPath(call.From.URI),
					Line: r.Start.Line + 1,
				})
			}

			// Recurse
			if err := t.buildTreeRecursive(ctx, call.From, opts, depth+1, visited, nodes, edges, maxDepth); err != nil {
				return err
			}
		}
		nodes[item.Name] = node
	} else {
		calls, err := t.client.OutgoingCalls(ctx, item)
		if err != nil {
			return err
		}

		node := nodes[item.Name]
		for _, call := range calls {
			if opts.ExcludeTests && output.IsTestFile(lsp.URIToPath(call.To.URI)) {
				continue
			}
			if opts.ExcludeStdlib && t.isStdlib(call.To.URI) {
				continue
			}

			node.Calls = append(node.Calls, call.To.Name)

			// Add edge
			for _, r := range call.FromRanges {
				*edges = append(*edges, output.TreeEdge{
					From: item.Name,
					To:   call.To.Name,
					File: file,
					Line: r.Start.Line + 1,
				})
			}

			// Recurse
			if err := t.buildTreeRecursive(ctx, call.To, opts, depth+1, visited, nodes, edges, maxDepth); err != nil {
				return err
			}
		}
		nodes[item.Name] = node
	}

	return nil
}
