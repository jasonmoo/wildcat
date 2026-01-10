// Package lsp provides a client for communicating with Language Server Protocol servers.
package lsp

// Position in a text document expressed as zero-based line and character offset.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range in a text document expressed as start and end positions.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location represents a location inside a resource, such as a line inside a text file.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextDocumentIdentifier identifies a text document using a URI.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentPositionParams is a parameter literal used in requests to pass
// a text document and a position inside that document.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// SymbolKind represents the kind of a symbol.
type SymbolKind int

const (
	SymbolKindFile          SymbolKind = 1
	SymbolKindModule        SymbolKind = 2
	SymbolKindNamespace     SymbolKind = 3
	SymbolKindPackage       SymbolKind = 4
	SymbolKindClass         SymbolKind = 5
	SymbolKindMethod        SymbolKind = 6
	SymbolKindProperty      SymbolKind = 7
	SymbolKindField         SymbolKind = 8
	SymbolKindConstructor   SymbolKind = 9
	SymbolKindEnum          SymbolKind = 10
	SymbolKindInterface     SymbolKind = 11
	SymbolKindFunction      SymbolKind = 12
	SymbolKindVariable      SymbolKind = 13
	SymbolKindConstant      SymbolKind = 14
	SymbolKindString        SymbolKind = 15
	SymbolKindNumber        SymbolKind = 16
	SymbolKindBoolean       SymbolKind = 17
	SymbolKindArray         SymbolKind = 18
	SymbolKindObject        SymbolKind = 19
	SymbolKindKey           SymbolKind = 20
	SymbolKindNull          SymbolKind = 21
	SymbolKindEnumMember    SymbolKind = 22
	SymbolKindStruct        SymbolKind = 23
	SymbolKindEvent         SymbolKind = 24
	SymbolKindOperator      SymbolKind = 25
	SymbolKindTypeParameter SymbolKind = 26
)

// SymbolInformation represents information about programming constructs like
// variables, classes, interfaces etc.
type SymbolInformation struct {
	Name          string     `json:"name"`
	Kind          SymbolKind `json:"kind"`
	Location      Location   `json:"location"`
	ContainerName string     `json:"containerName,omitempty"`
}

// WorkspaceSymbolParams is the parameters for a workspace/symbol request.
type WorkspaceSymbolParams struct {
	Query string `json:"query"`
}

// CallHierarchyItem represents an item in the call hierarchy.
type CallHierarchyItem struct {
	Name           string     `json:"name"`
	Kind           SymbolKind `json:"kind"`
	Tags           []int      `json:"tags,omitempty"`
	Detail         string     `json:"detail,omitempty"`
	URI            string     `json:"uri"`
	Range          Range      `json:"range"`
	SelectionRange Range      `json:"selectionRange"`
	Data           any        `json:"data,omitempty"`
}

// CallHierarchyIncomingCall represents an incoming call (a caller).
type CallHierarchyIncomingCall struct {
	From       CallHierarchyItem `json:"from"`
	FromRanges []Range           `json:"fromRanges"`
}

// CallHierarchyOutgoingCall represents an outgoing call (a callee).
type CallHierarchyOutgoingCall struct {
	To         CallHierarchyItem `json:"to"`
	FromRanges []Range           `json:"fromRanges"`
}

// CallHierarchyPrepareParams is the parameter for textDocument/prepareCallHierarchy.
type CallHierarchyPrepareParams struct {
	TextDocumentPositionParams
}

// CallHierarchyIncomingCallsParams is the parameter for callHierarchy/incomingCalls.
type CallHierarchyIncomingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

// CallHierarchyOutgoingCallsParams is the parameter for callHierarchy/outgoingCalls.
type CallHierarchyOutgoingCallsParams struct {
	Item CallHierarchyItem `json:"item"`
}

// ReferenceContext is used in ReferenceParams.
type ReferenceContext struct {
	IncludeDeclaration bool `json:"includeDeclaration"`
}

// ReferenceParams is the parameter for textDocument/references.
type ReferenceParams struct {
	TextDocumentPositionParams
	Context ReferenceContext `json:"context"`
}

// ImplementationParams is the parameter for textDocument/implementation.
type ImplementationParams struct {
	TextDocumentPositionParams
}

// TypeHierarchyPrepareParams is the parameter for textDocument/prepareTypeHierarchy.
type TypeHierarchyPrepareParams struct {
	TextDocumentPositionParams
}

// TypeHierarchyItem represents an item in the type hierarchy.
type TypeHierarchyItem struct {
	Name           string   `json:"name"`
	Kind           SymbolKind `json:"kind"`
	Tags           []int    `json:"tags,omitempty"`
	Detail         string   `json:"detail,omitempty"`
	URI            string   `json:"uri"`
	Range          Range    `json:"range"`
	SelectionRange Range    `json:"selectionRange"`
	Data           any      `json:"data,omitempty"`
}

// TypeHierarchySupertypesParams is the parameter for typeHierarchy/supertypes.
type TypeHierarchySupertypesParams struct {
	Item TypeHierarchyItem `json:"item"`
}

// TypeHierarchySubtypesParams is the parameter for typeHierarchy/subtypes.
type TypeHierarchySubtypesParams struct {
	Item TypeHierarchyItem `json:"item"`
}

// InitializeParams is the parameter for the initialize request.
type InitializeParams struct {
	ProcessID    int          `json:"processId"`
	RootURI      string       `json:"rootUri"`
	Capabilities Capabilities `json:"capabilities"`
}

// Capabilities represents client capabilities.
type Capabilities struct {
	TextDocument TextDocumentClientCapabilities `json:"textDocument,omitempty"`
	Workspace    WorkspaceClientCapabilities    `json:"workspace,omitempty"`
}

// TextDocumentClientCapabilities defines capabilities for text document features.
type TextDocumentClientCapabilities struct {
	CallHierarchy CallHierarchyClientCapabilities `json:"callHierarchy,omitempty"`
	References    ReferencesClientCapabilities    `json:"references,omitempty"`
}

// CallHierarchyClientCapabilities defines capabilities for call hierarchy.
type CallHierarchyClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// ReferencesClientCapabilities defines capabilities for references.
type ReferencesClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// WorkspaceClientCapabilities defines capabilities for workspace features.
type WorkspaceClientCapabilities struct {
	Symbol WorkspaceSymbolClientCapabilities `json:"symbol,omitempty"`
}

// WorkspaceSymbolClientCapabilities defines capabilities for workspace symbols.
type WorkspaceSymbolClientCapabilities struct {
	DynamicRegistration bool `json:"dynamicRegistration,omitempty"`
}

// InitializeResult is the result of the initialize request.
type InitializeResult struct {
	Capabilities ServerCapabilities `json:"capabilities"`
}

// ServerCapabilities describes the capabilities provided by the server.
type ServerCapabilities struct {
	CallHierarchyProvider   any  `json:"callHierarchyProvider,omitempty"`
	ReferencesProvider      any  `json:"referencesProvider,omitempty"`
	ImplementationProvider  any  `json:"implementationProvider,omitempty"`
	WorkspaceSymbolProvider bool `json:"workspaceSymbolProvider,omitempty"`
}

// TextDocumentItem is an item to transfer a text document from the client to the server.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// DidOpenTextDocumentParams is the parameter for textDocument/didOpen.
type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

// DidCloseTextDocumentParams is the parameter for textDocument/didClose.
type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}
