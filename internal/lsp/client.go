package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Client provides high-level access to LSP server functionality.
type Client struct {
	server      *Server
	rootURI     string
	initialized bool
}

// NewClient creates a new LSP client with the given server configuration.
func NewClient(ctx context.Context, config ServerConfig) (*Client, error) {
	server, err := StartServer(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("starting server: %w", err)
	}

	rootURI := "file://" + config.WorkDir

	return &Client{
		server:  server,
		rootURI: rootURI,
	}, nil
}

// Initialize performs the LSP initialize handshake.
func (c *Client) Initialize(ctx context.Context) error {
	if c.initialized {
		return nil
	}

	params := InitializeParams{
		ProcessID: os.Getpid(),
		RootURI:   c.rootURI,
		Capabilities: Capabilities{
			TextDocument: TextDocumentClientCapabilities{
				CallHierarchy: CallHierarchyClientCapabilities{
					DynamicRegistration: false,
				},
				References: ReferencesClientCapabilities{
					DynamicRegistration: false,
				},
			},
			Workspace: WorkspaceClientCapabilities{
				Symbol: WorkspaceSymbolClientCapabilities{
					DynamicRegistration: false,
				},
			},
		},
	}

	var result InitializeResult
	if err := c.server.Conn().Call("initialize", params, &result); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	// Send initialized notification
	if err := c.server.Conn().Notify("initialized", struct{}{}); err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}

	c.initialized = true
	return nil
}

// Shutdown gracefully shuts down the LSP server.
func (c *Client) Shutdown(ctx context.Context) error {
	if !c.initialized {
		return nil
	}

	// Send shutdown request
	if err := c.server.Conn().Call("shutdown", nil, nil); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	// Send exit notification
	if err := c.server.Conn().Notify("exit", nil); err != nil {
		return fmt.Errorf("exit notification: %w", err)
	}

	return c.server.Stop()
}

// Close closes the client connection.
func (c *Client) Close() error {
	return c.server.Stop()
}

// WorkspaceSymbol searches for symbols in the workspace.
func (c *Client) WorkspaceSymbol(ctx context.Context, query string) ([]SymbolInformation, error) {
	params := WorkspaceSymbolParams{
		Query: query,
	}

	var result []SymbolInformation
	if err := c.server.Conn().Call("workspace/symbol", params, &result); err != nil {
		return nil, fmt.Errorf("workspace/symbol: %w", err)
	}

	return result, nil
}

// PrepareCallHierarchy prepares a call hierarchy for a position.
func (c *Client) PrepareCallHierarchy(ctx context.Context, uri string, pos Position) ([]CallHierarchyItem, error) {
	params := CallHierarchyPrepareParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	}

	var result []CallHierarchyItem
	if err := c.server.Conn().Call("textDocument/prepareCallHierarchy", params, &result); err != nil {
		return nil, fmt.Errorf("textDocument/prepareCallHierarchy: %w", err)
	}

	return result, nil
}

// IncomingCalls returns the callers of a call hierarchy item.
func (c *Client) IncomingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyIncomingCall, error) {
	params := CallHierarchyIncomingCallsParams{
		Item: item,
	}

	var result []CallHierarchyIncomingCall
	if err := c.server.Conn().Call("callHierarchy/incomingCalls", params, &result); err != nil {
		return nil, fmt.Errorf("callHierarchy/incomingCalls: %w", err)
	}

	return result, nil
}

// OutgoingCalls returns the callees of a call hierarchy item.
func (c *Client) OutgoingCalls(ctx context.Context, item CallHierarchyItem) ([]CallHierarchyOutgoingCall, error) {
	params := CallHierarchyOutgoingCallsParams{
		Item: item,
	}

	var result []CallHierarchyOutgoingCall
	if err := c.server.Conn().Call("callHierarchy/outgoingCalls", params, &result); err != nil {
		return nil, fmt.Errorf("callHierarchy/outgoingCalls: %w", err)
	}

	return result, nil
}

// References finds all references to a symbol at a position.
func (c *Client) References(ctx context.Context, uri string, pos Position, includeDeclaration bool) ([]Location, error) {
	params := ReferenceParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
		Context: ReferenceContext{
			IncludeDeclaration: includeDeclaration,
		},
	}

	var result []Location
	if err := c.server.Conn().Call("textDocument/references", params, &result); err != nil {
		return nil, fmt.Errorf("textDocument/references: %w", err)
	}

	return result, nil
}

// Implementation finds the implementations of an interface or abstract method.
func (c *Client) Implementation(ctx context.Context, uri string, pos Position) ([]Location, error) {
	params := ImplementationParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	}

	var result []Location
	if err := c.server.Conn().Call("textDocument/implementation", params, &result); err != nil {
		return nil, fmt.Errorf("textDocument/implementation: %w", err)
	}

	return result, nil
}

// DidOpen notifies the server that a document was opened.
func (c *Client) DidOpen(ctx context.Context, uri, languageID, text string) error {
	params := DidOpenTextDocumentParams{
		TextDocument: TextDocumentItem{
			URI:        uri,
			LanguageID: languageID,
			Version:    1,
			Text:       text,
		},
	}

	return c.server.Conn().Notify("textDocument/didOpen", params)
}

// DidClose notifies the server that a document was closed.
func (c *Client) DidClose(ctx context.Context, uri string) error {
	params := DidCloseTextDocumentParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}

	return c.server.Conn().Notify("textDocument/didClose", params)
}

// FileURI converts a file path to a file:// URI.
func FileURI(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "file://" + path
	}
	return "file://" + absPath
}

// URIToPath converts a file:// URI to a file path.
func URIToPath(uri string) string {
	if len(uri) > 7 && uri[:7] == "file://" {
		return uri[7:]
	}
	return uri
}
