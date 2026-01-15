package cmd

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
	"time"

	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/lsp"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

var packageCmd = &cobra.Command{
	Use:   "package [path]",
	Short: "Show package profile with symbols in godoc order",
	Long: `Show a dense package map for AI orientation.

Provides a complete package profile with all symbols organized in godoc order:
constants, variables, functions, then types (each with constructors and methods).

Examples:
  wildcat package                    # Current package
  wildcat package ./internal/lsp     # Specific package
  wildcat package --exclude-stdlib   # Exclude stdlib from imports`,
	Args: cobra.MaximumNArgs(1),
	RunE: runPackage,
}

var (
	pkgExcludeStdlib bool
)

func init() {
	rootCmd.AddCommand(packageCmd)
	packageCmd.Flags().BoolVar(&pkgExcludeStdlib, "exclude-stdlib", false, "Exclude standard library from imports")
}

func runPackage(cmd *cobra.Command, args []string) error {
	writer, err := GetWriter(os.Stdout)
	if err != nil {
		return fmt.Errorf("invalid output format: %w", err)
	}

	pkgPath := "."
	if len(args) > 0 {
		pkgPath = args[0]
	}

	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// Resolve package path (handles bare paths like "internal/lsp")
	if pkgPath != "." {
		resolved, err := golang.ResolvePackagePath(pkgPath, workDir)
		if err != nil {
			return writer.WriteError("package_not_found", err.Error(), nil, nil)
		}
		pkgPath = resolved
	}

	// Get package info via go list
	goCmd := exec.Command("go", "list", "-json", pkgPath)
	goCmd.Dir = workDir
	out, err := goCmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return writer.WriteError("go_list_error", fmt.Sprintf("go list failed: %s", string(exitErr.Stderr)), nil, nil)
		}
		return writer.WriteError("go_list_error", err.Error(), nil, nil)
	}

	var pkg goListPackage
	if err := json.Unmarshal(out, &pkg); err != nil {
		return writer.WriteError("parse_error", err.Error(), nil, nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start LSP client
	client, err := lsp.NewClient(ctx, lsp.ServerConfig{
		Command: "gopls",
		Args:    []string{"serve"},
		WorkDir: workDir,
	})
	if err != nil {
		return writer.WriteError("lsp_error", err.Error(), nil, nil)
	}
	defer client.Close()

	if err := client.Initialize(ctx); err != nil {
		return writer.WriteError("lsp_error", err.Error(), nil, nil)
	}

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

	// Organize into godoc order
	result := collector.build(pkg.ImportPath, pkg.Name, pkg.Dir)

	// Add imports (just package names)
	for _, imp := range pkg.Imports {
		if pkgExcludeStdlib && isStdlib(imp) {
			continue
		}
		result.Imports = append(result.Imports, imp)
	}

	// Add imported_by (just package names)
	importedBy, err := findImportedBy(workDir, pkg.ImportPath)
	if err == nil {
		for _, dep := range importedBy {
			result.ImportedBy = append(result.ImportedBy, dep.Package)
		}
	}

	// Set query info
	result.Query = output.QueryInfo{
		Command: "package",
		Target:  pkgPath,
	}

	// Update summary
	result.Summary.Imports = len(result.Imports)
	result.Summary.ImportedBy = len(result.ImportedBy)

	// Default to compressed markdown unless -o flag was explicitly set
	if !cmd.Flags().Changed("output") {
		fmt.Print(renderPackageMarkdown(result))
		return nil
	}

	return writer.Write(result)
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
	file          string       // full path for LSP queries
	selRange      lsp.Range    // selection range for LSP queries (points to type name)
	satisfies     []string     // interfaces this type implements
	implementedBy []string     // types implementing this interface
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

func (c *packageCollector) build(importPath, name, dir string) *output.PackageResponse {
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

	return &output.PackageResponse{
		Package: output.PackageInfo{
			ImportPath: importPath,
			Name:       name,
			Dir:        dir,
		},
		Constants:  c.constants,
		Variables:  c.variables,
		Functions:  c.functions,
		Types:      types,
		Imports:    []string{},
		ImportedBy: []string{},
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
func renderPackageMarkdown(r *output.PackageResponse) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# package ")
	sb.WriteString(r.Package.ImportPath)
	sb.WriteString("\n# dir ")
	sb.WriteString(r.Package.Dir)
	sb.WriteString("\n")

	// Constants
	if len(r.Constants) > 0 {
		sb.WriteString("\n## Constants\n")
		for _, c := range r.Constants {
			writeSymbolMd(&sb, c.Signature, c.Location)
		}
	}

	// Variables
	if len(r.Variables) > 0 {
		sb.WriteString("\n## Variables\n")
		for _, v := range r.Variables {
			writeSymbolMd(&sb, v.Signature, v.Location)
		}
	}

	// Functions (standalone, not constructors)
	if len(r.Functions) > 0 {
		sb.WriteString("\n## Functions\n")
		for _, f := range r.Functions {
			writeSymbolMd(&sb, f.Signature, f.Location)
		}
	}

	// Types
	if len(r.Types) > 0 {
		sb.WriteString("\n## Types\n")
		for _, t := range r.Types {
			sb.WriteString("\n")
			// Build location with interface info
			loc := t.Location
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
			if len(t.Methods) > 0 {
				sb.WriteString(fmt.Sprintf("# Methods (%d)\n", len(t.Methods)))
				for _, m := range t.Methods {
					writeSymbolMd(&sb, m.Signature, m.Location)
				}
			}
		}
	}

	// Imports
	if len(r.Imports) > 0 {
		sb.WriteString("\n## Imports\n")
		for _, imp := range r.Imports {
			sb.WriteString(imp)
			sb.WriteString("\n")
		}
	}

	// Imported By
	if len(r.ImportedBy) > 0 {
		sb.WriteString("\n## Imported By\n")
		for _, imp := range r.ImportedBy {
			sb.WriteString(imp)
			sb.WriteString("\n")
		}
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

