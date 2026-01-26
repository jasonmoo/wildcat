package package_cmd

import (
	"context"
	"fmt"
	"go/ast"
	gotypes "go/types"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/jasonmoo/wildcat/internal/output"
	"github.com/spf13/cobra"
)

type PackageCommand struct {
	pkgPaths []string
}

var _ commands.Command[*PackageCommand] = (*PackageCommand)(nil)

func WithPackages(paths []string) func(*PackageCommand) error {
	return func(c *PackageCommand) error {
		c.pkgPaths = paths
		return nil
	}
}

func NewPackageCommand() *PackageCommand {
	return &PackageCommand{}
}

func (c *PackageCommand) Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "package [path...]",
		Short: "Show package profile with symbols in godoc order",
		Long: `Show a dense package map for AI orientation.

Provides a complete package profile with all symbols organized in godoc order:
constants, variables, functions, then types (each with constructors and methods).

Multiple packages can be specified to show profiles for each.

Examples:
  wildcat package                              # Current package
  wildcat package ./internal/lsp               # Specific package
  wildcat package internal/golang internal/lsp # Multiple packages`,
		RunE: func(cmd *cobra.Command, args []string) error {
			pkgPaths := args
			if len(pkgPaths) == 0 {
				pkgPaths = []string{"."}
			}
			return commands.RunCommand(cmd, c, WithPackages(pkgPaths))
		},
	}
}

func (c *PackageCommand) README() string {
	return "TODO"
}

func (c *PackageCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*PackageCommand) error) (commands.Result, error) {

	// Default to current package if none specified
	if len(c.pkgPaths) == 0 {
		c.pkgPaths = []string{"."}
	}

	// Single package - return simple response
	if len(c.pkgPaths) == 1 {
		return c.executeOne(ctx, wc, c.pkgPaths[0])
	}

	// Multiple packages - collect all responses
	var responses []*PackageCommandResponse
	for _, pkgPath := range c.pkgPaths {
		resp, err := c.executeOne(ctx, wc, pkgPath)
		if err != nil {
			return nil, err
		}
		responses = append(responses, resp)
	}

	return &MultiPackageResponse{
		Query: output.QueryInfo{
			Command: "package",
			Target:  strings.Join(c.pkgPaths, ", "),
		},
		Packages: responses,
	}, nil
}

func (c *PackageCommand) executeOne(ctx context.Context, wc *commands.Wildcat, pkgPath string) (*PackageCommandResponse, error) {
	pi, err := wc.Project.ResolvePackageName(ctx, pkgPath)
	if err != nil {
		return nil, fmt.Errorf("package_not_found: failed to resolve %q: %w", pkgPath, err)
	}

	pkg, err := wc.Package(pi)
	if err != nil {
		return nil, fmt.Errorf("package_not_found: %w", err)
	}

	var pkgret struct {
		Files      []output.FileInfo
		Embeds     []EmbedInfo
		Constants  []output.PackageSymbol
		Variables  []output.PackageSymbol
		Functions  []output.PackageSymbol
		Types      []output.PackageType
		Imports    []output.DepResult
		ImportedBy []output.DepResult
	}

	// Track types: map for accumulation, slice for source order
	type typeBuilder struct {
		signature     string
		location      string
		refs          *output.TargetRefs
		methods       []output.PackageSymbol
		functions     []output.PackageSymbol // constructors
		isInterface   bool
		satisfies     []string
		implementedBy []string
	}
	types := make(map[string]*typeBuilder)
	var typeOrder []string

	ensureType := func(name string) *typeBuilder {
		if tb, ok := types[name]; ok {
			return tb
		}
		tb := &typeBuilder{}
		types[name] = tb
		typeOrder = append(typeOrder, name)
		return tb
	}

	// Track per-file stats
	type fileStats struct {
		lineCount  int
		exported   int
		unexported int
		refs       output.TargetRefs // aggregate refs
	}
	fileStatsMap := make(map[string]*fileStats)
	var fileOrder []string

	// Collect imports from pkg.Imports
	for _, fi := range pkg.Imports {
		fileName := filepath.Base(fi.FilePath)
		for _, imp := range fi.Imports {
			pkgret.Imports = append(pkgret.Imports, output.DepResult{
				Package:  imp.Path,
				Location: fmt.Sprintf("%s:%d", fileName, imp.Pos.Line),
			})
		}
	}

	// Collect ImportedBy from all project packages
	for _, p := range wc.Project.Packages {
		for _, fi := range p.Imports {
			for _, imp := range fi.Imports {
				if imp.Path == pi.PkgPath {
					pkgret.ImportedBy = append(pkgret.ImportedBy, output.DepResult{
						Package:  p.Identifier.PkgPath,
						Location: fmt.Sprintf("%s:%d", fi.FilePath, imp.Pos.Line),
					})
				}
			}
		}
	}

	// Collect file stats from pkg.Files
	for _, pf := range pkg.Files {
		fileName := filepath.Base(pf.FilePath)
		fileStatsMap[fileName] = &fileStats{
			lineCount:  pf.LineCount,
			exported:   pf.Exported,
			unexported: pf.Unexported,
		}
		fileOrder = append(fileOrder, fileName)
	}

	// Collect symbols from pkg.Symbols
	for _, sym := range pkg.Symbols {
		switch sym.Object.(type) {
		case *gotypes.Const:
			refs := getSymbolRefs(wc, sym.PkgSymbol())
			pkgret.Constants = append(pkgret.Constants, output.PackageSymbol{
				Signature: sym.Signature(),
				Location:  sym.FileLocation(),
				Refs:      refs,
			})
		case *gotypes.Var:
			refs := getSymbolRefs(wc, sym.PkgSymbol())
			pkgret.Variables = append(pkgret.Variables, output.PackageSymbol{
				Signature: sym.Signature(),
				Location:  sym.FileLocation(),
				Refs:      refs,
			})
		case *gotypes.TypeName:
			tb := ensureType(sym.Name)
			tb.signature = sym.Signature()
			tb.location = sym.FileLocation()
			_, tb.isInterface = sym.Object.Type().Underlying().(*gotypes.Interface)
			tb.refs = getSymbolRefs(wc, sym.PkgSymbol())

			// Add methods
			for _, m := range sym.Methods {
				mRefs := getSymbolRefs(wc, m.PkgTypeSymbol())
				tb.methods = append(tb.methods, output.PackageSymbol{
					Signature: m.Signature(),
					Location:  m.FileLocation(),
					Refs:      mRefs,
				})
			}

			// Add constructors
			for _, c := range sym.Constructors {
				cRefs := getSymbolRefs(wc, c.PkgSymbol())
				tb.functions = append(tb.functions, output.PackageSymbol{
					Signature: c.Signature(),
					Location:  c.FileLocation(),
					Refs:      cRefs,
				})
			}

			// Use precomputed interface relationships
			for _, s := range sym.Satisfies {
				qualified := s.PkgPathSymbol()
				if s.PackageIdentifier.PkgPath == pkg.Identifier.PkgPath {
					qualified = s.Name
				}
				tb.satisfies = append(tb.satisfies, qualified)
			}
			for _, impl := range sym.ImplementedBy {
				qualified := impl.PkgPathSymbol()
				if impl.PackageIdentifier.PkgPath == pkg.Identifier.PkgPath {
					qualified = impl.Name
				}
				tb.implementedBy = append(tb.implementedBy, qualified)
			}
		case *gotypes.Func:
			// Check if this function is a constructor for some type
			isConstructor := false
			if fd, ok := sym.Node.(*ast.FuncDecl); ok {
				if typeName := golang.ConstructorTypeName(fd.Type); typeName != "" {
					if pkg.Package.Types.Scope().Lookup(typeName) != nil {
						isConstructor = true
					}
				}
			}
			if !isConstructor {
				refs := getSymbolRefs(wc, sym.PkgSymbol())
				pkgret.Functions = append(pkgret.Functions, output.PackageSymbol{
					Signature: sym.Signature(),
					Location:  sym.FileLocation(),
					Refs:      refs,
				})
			}
		}
	}

	// Sort types alphabetically
	sort.Strings(typeOrder)
	for _, name := range typeOrder {
		tb := types[name]
		// Sort methods and constructors by signature
		sort.Slice(tb.methods, func(i, j int) bool {
			return tb.methods[i].Signature < tb.methods[j].Signature
		})
		sort.Slice(tb.functions, func(i, j int) bool {
			return tb.functions[i].Signature < tb.functions[j].Signature
		})
		sort.Strings(tb.satisfies)
		sort.Strings(tb.implementedBy)
		pkgret.Types = append(pkgret.Types, output.PackageType{
			Signature:     tb.signature,
			Location:      tb.location,
			Refs:          tb.refs,
			Methods:       tb.methods,
			Functions:     tb.functions,
			Satisfies:     tb.satisfies,
			ImplementedBy: tb.implementedBy,
		})
	}

	// Build Files from tracked stats
	for _, fileName := range fileOrder {
		fs := fileStatsMap[fileName]
		pkgret.Files = append(pkgret.Files, output.FileInfo{
			Name:       fileName,
			LineCount:  fs.lineCount,
			Exported:   fs.exported,
			Unexported: fs.unexported,
			Refs: &output.TargetRefs{
				Internal: fs.refs.Internal,
				External: fs.refs.External,
				Packages: fs.refs.Packages,
			},
		})
	}

	// Count methods for summary
	var methodCount int
	for _, t := range pkgret.Types {
		methodCount += len(t.Methods)
	}

	// Collect embed directives
	for _, ed := range golang.FindEmbedDirectives(pkg) {
		fileCount, rawSize, errCount := calculateEmbedSize(pi.PkgDir, ed.Patterns)
		var errMsg string
		if errCount > 0 {
			errMsg = fmt.Sprintf("%d inaccessible", errCount)
		}
		pkgret.Embeds = append(pkgret.Embeds, EmbedInfo{
			Patterns:  ed.Patterns,
			Variable:  fmt.Sprintf("var %s %s", ed.VarName, ed.VarType),
			Location:  fmt.Sprintf("%s:%d", filepath.Base(ed.Position.Filename), ed.Position.Line),
			FileCount: fileCount,
			TotalSize: formatSize(rawSize),
			rawSize:   rawSize,
			Error:     errMsg,
		})
	}

	// Sort all slices alphabetically
	sort.Slice(pkgret.Constants, func(i, j int) bool {
		return pkgret.Constants[i].Signature < pkgret.Constants[j].Signature
	})
	sort.Slice(pkgret.Variables, func(i, j int) bool {
		return pkgret.Variables[i].Signature < pkgret.Variables[j].Signature
	})
	sort.Slice(pkgret.Functions, func(i, j int) bool {
		return pkgret.Functions[i].Signature < pkgret.Functions[j].Signature
	})
	sort.Slice(pkgret.Imports, func(i, j int) bool {
		return pkgret.Imports[i].Package < pkgret.Imports[j].Package
	})
	sort.Slice(pkgret.ImportedBy, func(i, j int) bool {
		return pkgret.ImportedBy[i].Package < pkgret.ImportedBy[j].Package
	})

	return &PackageCommandResponse{
		Query: output.QueryInfo{
			Command: "package",
			Target:  pkgPath,
		},
		Package: output.PackageInfo{
			ImportPath: pi.PkgPath,
			Name:       pi.PkgShortPath,
			Dir:        pi.PkgDir,
		},
		Summary: output.PackageSummary{
			Constants:  len(pkgret.Constants),
			Variables:  len(pkgret.Variables),
			Functions:  len(pkgret.Functions),
			Types:      len(pkgret.Types),
			Methods:    methodCount,
			Imports:    len(pkgret.Imports),
			ImportedBy: len(pkgret.ImportedBy),
		},
		Files:      pkgret.Files,
		Embeds:     pkgret.Embeds,
		Channels:   c.collectChannels(wc, pkg),
		Constants:  pkgret.Constants,
		Variables:  pkgret.Variables,
		Functions:  pkgret.Functions,
		Types:      pkgret.Types,
		Imports:    pkgret.Imports,
		ImportedBy: pkgret.ImportedBy,
	}, nil
}

// getSymbolRefs looks up a symbol and returns its reference counts.
// symbolKey should be in format: pkg.Name or pkg.Type.Method
func getSymbolRefs(wc *commands.Wildcat, symbolKey string) *output.TargetRefs {
	matches := wc.Index.Lookup(symbolKey)
	if len(matches) == 0 {
		return nil
	}
	if len(matches) > 1 {
		var candidates []string
		for _, m := range matches {
			candidates = append(candidates, m.PkgPathSymbol())
		}
		wc.AddDiagnostic("warning", "", "ambiguous symbol %q matches %v; refs unavailable", symbolKey, candidates)
		return nil
	}
	counts := golang.CountReferences(wc.Project.Packages, matches[0])
	return &output.TargetRefs{
		Internal: counts.Internal,
		External: counts.External,
		Packages: counts.PackageCount(),
	}
}

// calculateEmbedSize calculates the total file count and size for embed patterns.
// Returns (fileCount, totalSize, errorCount) where errorCount indicates files that
// couldn't be accessed due to permission or other errors.
func calculateEmbedSize(pkgDir string, patterns []string) (int, int64, int) {
	var fileCount int
	var totalSize int64
	var errCount int
	seen := make(map[string]bool) // avoid counting same file twice

	for _, pattern := range patterns {
		// Resolve pattern relative to package directory
		fullPattern := filepath.Join(pkgDir, pattern)

		// Use filepath.Glob for simple patterns
		matches, err := filepath.Glob(fullPattern)
		if err != nil {
			errCount++
			continue
		}

		for _, match := range matches {
			// Walk if it's a directory
			info, err := os.Stat(match)
			if err != nil {
				errCount++
				continue
			}

			if info.IsDir() {
				// Walk the directory
				filepath.WalkDir(match, func(path string, d fs.DirEntry, err error) error {
					if err != nil {
						errCount++
						return nil
					}
					if d.IsDir() {
						return nil
					}
					if seen[path] {
						return nil
					}
					seen[path] = true
					if fi, err := d.Info(); err == nil {
						fileCount++
						totalSize += fi.Size()
					} else {
						errCount++
					}
					return nil
				})
			} else {
				if seen[match] {
					continue
				}
				seen[match] = true
				fileCount++
				totalSize += info.Size()
			}
		}
	}

	return fileCount, totalSize, errCount
}

// collectChannels collects channel operations from a package grouped by element type and function.
func (c *PackageCommand) collectChannels(wc *commands.Wildcat, pkg *golang.Package) []ChannelGroup {
	// Collect ops by element type, then by function
	type opInfo struct {
		kind      string
		operation string
		location  string
		funcDecl  *ast.FuncDecl
	}
	typeOps := make(map[string][]opInfo)

	golang.WalkChannelOps([]*golang.Package{pkg}, func(op golang.ChannelOp) bool {
		// Skip test packages
		if strings.HasSuffix(op.Package.Identifier.PkgPath, ".test") {
			return true
		}

		base := filepath.Base(op.File)
		location := fmt.Sprintf("%s:%d", base, op.Line)

		typeOps[op.ElemType] = append(typeOps[op.ElemType], opInfo{
			kind:      string(op.Kind),
			operation: golang.FormatNode(op.Node),
			location:  location,
			funcDecl:  op.EnclosingFunc,
		})
		return true
	})

	// Sort element types
	var elemTypes []string
	for t := range typeOps {
		elemTypes = append(elemTypes, t)
	}
	sort.Strings(elemTypes)

	// Build groups
	var groups []ChannelGroup
	for _, elemType := range elemTypes {
		group := ChannelGroup{ElementType: elemType}

		// Group non-make ops by function
		funcOps := make(map[string][]ChannelOp) // funcKey -> ops
		funcDecls := make(map[string]*ast.FuncDecl)

		for _, op := range typeOps[elemType] {
			channelOp := ChannelOp{
				Kind:      op.kind,
				Operation: op.operation,
				Location:  op.location,
			}

			if op.kind == "make" {
				group.Makes = append(group.Makes, channelOp)
			} else if op.funcDecl != nil {
				// Group by function
				var funcKey string
				if sym := pkg.SymbolByIdent(op.funcDecl.Name); sym != nil {
					funcKey = sym.PkgTypeSymbol()
				} else {
					funcKey = "<lookup-error>"
				}
				funcOps[funcKey] = append(funcOps[funcKey], channelOp)
				funcDecls[funcKey] = op.funcDecl
			} else {
				// Package-level (init or var init) - use special key
				funcOps["<init>"] = append(funcOps["<init>"], channelOp)
			}
		}

		// Build function list sorted by name
		var funcKeys []string
		for k := range funcOps {
			funcKeys = append(funcKeys, k)
		}
		sort.Strings(funcKeys)

		for _, funcKey := range funcKeys {
			ops := funcOps[funcKey]
			fn := ChannelFunc{
				Operations: ops,
			}

			if funcKey == "<init>" {
				fn.Signature = "<package init>"
				fn.Definition = ""
			} else if fd := funcDecls[funcKey]; fd != nil {
				fn.Signature = golang.FormatNode(fd)

				startPos := pkg.Package.Fset.Position(fd.Pos())
				endPos := pkg.Package.Fset.Position(fd.End())
				fn.Definition = fmt.Sprintf("%s:%d:%d", filepath.Base(startPos.Filename), startPos.Line, endPos.Line)

				// Get refs for this function
				fn.Refs = getSymbolRefs(wc, funcKey)
			}

			group.Functions = append(group.Functions, fn)
		}

		groups = append(groups, group)
	}

	return groups
}
