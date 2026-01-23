package package_cmd

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
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

	// handle opts
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, fmt.Errorf("interal_error: failed to apply opt: %w", err)
		}
	}

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

	// Short package name for symbol lookups (last segment of import path)
	pkgShortName := pi.PkgPath
	if lastSlash := strings.LastIndex(pkgShortName, "/"); lastSlash >= 0 {
		pkgShortName = pkgShortName[lastSlash+1:]
	}

	var pkgret struct {
		Files      []output.FileInfo      // √
		Embeds     []EmbedInfo            // √
		Constants  []output.PackageSymbol // √
		Variables  []output.PackageSymbol // √
		Functions  []output.PackageSymbol // √
		Types      []output.PackageType   // √
		Imports    []output.DepResult     // √
		ImportedBy []output.DepResult     // √
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

	addFileSymbol := func(fileName string, name string) {
		fs := fileStatsMap[fileName]
		if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
			fs.exported++
		} else {
			fs.unexported++
		}
	}

	addFileRefs := func(fileName string, refs *output.TargetRefs) {
		if refs == nil {
			return
		}
		fs := fileStatsMap[fileName]
		fs.refs.Internal += refs.Internal
		fs.refs.External += refs.External
		fs.refs.Packages += refs.Packages
	}

	for _, f := range pkg.Package.Syntax {

		fsetFile := pkg.Package.Fset.File(f.Pos())
		fileName := filepath.Base(fsetFile.Name())
		fileStatsMap[fileName] = &fileStats{
			lineCount: fsetFile.LineCount(),
		}
		fileOrder = append(fileOrder, fileName)

		for _, imp := range f.Imports {
			pkgret.Imports = append(pkgret.Imports, output.DepResult{
				Package:  strings.Trim(imp.Path.Value, `"`),
				Location: makeLocation(pkg.Package.Fset, fileName, imp.Pos()),
			})
		}

		for _, d := range f.Decls {

			switch v := d.(type) {

			case *ast.FuncDecl:
				sym := output.PackageSymbol{
					Signature: golang.FormatFuncDecl(v),
					Location:  makeLocation(pkg.Package.Fset, fileName, v.Pos()),
				}
				addFileSymbol(fileName, v.Name.Name)
				if v.Recv != nil && len(v.Recv.List) > 0 {
					// Method - attach to receiver type
					typeName := golang.ReceiverTypeName(v.Recv.List[0].Type)
					// Symbol key: pkg.Type.Method
					symbolKey := pkgShortName + "." + typeName + "." + v.Name.Name
					sym.Refs = getSymbolRefs(wc, symbolKey)
					addFileRefs(fileName, sym.Refs)
					ensureType(typeName).methods = append(ensureType(typeName).methods, sym)
				} else if typeName := golang.ConstructorTypeName(v.Type); typeName != "" && pkg.Package.Types.Scope().Lookup(typeName) != nil {
					// Constructor - attach to returned type (only if type is defined in this package)
					// Symbol key: pkg.FuncName
					symbolKey := pkgShortName + "." + v.Name.Name
					sym.Refs = getSymbolRefs(wc, symbolKey)
					addFileRefs(fileName, sym.Refs)
					ensureType(typeName).functions = append(ensureType(typeName).functions, sym)
				} else {
					// Regular function
					// Symbol key: pkg.FuncName
					symbolKey := pkgShortName + "." + v.Name.Name
					sym.Refs = getSymbolRefs(wc, symbolKey)
					addFileRefs(fileName, sym.Refs)
					pkgret.Functions = append(pkgret.Functions, sym)
				}

			case *ast.GenDecl:
				for _, spec := range v.Specs {
					switch vv := spec.(type) {
					case *ast.TypeSpec:
						addFileSymbol(fileName, vv.Name.Name)
						tb := ensureType(vv.Name.Name)
						tb.signature = golang.FormatTypeSpec(v.Tok, vv)
						tb.location = makeLocation(pkg.Package.Fset, fileName, vv.Pos())
						_, tb.isInterface = vv.Type.(*ast.InterfaceType)
						// Symbol key: pkg.TypeName
						symbolKey := pkgShortName + "." + vv.Name.Name
						tb.refs = getSymbolRefs(wc, symbolKey)
						addFileRefs(fileName, tb.refs)
					case *ast.ValueSpec:
						// ValueSpec can have multiple names (e.g., var a, b, c int)
						// but signature covers all, so use first name for refs lookup
						for _, ident := range vv.Names {
							addFileSymbol(fileName, ident.Name)
						}
						var refs *output.TargetRefs
						if len(vv.Names) > 0 {
							symbolKey := pkgShortName + "." + vv.Names[0].Name
							refs = getSymbolRefs(wc, symbolKey)
							addFileRefs(fileName, refs)
						}
						sym := output.PackageSymbol{
							Signature: golang.FormatValueSpec(v.Tok, vv),
							Location:  makeLocation(pkg.Package.Fset, fileName, vv.Pos()),
							Refs:      refs,
						}
						switch v.Tok {
						case token.CONST:
							pkgret.Constants = append(pkgret.Constants, sym)
						case token.VAR:
							pkgret.Variables = append(pkgret.Variables, sym)
						default:
							fmt.Println("unknown value spec", sym)
						}
					}
				}
			}

		}
	}

	// Collect all interfaces from project + stdlib
	ifaces := golang.CollectInterfaces(wc.Project, wc.Stdlib)

	// ImplementedBy: for each interface in this package, find types that implement it
	for name, tb := range types {
		if !tb.isInterface {
			continue
		}
		obj := pkg.Package.Types.Scope().Lookup(name)
		if obj == nil {
			continue
		}
		iface, ok := obj.Type().Underlying().(*gotypes.Interface)
		if !ok {
			continue
		}
		implementors := golang.FindImplementors(iface, pkg.Identifier.PkgPath, name, wc.Project.Packages)
		for _, impl := range implementors {
			qualified := impl.QualifiedName()
			if impl.PkgPath() == pkg.Identifier.PkgPath {
				qualified = impl.Name // same package, just use name
			}
			tb.implementedBy = append(tb.implementedBy, qualified)
		}
	}

	// Satisfies: for each concrete type, find interfaces it implements
	for name, tb := range types {
		if tb.isInterface {
			continue
		}
		obj := pkg.Package.Types.Scope().Lookup(name)
		if obj == nil {
			continue
		}
		satisfied := golang.FindSatisfiedInterfaces(obj.Type(), pkg.Identifier.PkgPath, name, ifaces)
		for _, iface := range satisfied {
			qualified := iface.QualifiedName()
			if iface.PkgPath() == pkg.Identifier.PkgPath {
				qualified = iface.Name // same package, just use name
			}
			tb.satisfies = append(tb.satisfies, qualified)
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

	for _, p := range wc.Project.Packages {
		for _, f := range p.Package.Syntax {
			for _, imp := range f.Imports {
				if strings.Trim(imp.Path.Value, `"`) == pi.PkgPath {
					fileName := p.Package.Fset.Position(imp.Pos()).Filename
					pkgret.ImportedBy = append(pkgret.ImportedBy, output.DepResult{
						Package:  p.Identifier.PkgPath,
						Location: makeLocation(p.Package.Fset, fileName, imp.Pos()),
					})
				}
			}
		}
	}

	// Build Files from tracked stats
	for _, fileName := range fileOrder {
		fs := fileStatsMap[fileName]
		fi := output.FileInfo{
			Name:       fileName,
			LineCount:  fs.lineCount,
			Exported:   fs.exported,
			Unexported: fs.unexported,
		}
		if fs.refs.Internal > 0 || fs.refs.External > 0 || fs.refs.Packages > 0 {
			fi.Refs = &output.TargetRefs{
				Internal: fs.refs.Internal,
				External: fs.refs.External,
				Packages: fs.refs.Packages,
			}
		}
		pkgret.Files = append(pkgret.Files, fi)
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

func makeLocation(fset *token.FileSet, fileName string, pos token.Pos) string {
	return fmt.Sprintf("%s:%d", fileName, fset.Position(pos).Line)
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
			candidates = append(candidates, m.Package.Identifier.PkgPath+"."+m.Name)
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

	// Short package name for symbol lookups
	pkgShortName := pkg.Identifier.PkgPath
	if lastSlash := strings.LastIndex(pkgShortName, "/"); lastSlash >= 0 {
		pkgShortName = pkgShortName[lastSlash+1:]
	}

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
				funcKey := op.funcDecl.Name.Name
				if op.funcDecl.Recv != nil && len(op.funcDecl.Recv.List) > 0 {
					typeName := golang.ReceiverTypeName(op.funcDecl.Recv.List[0].Type)
					funcKey = typeName + "." + funcKey
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
				fn.Signature = golang.FormatFuncDecl(fd)

				startPos := pkg.Package.Fset.Position(fd.Pos())
				endPos := pkg.Package.Fset.Position(fd.End())
				fn.Definition = fmt.Sprintf("%s:%d:%d", filepath.Base(startPos.Filename), startPos.Line, endPos.Line)

				// Get refs for this function
				symbolKey := pkgShortName + "." + funcKey
				fn.Refs = getSymbolRefs(wc, symbolKey)
			}

			group.Functions = append(group.Functions, fn)
		}

		groups = append(groups, group)
	}

	return groups
}
