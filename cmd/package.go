package cmd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

	return writer.Write(result)
}

// packageCollector collects and organizes symbols from a package.
type packageCollector struct {
	dir       string
	files     map[string][]string // file path -> lines
	constants []output.PackageSymbol
	variables []output.PackageSymbol
	functions []output.PackageSymbol
	types     map[string]*typeInfo // type name -> info
	typeOrder []string             // preserve order
}

type typeInfo struct {
	signature string
	location  string
	functions []output.PackageSymbol
	methods   []output.PackageSymbol
}

func newPackageCollector(dir string) *packageCollector {
	return &packageCollector{
		dir:   dir,
		files: make(map[string][]string),
		types: make(map[string]*typeInfo),
	}
}

func (c *packageCollector) addFile(path string, symbols []lsp.DocumentSymbol) error {
	// Read file lines
	lines, err := readFileLines(path)
	if err != nil {
		return err
	}
	c.files[path] = lines

	// Process each symbol
	for _, sym := range symbols {
		c.processSymbol(path, lines, sym)
	}

	return nil
}

func (c *packageCollector) processSymbol(path string, lines []string, sym lsp.DocumentSymbol) {
	startLine := sym.Range.Start.Line + 1 // Convert to 1-indexed
	endLine := sym.Range.End.Line + 1

	location := formatLocation(path, startLine, endLine)

	// Use AST-based cleaning for all declarations
	signature := cleanDeclaration(lines, sym)
	if signature == "" {
		signature = sym.Name // Fallback
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

func (c *packageCollector) ensureType(name string) {
	if _, ok := c.types[name]; !ok {
		c.types[name] = &typeInfo{}
		c.typeOrder = append(c.typeOrder, name)
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
			Signature:    info.signature,
			Location:     info.location,
			Functions: info.functions,
			Methods:      info.methods,
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

// cleanDeclaration extracts any declaration and cleans it using go/parser and go/printer.
// Removes comments and struct tags, returns canonical Go formatting.
func cleanDeclaration(lines []string, sym lsp.DocumentSymbol) string {
	startLine := sym.Range.Start.Line // 0-indexed
	endLine := sym.Range.End.Line + 1 // exclusive

	if startLine >= len(lines) || endLine > len(lines) {
		return ""
	}

	// For constants/variables, find and parse the enclosing block
	if sym.Kind == lsp.SymbolKindConstant || sym.Kind == lsp.SymbolKindVariable {
		return cleanValueDeclaration(lines, sym)
	}

	// Extract the source lines
	source := strings.Join(lines[startLine:endLine], "\n")

	// Wrap in a package so it parses
	wrapped := "package p\n" + source

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", wrapped, parser.ParseComments)
	if err != nil {
		return ""
	}

	if len(f.Decls) == 0 {
		return ""
	}

	decl := f.Decls[0]

	// Clean the declaration based on type
	switch d := decl.(type) {
	case *ast.GenDecl:
		d.Doc = nil
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				s.Doc = nil
				s.Comment = nil
				stripTypeComments(s.Type)
			case *ast.ValueSpec:
				s.Doc = nil
				s.Comment = nil
			}
		}
	case *ast.FuncDecl:
		d.Doc = nil
		d.Body = nil // Remove function body, keep just signature
	}

	// Print the cleaned declaration
	var buf bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces, Tabwidth: 4}
	if err := cfg.Fprint(&buf, fset, decl); err != nil {
		return ""
	}

	return buf.String()
}

// cleanValueDeclaration handles const and var declarations by finding the enclosing block.
func cleanValueDeclaration(lines []string, sym lsp.DocumentSymbol) string {
	symLine := sym.Range.Start.Line // 0-indexed

	// Find the start of the const/var block (scan backwards)
	blockStart := symLine
	for i := symLine; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "const ") || strings.HasPrefix(line, "const(") ||
			strings.HasPrefix(line, "var ") || strings.HasPrefix(line, "var(") {
			blockStart = i
			break
		}
		// Stop if we hit another declaration type
		if strings.HasPrefix(line, "func ") || strings.HasPrefix(line, "type ") {
			break
		}
	}

	// Find the end of the block (scan forwards for closing paren or single-line end)
	blockEnd := symLine + 1
	if strings.Contains(lines[blockStart], "(") {
		// It's a block, find closing paren
		depth := 0
		for i := blockStart; i < len(lines); i++ {
			for _, ch := range lines[i] {
				if ch == '(' {
					depth++
				} else if ch == ')' {
					depth--
					if depth == 0 {
						blockEnd = i + 1
						break
					}
				}
			}
			if depth == 0 && blockEnd > blockStart {
				break
			}
		}
	}

	// Parse the whole block
	source := strings.Join(lines[blockStart:blockEnd], "\n")
	wrapped := "package p\n" + source

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", wrapped, parser.ParseComments)
	if err != nil {
		return ""
	}

	if len(f.Decls) == 0 {
		return ""
	}

	// Find our specific constant/variable by name
	genDecl, ok := f.Decls[0].(*ast.GenDecl)
	if !ok {
		return ""
	}

	for _, spec := range genDecl.Specs {
		valueSpec, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}

		for _, name := range valueSpec.Names {
			if name.Name == sym.Name {
				// Found it - create a new GenDecl with just this spec
				valueSpec.Doc = nil
				valueSpec.Comment = nil

				newDecl := &ast.GenDecl{
					Tok:   genDecl.Tok,
					Specs: []ast.Spec{valueSpec},
				}

				var buf bytes.Buffer
				cfg := printer.Config{Mode: printer.UseSpaces, Tabwidth: 4}
				if err := cfg.Fprint(&buf, fset, newDecl); err != nil {
					return ""
				}

				return buf.String()
			}
		}
	}

	return ""
}

// stripTypeComments removes comments and tags from type definitions.
func stripTypeComments(expr ast.Expr) {
	switch t := expr.(type) {
	case *ast.StructType:
		if t.Fields != nil {
			for _, field := range t.Fields.List {
				field.Tag = nil
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

// readFileLines reads a file and returns its lines.
func readFileLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
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
