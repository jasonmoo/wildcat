package package_cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
)

type PackageCommand struct {
	workDir string
	pkgPath string
}

var _ commands.Command[*PackageCommand] = (*PackageCommand)(nil)

func WithPackage(path string) func(*PackageCommand) error {
	return func(c *PackageCommand) error {
		c.pkgPath = path
		return nil
	}
}

func WithWorkDir(path string) func(*PackageCommand) error {
	return func(c *PackageCommand) error {
		c.workDir = path
		return nil
	}
}

func NewPackageCommand() *PackageCommand {
	return &PackageCommand{}
}

type goListPackage struct {
	Dir        string   `json:"Dir"`
	ImportPath string   `json:"ImportPath"`
	Name       string   `json:"Name"`
	GoFiles    []string `json:"GoFiles"`
	Imports    []string `json:"Imports"`
}

func (c *PackageCommand) Execute(ctx context.Context, opts ...func(*PackageCommand) error) (commands.Result, *commands.Error) {

	// handle opts
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, commands.NewErrorf("opts_error", "failed to apply opt: %w", err)
		}
	}

	// ensure workdir
	if c.workDir == "" {
		return nil, commands.NewErrorf(
			"missing_required_param", "WithWorkDir is required for this command",
		)
	}
	resolvePkgPath, err := golang.ResolvePackagePath(c.pkgPath, c.workDir)
	if err != nil {
		// Suggestions: []string, TODO
		return nil, commands.NewErrorf("package_not_found", "failed to resolve package: %w", err)
	}

	goCmd := exec.Command("go", "list", "-json", resolvePkgPath)
	goCmd.Dir = c.workDir
	out, err := goCmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, commands.NewErrorf("go_list_error", "go list failed: %s", string(exitErr.Stderr))
		}
		return nil, commands.NewErrorf("go_list_error", "%w", err)
	}

	// goListPackage represents the JSON output from `go list -json`
	var pkg goListPackage
	if err := json.Unmarshal(out, &pkg); err != nil {
		return nil, commands.NewErrorf("parse_error", "%w", err)
	}

	client, err := lsp.NewClient(ctx, lsp.ServerConfig{
		Command: "gopls",
		Args:    []string{"serve"},
		WorkDir: c.workDir,
	})
	if err != nil {
		return nil, commands.NewErrorf("lsp_error", "%w", err)
	}
	defer client.Close()

	// Collect symbols from all Go files
	collector := newPackageCollector(pkg.Dir)

	// Process files alphabetically
	files := make([]string, len(pkg.GoFiles))
	copy(files, pkg.GoFiles)
	sort.Strings(files)

	for _, file := range files {
		fullPath := filepath.Join(pkg.Dir, file)
		uri := lsp.FileURI(fullPath)

		symbols, err := client.DocumentSymbol(ctx, uri)
		if err != nil {
			continue // Skip files that fail
		}

		if err := collector.addFile(fullPath, symbols); err != nil {
			continue
		}
	}

	// Enrich types with interface relationships
	collector.enrichWithInterfaces(ctx, client)

	// Collect file info (line counts)
	var fileInfos []output.FileInfo
	for _, file := range files {
		fullPath := filepath.Join(pkg.Dir, file)
		lineCount := countLines(fullPath)
		fileInfos = append(fileInfos, output.FileInfo{
			Name:      file,
			LineCount: lineCount,
		})
	}

	// Organize into godoc order
	result := collector.build(pkg.ImportPath, pkg.Name, pkg.Dir)
	result.Files = fileInfos

	// Add imports with locations
	for _, imp := range pkg.Imports {
		location := findImportLocation(pkg.Dir, pkg.GoFiles, imp)
		result.Imports = append(result.Imports, output.DepResult{
			Package:  imp,
			Location: location,
		})
	}

	// Add imported_by with locations
	importedBy, err := findImportedBy(c.workDir, pkg.ImportPath)
	if err == nil {
		result.ImportedBy = importedBy
	}

	// Set query info
	result.Query = output.QueryInfo{
		Command: "package",
		Target:  c.pkgPath,
	}

	// Update summary
	result.Summary.Imports = len(result.Imports)
	result.Summary.ImportedBy = len(result.ImportedBy)

	return result, nil
}

// packageCollector collects and organizes symbols from a package.
type packageCollector struct {
	dir       string
	files     map[string]*parsedFile // file path -> parsed AST
	constants []output.PackageSymbol
	variables []output.PackageSymbol
	functions []output.PackageSymbol
	types     map[string]*typeInfo // type name -> info
	typeOrder []string             // preserve order
}

// parsedFile holds a parsed Go source file.
type parsedFile struct {
	fset *token.FileSet
	file *ast.File
}

type typeInfo struct {
	signature     string
	location      string
	functions     []output.PackageSymbol
	methods       []output.PackageSymbol
	isInterface   bool
	file          string    // full path for LSP queries
	selRange      lsp.Range // selection range for LSP queries (points to type name)
	satisfies     []string  // interfaces this type implements
	implementedBy []string  // types implementing this interface
}

func newPackageCollector(dir string) *packageCollector {
	return &packageCollector{
		dir:   dir,
		files: make(map[string]*parsedFile),
		types: make(map[string]*typeInfo),
	}
}

func (c *packageCollector) addFile(path string, symbols []lsp.DocumentSymbol) error {
	// Parse the file using AST
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err == nil {
		c.files[path] = &parsedFile{fset: fset, file: file}
	}

	// Process each symbol
	for _, sym := range symbols {
		c.processSymbol(path, sym)
	}

	return nil
}

func (c *packageCollector) processSymbol(path string, sym lsp.DocumentSymbol) {
	startLine := sym.Range.Start.Line + 1 // Convert to 1-indexed
	endLine := sym.Range.End.Line + 1

	location := formatLocation(path, startLine, endLine)

	// Get signature from AST
	signature := ""
	if pf := c.files[path]; pf != nil {
		signature = c.extractFromAST(pf, sym)
	}
	if signature == "" {
		signature = sym.Name // Fallback to symbol name
	}

	switch sym.Kind {
	case lsp.SymbolKindConstant:
		c.constants = append(c.constants, output.PackageSymbol{
			Signature: signature,
			Location:  location,
		})

	case lsp.SymbolKindVariable:
		c.variables = append(c.variables, output.PackageSymbol{
			Signature: signature,
			Location:  location,
		})

	case lsp.SymbolKindFunction:
		// Check if it's a constructor
		typeName := detectConstructor(signature)
		if typeName != "" {
			c.ensureType(typeName)
			c.types[typeName].functions = append(c.types[typeName].functions, output.PackageSymbol{
				Signature: signature,
				Location:  location,
			})
		} else {
			c.functions = append(c.functions, output.PackageSymbol{
				Signature: signature,
				Location:  location,
			})
		}

	case lsp.SymbolKindStruct, lsp.SymbolKindClass, lsp.SymbolKindInterface:
		typeName := sym.Name
		c.ensureType(typeName)
		c.types[typeName].location = location
		c.types[typeName].signature = signature
		c.types[typeName].isInterface = sym.Kind == lsp.SymbolKindInterface
		c.types[typeName].file = path
		c.types[typeName].selRange = sym.SelectionRange // Points to type name for LSP queries

	case lsp.SymbolKindMethod:
		// Parse receiver from method name like "(*Query).String"
		typeName, _ := parseMethodReceiver(sym.Name)
		if typeName != "" {
			c.ensureType(typeName)
			c.types[typeName].methods = append(c.types[typeName].methods, output.PackageSymbol{
				Signature: signature,
				Location:  location,
			})
		}
	}
}

// extractFromAST finds a declaration in the parsed AST by line number and renders it cleanly.
func (c *packageCollector) extractFromAST(pf *parsedFile, sym lsp.DocumentSymbol) string {
	targetLine := sym.Range.Start.Line + 1 // Convert to 1-indexed

	for _, decl := range pf.file.Decls {
		declLine := pf.fset.Position(decl.Pos()).Line

		switch d := decl.(type) {
		case *ast.FuncDecl:
			if declLine == targetLine || pf.fset.Position(d.Name.Pos()).Line == targetLine {
				return renderFuncDecl(d)
			}

		case *ast.GenDecl:
			// For GenDecl, check each spec
			for _, spec := range d.Specs {
				specLine := pf.fset.Position(spec.Pos()).Line

				switch s := spec.(type) {
				case *ast.TypeSpec:
					if specLine == targetLine && s.Name.Name == sym.Name {
						return renderTypeSpec(d.Tok, s)
					}
				case *ast.ValueSpec:
					if specLine == targetLine {
						for _, name := range s.Names {
							if name.Name == sym.Name {
								return renderValueSpec(d.Tok, s)
							}
						}
					}
				}
			}
		}
	}

	return ""
}

// renderFuncDecl renders a function declaration without its body.
func renderFuncDecl(decl *ast.FuncDecl) string {
	cleaned := *decl
	cleaned.Doc = nil
	cleaned.Body = nil

	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), &cleaned); err != nil {
		return ""
	}
	return buf.String()
}

// renderTypeSpec renders a type specification.
func renderTypeSpec(tok token.Token, spec *ast.TypeSpec) string {
	spec.Doc = nil
	spec.Comment = nil

	// Strip comments from struct fields and interface methods
	switch t := spec.Type.(type) {
	case *ast.StructType:
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				field.Doc = nil
				field.Comment = nil
			}
		}
	case *ast.InterfaceType:
		if t.Methods != nil {
			for _, method := range t.Methods.List {
				method.Doc = nil
				method.Comment = nil
			}
		}
	}

	decl := &ast.GenDecl{
		Tok:   tok,
		Specs: []ast.Spec{spec},
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), decl); err != nil {
		return ""
	}
	return buf.String()
}

// renderValueSpec renders a const or var specification.
// For constants with multiline values, truncates to first line.
func renderValueSpec(tok token.Token, spec *ast.ValueSpec) string {
	spec.Doc = nil
	spec.Comment = nil

	decl := &ast.GenDecl{
		Tok:   tok,
		Specs: []ast.Spec{spec},
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), decl); err != nil {
		return ""
	}

	result := buf.String()

	// Truncate multiline constants (but not vars which may be struct literals)
	if tok == token.CONST && strings.Contains(result, "\n") {
		firstLine := strings.SplitN(result, "\n", 2)[0]
		return firstLine + "..."
	}

	return result
}

func (c *packageCollector) ensureType(name string) {
	if _, ok := c.types[name]; !ok {
		c.types[name] = &typeInfo{}
		c.typeOrder = append(c.typeOrder, name)
	}
}

// enrichWithInterfaces queries LSP for interface relationships on each type.
func (c *packageCollector) enrichWithInterfaces(ctx context.Context, client *lsp.Client) {
	// Get direct deps for filtering indirect dependencies
	workDir, _ := os.Getwd()
	directDeps := golang.DirectDeps(workDir)

	for typeName, info := range c.types {
		if info.file == "" {
			continue // Type not defined in this package
		}

		uri := lsp.FileURI(info.file)
		pos := info.selRange.Start // Use selection range start (points to type name)

		if info.isInterface {
			// For interfaces: find implementations
			impls, err := client.Implementation(ctx, uri, pos)
			if err == nil {
				for _, impl := range impls {
					implFile := lsp.URIToPath(impl.URI)

					// Filter indirect dependencies
					if !golang.IsDirectDep(implFile, directDeps) {
						continue
					}

					// Extract type name from the implementation location
					implName := extractTypeNameAtLocation(implFile, impl.Range.Start.Line)
					if implName != "" {
						info.implementedBy = append(info.implementedBy, implName)
					}
				}
			}
		} else if len(info.methods) > 0 {
			// For types with methods: find interfaces they satisfy
			items, err := client.PrepareTypeHierarchy(ctx, uri, pos)
			if err == nil && len(items) > 0 {
				supertypes, err := client.Supertypes(ctx, items[0])
				if err == nil {
					seen := make(map[string]string) // key -> shortest name
					for _, st := range supertypes {
						stFile := lsp.URIToPath(st.URI)

						// Filter indirect dependencies
						if !golang.IsDirectDep(stFile, directDeps) {
							continue
						}

						// Skip unexported interfaces (not useful to show)
						// Exception: "error" is the builtin error interface
						if len(st.Name) == 0 {
							continue
						}
						if st.Name[0] < 'A' || st.Name[0] > 'Z' {
							if st.Name != "error" {
								continue
							}
						}
						// Build qualified name from URI
						name := qualifiedInterfaceName(st.URI, st.Name)
						// Skip versioned/experimental packages (e.g., json@v0.0.0-...)
						if strings.Contains(name, "@") {
							continue
						}
						// Dedup by interface name, keeping shorter path (prefer stdlib)
						key := strings.ToLower(st.Name)
						if existing, ok := seen[key]; !ok || len(name) < len(existing) {
							seen[key] = name
						}
					}
					// Collect deduplicated results (sorted for deterministic output)
					for _, name := range seen {
						info.satisfies = append(info.satisfies, name)
					}
					sort.Strings(info.satisfies)
				}
			}
		}

		c.types[typeName] = info
	}
}

func (c *packageCollector) build(importPath, name, dir string) *PackageCommandResponse {
	// Sort types alphabetically (godoc order)
	sort.Strings(c.typeOrder)

	var types []output.PackageType
	var methodCount int
	for _, typeName := range c.typeOrder {
		info := c.types[typeName]
		if info.signature == "" {
			// Type was referenced (constructor/method) but not defined in this package
			continue
		}
		types = append(types, output.PackageType{
			Signature:     info.signature,
			Location:      info.location,
			Functions:     info.functions,
			Methods:       info.methods,
			Satisfies:     info.satisfies,
			ImplementedBy: info.implementedBy,
		})
		methodCount += len(info.methods)
	}

	return &PackageCommandResponse{
		Package: output.PackageInfo{
			ImportPath: importPath,
			Name:       name,
			Dir:        dir,
		},
		Constants:  c.constants,
		Variables:  c.variables,
		Functions:  c.functions,
		Types:      types,
		Imports:    []output.DepResult{},
		ImportedBy: []output.DepResult{},
		Summary: output.PackageSummary{
			Constants: len(c.constants),
			Variables: len(c.variables),
			Functions: len(c.functions),
			Types:     len(types),
			Methods:   methodCount,
		},
	}
}

// formatLocation returns file:line or file:line:line_end format.
// Uses just the filename since package dir is already in the output.
func formatLocation(path string, start, end int) string {
	filename := filepath.Base(path)
	if start == end {
		return fmt.Sprintf("%s:%d", filename, start)
	}
	return fmt.Sprintf("%s:%d:%d", filename, start, end)
}

// detectConstructor checks if a function returns a type defined in this package.
// Returns the type name if found, empty string otherwise.
// Any function returning T or *T is grouped under type T.
func detectConstructor(signature string) string {
	// Wrap in package and parse
	wrapped := "package p\n" + signature + " {}"
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", wrapped, 0)
	if err != nil {
		return ""
	}

	for _, decl := range f.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Type.Results == nil {
			continue
		}

		// Check the first return type
		for _, result := range funcDecl.Type.Results.List {
			typeName := extractTypeName(result.Type)
			if typeName != "" && len(typeName) > 0 && typeName[0] >= 'A' && typeName[0] <= 'Z' {
				return typeName
			}
		}
	}

	return ""
}

// extractTypeName gets the base type name from an ast expression.
func extractTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return extractTypeName(t.X)
	case *ast.SelectorExpr:
		// pkg.Type - return just the type name
		return t.Sel.Name
	}
	return ""
}

// parseMethodReceiver extracts the type name from a method name like "(*Query).String".
func parseMethodReceiver(name string) (typeName, methodName string) {
	// Handle (*Type).Method or (Type).Method or Type.Method
	if idx := strings.LastIndex(name, "."); idx != -1 {
		receiver := name[:idx]
		methodName = name[idx+1:]

		// Remove parentheses and pointer
		receiver = strings.TrimPrefix(receiver, "(")
		receiver = strings.TrimSuffix(receiver, ")")
		receiver = strings.TrimPrefix(receiver, "*")

		return receiver, methodName
	}
	return "", name
}

// renderPackageMarkdown renders the package response as compressed markdown.
func renderPackageMarkdown(r *PackageCommandResponse) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# package ")
	sb.WriteString(r.Package.ImportPath)
	sb.WriteString("\n# dir ")
	sb.WriteString(r.Package.Dir)
	sb.WriteString("\n")

	// Files - calculate total lines first
	totalLines := 0
	for _, f := range r.Files {
		totalLines += f.LineCount
	}
	sb.WriteString(fmt.Sprintf("\n# Files (%d lines)\n", totalLines))
	for _, f := range r.Files {
		sb.WriteString(fmt.Sprintf("%s // %d lines\n", f.Name, f.LineCount))
	}

	// Constants
	sb.WriteString(fmt.Sprintf("\n# Constants (%d)\n", len(r.Constants)))
	for _, c := range r.Constants {
		writeSymbolMd(&sb, c.Signature, c.Location)
	}

	// Variables
	sb.WriteString(fmt.Sprintf("\n# Variables (%d)\n", len(r.Variables)))
	for _, v := range r.Variables {
		writeSymbolMd(&sb, v.Signature, v.Location)
	}

	// Functions (standalone, not constructors)
	sb.WriteString(fmt.Sprintf("\n# Functions (%d)\n", len(r.Functions)))
	for _, f := range r.Functions {
		writeSymbolMd(&sb, f.Signature, f.Location)
	}

	// Types
	sb.WriteString(fmt.Sprintf("\n# Types (%d)\n", len(r.Types)))
	for _, t := range r.Types {
		sb.WriteString("\n")
		// Build location with method count and interface info
		loc := t.Location
		if len(t.Methods) > 0 {
			loc += fmt.Sprintf(" // %d methods", len(t.Methods))
		}
		if len(t.Satisfies) > 0 {
			loc += ", satisfies: " + strings.Join(t.Satisfies, ", ")
		}
		if len(t.ImplementedBy) > 0 {
			loc += ", implemented by: " + strings.Join(t.ImplementedBy, ", ")
		}
		writeSymbolMd(&sb, t.Signature, loc)

		// Constructor functions
		for _, f := range t.Functions {
			writeSymbolMd(&sb, f.Signature, f.Location)
		}

		// Methods
		for _, m := range t.Methods {
			writeSymbolMd(&sb, m.Signature, m.Location)
		}
	}

	// Imports grouped by file
	sb.WriteString(fmt.Sprintf("\n# Imports (%d)\n", len(r.Imports)))
	if len(r.Imports) > 0 {
		// Group by file and track line ranges
		type fileImports struct {
			packages []string
			minLine  int
			maxLine  int
		}
		byFile := make(map[string]*fileImports)
		var fileOrder []string

		for _, imp := range r.Imports {
			if imp.Location == "" {
				continue
			}
			// Parse file:line from location
			file, line := parseFileLineFromLocation(imp.Location)
			if file == "" {
				continue
			}

			if fi, ok := byFile[file]; ok {
				fi.packages = append(fi.packages, imp.Package)
				if line < fi.minLine {
					fi.minLine = line
				}
				if line > fi.maxLine {
					fi.maxLine = line
				}
			} else {
				byFile[file] = &fileImports{
					packages: []string{imp.Package},
					minLine:  line,
					maxLine:  line,
				}
				fileOrder = append(fileOrder, file)
			}
		}

		// Output grouped by file
		for _, file := range fileOrder {
			fi := byFile[file]
			sb.WriteString(fmt.Sprintf("# %s:%d:%d\n", file, fi.minLine, fi.maxLine))
			for _, pkg := range fi.packages {
				sb.WriteString(pkg)
				sb.WriteString("\n")
			}
		}
	}

	// Imported By
	sb.WriteString(fmt.Sprintf("\n# Imported By (%d)\n", len(r.ImportedBy)))
	for _, imp := range r.ImportedBy {
		sb.WriteString(imp.Package)
		if imp.Location != "" {
			sb.WriteString(" // ")
			sb.WriteString(imp.Location)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// writeSymbolMd writes a symbol with its location as a trailing comment.
func writeSymbolMd(sb *strings.Builder, signature, location string) {
	// Handle multiline signatures
	if strings.Contains(signature, "\n") {
		sb.WriteString(signature)
		sb.WriteString(" // ")
		sb.WriteString(location)
		sb.WriteString("\n")
	} else {
		sb.WriteString(signature)
		sb.WriteString(" // ")
		sb.WriteString(location)
		sb.WriteString("\n")
	}
}

// qualifiedInterfaceName builds a package-qualified interface name from a URI.
// e.g., "file:///home/.../go/src/fmt/print.go" + "Stringer" -> "fmt.Stringer"
func qualifiedInterfaceName(uri, name string) string {
	path := lsp.URIToPath(uri)
	dir := filepath.Dir(path)
	pkg := filepath.Base(dir)

	// For stdlib, just use the package name
	// For module packages, we could use the full import path but that's verbose
	if pkg != "" && pkg != "." {
		return pkg + "." + name
	}
	return name
}

// extractTypeNameAtLocation parses a file and extracts the type name at the given line.
func extractTypeNameAtLocation(file string, line int) string {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		return ""
	}

	// LSP line is 0-indexed, go/token is 1-indexed
	targetLine := line + 1

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if fset.Position(ts.Name.Pos()).Line == targetLine {
						return ts.Name.Name
					}
				}
			}
		case *ast.FuncDecl:
			// Implementation might point to a method - extract receiver type
			if d.Recv != nil && len(d.Recv.List) > 0 {
				if fset.Position(d.Name.Pos()).Line == targetLine {
					return extractTypeName(d.Recv.List[0].Type)
				}
			}
		}
	}
	return ""
}

// findImportedBy finds all packages in the module that import the target.
func findImportedBy(workDir, targetImportPath string) ([]output.DepResult, error) {
	// List all packages in the module
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = workDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var results []output.DepResult
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	for decoder.More() {
		var pkg goListPackage
		if err := decoder.Decode(&pkg); err != nil {
			continue
		}

		// Check if this package imports our target
		for _, imp := range pkg.Imports {
			if imp == targetImportPath {
				location := findImportLocation(pkg.Dir, pkg.GoFiles, targetImportPath)
				results = append(results, output.DepResult{
					Package:  pkg.ImportPath,
					Location: location,
				})
				break
			}
		}
	}

	return results, nil
}

// findImportLocation finds where a package is imported in source files.
// Returns file:line format or empty string if not found.
func findImportLocation(dir string, files []string, importPath string) string {
	for _, file := range files {
		fullPath := filepath.Join(dir, file)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			// Simple heuristic: look for the import path in quotes
			if strings.Contains(line, `"`+importPath+`"`) {
				return fmt.Sprintf("%s:%d", fullPath, i+1)
			}
		}
	}
	return ""
}

// countLines counts total lines in a file.
func countLines(path string) int {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return bytes.Count(content, []byte("\n")) + 1
}

// parseFileLineFromLocation extracts file path and line number from "path:line" format.
func parseFileLineFromLocation(loc string) (string, int) {
	if loc == "" {
		return "", 0
	}
	idx := strings.LastIndex(loc, ":")
	if idx < 0 {
		return loc, 0
	}
	file := loc[:idx]
	var line int
	fmt.Sscanf(loc[idx+1:], "%d", &line)
	return file, line
}

func (c *PackageCommand) Help() commands.Help {
	return commands.Help{
		Use:   "package [path]",
		Short: "Show package profile with symbols in godoc order",
		Long: `Show a dense package map for AI orientation.

Provides a complete package profile with all symbols organized in godoc order:
constants, variables, functions, then types (each with constructors and methods).

Examples:
  wildcat package                    # Current package
  wildcat package ./internal/lsp     # Specific package
  wildcat package --exclude-stdlib   # Exclude stdlib from imports`,
	}
}

func (c *PackageCommand) README() string {
	return "TODO"
}
