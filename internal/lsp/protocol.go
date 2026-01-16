// Package lsp provides a client for communicating with Language Server Protocol servers.
package lsp

import "strings"

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

// String returns the human-readable name of a SymbolKind.
func (k SymbolKind) String() string {
	names := map[SymbolKind]string{
		SymbolKindFile:          "file",
		SymbolKindModule:        "module",
		SymbolKindNamespace:     "namespace",
		SymbolKindPackage:       "package",
		SymbolKindClass:         "class",
		SymbolKindMethod:        "method",
		SymbolKindProperty:      "property",
		SymbolKindField:         "field",
		SymbolKindConstructor:   "constructor",
		SymbolKindEnum:          "enum",
		SymbolKindInterface:     "interface",
		SymbolKindFunction:      "function",
		SymbolKindVariable:      "variable",
		SymbolKindConstant:      "constant",
		SymbolKindString:        "string",
		SymbolKindNumber:        "number",
		SymbolKindBoolean:       "boolean",
		SymbolKindArray:         "array",
		SymbolKindObject:        "object",
		SymbolKindKey:           "key",
		SymbolKindNull:          "null",
		SymbolKindEnumMember:    "enum_member",
		SymbolKindStruct:        "struct",
		SymbolKindEvent:         "event",
		SymbolKindOperator:      "operator",
		SymbolKindTypeParameter: "type_parameter",
	}
	if name, ok := names[k]; ok {
		return name
	}
	return "unknown"
}

// SymbolInformation represents information about programming constructs like
// variables, classes, interfaces etc.
type SymbolInformation struct {
	Name          string     `json:"name"`
	Kind          SymbolKind `json:"kind"`
	Location      Location   `json:"location"`
	ContainerName string     `json:"containerName,omitempty"`
}

// FullName returns a fully qualified "import/path.Symbol" format.
// Uses the full ContainerName as the import path prefix.
func (s SymbolInformation) FullName() string {
	if s.ContainerName == "" {
		return s.Name
	}

	// If Name already starts with full ContainerName, it's already fully qualified
	if strings.HasPrefix(s.Name, s.ContainerName+".") {
		return s.Name
	}

	// Get short package name to check if Name has it
	shortPkg := s.ContainerName
	if idx := strings.LastIndex(s.ContainerName, "/"); idx >= 0 {
		shortPkg = s.ContainerName[idx+1:]
	}

	// If Name has the short package prefix, strip it and use full path
	if strings.HasPrefix(s.Name, shortPkg+".") {
		name := strings.TrimPrefix(s.Name, shortPkg+".")
		return s.ContainerName + "." + name
	}

	return s.ContainerName + "." + s.Name
}

// ShortName returns a consistent "pkg.Symbol" format.
// Uses the last segment of ContainerName as the package prefix.
func (s SymbolInformation) ShortName() string {
	if s.ContainerName == "" {
		return s.Name
	}

	// Get short package name (last path segment)
	shortPkg := s.ContainerName
	if idx := strings.LastIndex(s.ContainerName, "/"); idx >= 0 {
		shortPkg = s.ContainerName[idx+1:]
	}

	// Check if Name already has the package prefix
	if strings.HasPrefix(s.Name, shortPkg+".") {
		return s.Name
	}

	return shortPkg + "." + s.Name
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
	Window       WindowClientCapabilities       `json:"window,omitempty"`
}

// TextDocumentClientCapabilities defines capabilities for text document features.
type TextDocumentClientCapabilities struct {
	CallHierarchy  CallHierarchyClientCapabilities  `json:"callHierarchy,omitempty"`
	References     ReferencesClientCapabilities     `json:"references,omitempty"`
	DocumentSymbol DocumentSymbolClientCapabilities `json:"documentSymbol,omitempty"`
}

// DocumentSymbolClientCapabilities defines capabilities for document symbols.
type DocumentSymbolClientCapabilities struct {
	DynamicRegistration               bool `json:"dynamicRegistration,omitempty"`
	HierarchicalDocumentSymbolSupport bool `json:"hierarchicalDocumentSymbolSupport,omitempty"`
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

// DocumentSymbol represents a symbol in a document with hierarchy support.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           SymbolKind       `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// DocumentSymbolParams is the parameter for textDocument/documentSymbol.
type DocumentSymbolParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

// ProgressParams is the parameter for $/progress notifications.
type ProgressParams struct {
	Token string `json:"token"`
	Value any    `json:"value"`
}

// WorkDoneProgressBegin signals the start of a long-running operation.
type WorkDoneProgressBegin struct {
	Kind        string `json:"kind"` // Always "begin"
	Title       string `json:"title"`
	Cancellable bool   `json:"cancellable,omitempty"`
	Message     string `json:"message,omitempty"`
	Percentage  int    `json:"percentage,omitempty"`
}

// WorkDoneProgressReport signals progress during a long-running operation.
type WorkDoneProgressReport struct {
	Kind        string `json:"kind"` // Always "report"
	Cancellable bool   `json:"cancellable,omitempty"`
	Message     string `json:"message,omitempty"`
	Percentage  int    `json:"percentage,omitempty"`
}

// WorkDoneProgressEnd signals the end of a long-running operation.
type WorkDoneProgressEnd struct {
	Kind    string `json:"kind"` // Always "end"
	Message string `json:"message,omitempty"`
}

// WindowClientCapabilities defines capabilities for window features.
type WindowClientCapabilities struct {
	WorkDoneProgress bool `json:"workDoneProgress,omitempty"`
}
