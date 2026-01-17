package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DebugBuffer collects debug messages for later output.
type DebugBuffer struct {
	mu      sync.Mutex
	entries []debugEntry
	start   time.Time
}

type debugEntry struct {
	elapsed time.Duration
	message string
}

// NewDebugBuffer creates a new debug buffer.
func NewDebugBuffer() *DebugBuffer {
	return &DebugBuffer{start: time.Now()}
}

// Log adds a message to the buffer.
func (b *DebugBuffer) Log(format string, args ...any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = append(b.entries, debugEntry{
		elapsed: time.Since(b.start),
		message: fmt.Sprintf(format, args...),
	})
}

// Dump writes all collected messages to stderr.
func (b *DebugBuffer) Dump() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range b.entries {
		fmt.Fprintf(os.Stderr, "[%s] %s\n", e.elapsed.Truncate(time.Microsecond), e.message)
	}
}

// Client provides high-level access to LSP server functionality.
type Client struct {
	server      *Server
	rootURI     string
	initialized bool

	// Progress tracking for LSP server readiness
	activeProgress map[string]bool // tokens with "begin" but not yet "end"
	serverReady    chan struct{}

	// DebugLog, if set, receives debug messages about LSP notifications
	DebugLog func(format string, args ...any)
}

// NewClient creates a new LSP client with the given server configuration.
func NewClient(ctx context.Context, config ServerConfig) (*Client, error) {
	server, err := StartServer(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("starting server: %w", err)
	}

	rootURI := "file://" + config.WorkDir

	c := &Client{
		server:         server,
		rootURI:        rootURI,
		activeProgress: make(map[string]bool),
		serverReady:    make(chan struct{}),
	}

	// Set up notification handler for progress tracking
	server.Conn().SetNotificationHandler(c.handleNotification)

	return c, nil
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
				DocumentSymbol: DocumentSymbolClientCapabilities{
					HierarchicalDocumentSymbolSupport: true,
				},
			},
			Workspace: WorkspaceClientCapabilities{
				Symbol: WorkspaceSymbolClientCapabilities{
					DynamicRegistration: false,
				},
			},
			Window: WindowClientCapabilities{
				WorkDoneProgress: true,
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

// handleNotification processes incoming notifications from the LSP server.
func (c *Client) handleNotification(method string, params json.RawMessage) {
	if c.DebugLog != nil {
		c.DebugLog("[LSP NOTIFY] method=%s params=%s", method, string(params))
	}

	if method != "$/progress" {
		return
	}

	var progress ProgressParams
	if err := json.Unmarshal(params, &progress); err != nil {
		if c.DebugLog != nil {
			c.DebugLog("[PROGRESS] failed to unmarshal params: %v", err)
		}
		return
	}

	// Parse the value to determine the kind
	var kindPeek struct {
		Kind  string `json:"kind"`
		Title string `json:"title"`
	}
	valueBytes, err := json.Marshal(progress.Value)
	if err != nil {
		return
	}
	if err := json.Unmarshal(valueBytes, &kindPeek); err != nil {
		return
	}

	if c.DebugLog != nil {
		c.DebugLog("[PROGRESS] kind=%s token=%s title=%q activeCount=%d", kindPeek.Kind, progress.Token, kindPeek.Title, len(c.activeProgress))
	}

	switch kindPeek.Kind {
	case "begin":
		c.activeProgress[progress.Token] = true
		if c.DebugLog != nil {
			c.DebugLog("[PROGRESS] BEGIN token=%s title=%q activeCount=%d", progress.Token, kindPeek.Title, len(c.activeProgress))
		}
	case "end":
		delete(c.activeProgress, progress.Token)
		if c.DebugLog != nil {
			c.DebugLog("[PROGRESS] END token=%s activeCount=%d", progress.Token, len(c.activeProgress))
		}
	}

	if len(c.activeProgress) == 0 {
		select {
		case <-c.serverReady:
			if c.DebugLog != nil {
				c.DebugLog("[PROGRESS] serverReady already closed")
			}
		default:
			if c.DebugLog != nil {
				c.DebugLog("[PROGRESS] closing serverReady channel")
			}
			close(c.serverReady)
		}
	}
}

// WaitForReady blocks until the LSP server has finished initial indexing.
func (c *Client) WaitForReady(ctx context.Context) error {
	if c.DebugLog != nil {
		c.DebugLog("[WAIT] WaitForReady called, activeProgress=%d", len(c.activeProgress))
	}
	select {
	case <-ctx.Done():
		if c.DebugLog != nil {
			c.DebugLog("[WAIT] context done before ready")
		}
		return ctx.Err()
	case <-c.serverReady:
		if c.DebugLog != nil {
			c.DebugLog("[WAIT] serverReady signaled, returning")
		}
		return nil
	}
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

// PrepareTypeHierarchy prepares type hierarchy information at a position.
func (c *Client) PrepareTypeHierarchy(ctx context.Context, uri string, pos Position) ([]TypeHierarchyItem, error) {
	params := TypeHierarchyPrepareParams{
		TextDocumentPositionParams: TextDocumentPositionParams{
			TextDocument: TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	}

	var result []TypeHierarchyItem
	if err := c.server.Conn().Call("textDocument/prepareTypeHierarchy", params, &result); err != nil {
		return nil, fmt.Errorf("textDocument/prepareTypeHierarchy: %w", err)
	}

	return result, nil
}

// Supertypes returns the supertypes (implemented interfaces) of a type.
func (c *Client) Supertypes(ctx context.Context, item TypeHierarchyItem) ([]TypeHierarchyItem, error) {
	params := TypeHierarchySupertypesParams{
		Item: item,
	}

	var result []TypeHierarchyItem
	if err := c.server.Conn().Call("typeHierarchy/supertypes", params, &result); err != nil {
		return nil, fmt.Errorf("typeHierarchy/supertypes: %w", err)
	}

	return result, nil
}

// Subtypes returns the subtypes (implementing types) of a type or interface.
func (c *Client) Subtypes(ctx context.Context, item TypeHierarchyItem) ([]TypeHierarchyItem, error) {
	params := TypeHierarchySubtypesParams{
		Item: item,
	}

	var result []TypeHierarchyItem
	if err := c.server.Conn().Call("typeHierarchy/subtypes", params, &result); err != nil {
		return nil, fmt.Errorf("typeHierarchy/subtypes: %w", err)
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

// DocumentSymbol returns the symbols in a document with hierarchy.
func (c *Client) DocumentSymbol(ctx context.Context, uri string) ([]DocumentSymbol, error) {
	params := DocumentSymbolParams{
		TextDocument: TextDocumentIdentifier{URI: uri},
	}

	var result []DocumentSymbol
	if err := c.server.Conn().Call("textDocument/documentSymbol", params, &result); err != nil {
		return nil, fmt.Errorf("textDocument/documentSymbol: %w", err)
	}

	return result, nil
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
