package package_cmd

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	gotypes "go/types"
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
	pkgPath string
}

var _ commands.Command[*PackageCommand] = (*PackageCommand)(nil)

func WithPackage(path string) func(*PackageCommand) error {
	return func(c *PackageCommand) error {
		c.pkgPath = path
		return nil
	}
}

func NewPackageCommand() *PackageCommand {
	return &PackageCommand{}
}

func (c *PackageCommand) Cmd() *cobra.Command {
	return &cobra.Command{
		Use:   "package [path]",
		Short: "Show package profile with symbols in godoc order",
		Long: `Show a dense package map for AI orientation.

Provides a complete package profile with all symbols organized in godoc order:
constants, variables, functions, then types (each with constructors and methods).

Examples:
  wildcat package                    # Current package
  wildcat package ./internal/lsp     # Specific package`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {

			pkgPath := "."
			if len(args) > 0 {
				pkgPath = args[0]
			}

			wc, err := commands.LoadWildcat(cmd.Context(), ".")
			if err != nil {
				return err
			}

			result, err := c.Execute(cmd.Context(), wc, WithPackage(pkgPath))
			if err != nil {
				return err
			}

			// Check if JSON output requested via inherited flag
			if outputFlag := cmd.Flag("output"); outputFlag != nil && outputFlag.Changed && outputFlag.Value.String() == "json" {
				data, err := result.MarshalJSON()
				if err != nil {
					return err
				}
				os.Stdout.Write(data)
				os.Stdout.WriteString("\n")
				return nil
			}

			// Default to markdown
			md, err := result.MarshalMarkdown()
			if err != nil {
				return err
			}
			os.Stdout.Write(md)
			os.Stdout.WriteString("\n")
			return nil
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

	pi, err := wc.Project.ResolvePackageName(ctx, c.pkgPath)
	if err != nil {
		// Suggestions: []string, TODO
		return commands.NewErrorResultf("package_not_found", "failed to resolve package: %w", err), nil
	}

	pkg := wc.Package(pi)

	var pkgret struct {
		Files      []output.FileInfo      // √
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

	for _, f := range pkg.Package.Syntax {

		fsetFile := pkg.Package.Fset.File(f.Pos())
		fileName := filepath.Base(fsetFile.Name())
		pkgret.Files = append(pkgret.Files, output.FileInfo{
			Name:      fileName,
			LineCount: fsetFile.LineCount(),
		})

		for _, imp := range f.Imports {
			pkgret.Imports = append(pkgret.Imports, output.DepResult{
				Package:  strings.Trim(imp.Path.Value, `"`),
				Location: makeLocation(pkg.Package.Fset, fileName, imp.Pos()),
			})
		}

		for _, d := range f.Decls {

			switch v := d.(type) {

			case *ast.FuncDecl:
				sig, err := golang.FormatFuncDecl(v)
				if err != nil {
					return nil, fmt.Errorf("internal_error: %w", err)
				}
				sym := output.PackageSymbol{
					Signature: sig,
					Location:  makeLocation(pkg.Package.Fset, fileName, v.Pos()),
				}
				if v.Recv != nil && len(v.Recv.List) > 0 {
					// Method - attach to receiver type
					typeName := golang.ReceiverTypeName(v.Recv.List[0].Type)
					ensureType(typeName).methods = append(ensureType(typeName).methods, sym)
				} else if typeName := golang.ConstructorTypeName(v.Type); typeName != "" && pkg.Package.Types.Scope().Lookup(typeName) != nil {
					// Constructor - attach to returned type (only if type is defined in this package)
					ensureType(typeName).functions = append(ensureType(typeName).functions, sym)
				} else {
					pkgret.Functions = append(pkgret.Functions, sym)
				}

			case *ast.GenDecl:
				for _, spec := range v.Specs {
					switch vv := spec.(type) {
					case *ast.TypeSpec:
						sig, err := golang.FormatTypeSpec(v.Tok, vv)
						if err != nil {
							return nil, fmt.Errorf("internal_error: %w", err)
						}
						tb := ensureType(vv.Name.Name)
						tb.signature = sig
						tb.location = makeLocation(pkg.Package.Fset, fileName, vv.Pos())
						_, tb.isInterface = vv.Type.(*ast.InterfaceType)
					case *ast.ValueSpec:
						sig, err := golang.FormatValueSpec(v.Tok, vv)
						if err != nil {
							return nil, fmt.Errorf("internal_error: %w", err)
						}
						sym := output.PackageSymbol{
							Signature: sig,
							Location:  makeLocation(pkg.Package.Fset, fileName, vv.Pos()),
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

	// Collect all interfaces to check against: project packages + stdlib
	type ifaceInfo struct {
		pkgPath string
		name    string
		named   *gotypes.Named // the named type (may be generic)
	}
	var ifaces []ifaceInfo
	// From project packages
	for _, p := range wc.Project.Packages {
		for _, iname := range p.Package.Types.Scope().Names() {
			obj := p.Package.Types.Scope().Lookup(iname)
			if obj == nil {
				continue
			}
			// Only consider type declarations, not variables
			if _, ok := obj.(*gotypes.TypeName); !ok {
				continue
			}
			named, ok := obj.Type().(*gotypes.Named)
			if !ok {
				continue
			}
			if _, ok := named.Underlying().(*gotypes.Interface); ok {
				ifaces = append(ifaces, ifaceInfo{p.Identifier.PkgPath, iname, named})
			}
		}
	}
	// From stdlib
	for _, p := range wc.Stdlib {
		for _, iname := range p.Types.Scope().Names() {
			obj := p.Types.Scope().Lookup(iname)
			if obj == nil {
				continue
			}
			// Only consider type declarations, not variables
			if _, ok := obj.(*gotypes.TypeName); !ok {
				continue
			}
			named, ok := obj.Type().(*gotypes.Named)
			if !ok {
				continue
			}
			if _, ok := named.Underlying().(*gotypes.Interface); ok {
				ifaces = append(ifaces, ifaceInfo{p.PkgPath, iname, named})
			}
		}
	}

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
		// Check all types in the project
		for _, p := range wc.Project.Packages {
			for _, tname := range p.Package.Types.Scope().Names() {
				tobj := p.Package.Types.Scope().Lookup(tname)
				if tobj == nil {
					continue
				}
				T := tobj.Type()
				// Check both T and *T
				if gotypes.Implements(T, iface) || gotypes.Implements(gotypes.NewPointer(T), iface) {
					// Don't list the interface itself
					if p.Identifier.PkgPath == pkg.Identifier.PkgPath && tname == name {
						continue
					}
					qualified := p.Identifier.PkgPath + "." + tname
					if p.Identifier.PkgPath == pkg.Identifier.PkgPath {
						qualified = tname // same package, just use name
					}
					tb.implementedBy = append(tb.implementedBy, qualified)
				}
			}
		}
	}

	// Get the builtin error interface from universe scope
	errorIface := gotypes.Universe.Lookup("error").Type().Underlying().(*gotypes.Interface)

	// Satisfies: for each concrete type, find interfaces it implements
	for name, tb := range types {
		if tb.isInterface {
			continue
		}
		obj := pkg.Package.Types.Scope().Lookup(name)
		if obj == nil {
			continue
		}
		T := obj.Type()
		ptrT := gotypes.NewPointer(T)

		// Check builtin error interface
		if gotypes.Implements(T, errorIface) || gotypes.Implements(ptrT, errorIface) {
			tb.satisfies = append(tb.satisfies, "error")
		}

		for _, i := range ifaces {
			// Skip self (type shouldn't satisfy itself)
			if i.pkgPath == pkg.Identifier.PkgPath && i.name == name {
				continue
			}
			iface := i.named.Underlying().(*gotypes.Interface)
			// Skip empty interface
			if iface.NumMethods() == 0 {
				continue
			}

			var implements bool
			if i.named.TypeParams().Len() > 0 {
				// Generic interface - try instantiating with T and *T
				if inst, err := gotypes.Instantiate(nil, i.named, []gotypes.Type{T}, false); err == nil {
					if instIface, ok := inst.Underlying().(*gotypes.Interface); ok {
						implements = gotypes.Implements(T, instIface) || gotypes.Implements(ptrT, instIface)
					}
				}
				if !implements {
					if inst, err := gotypes.Instantiate(nil, i.named, []gotypes.Type{ptrT}, false); err == nil {
						if instIface, ok := inst.Underlying().(*gotypes.Interface); ok {
							implements = gotypes.Implements(T, instIface) || gotypes.Implements(ptrT, instIface)
						}
					}
				}
			} else {
				// Non-generic interface
				implements = gotypes.Implements(T, iface) || gotypes.Implements(ptrT, iface)
			}

			if implements {
				qualified := i.pkgPath + "." + i.name
				if i.pkgPath == pkg.Identifier.PkgPath {
					qualified = i.name
				}
				tb.satisfies = append(tb.satisfies, qualified)
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

	// Count methods for summary
	var methodCount int
	for _, t := range pkgret.Types {
		methodCount += len(t.Methods)
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
			Target:  c.pkgPath,
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
