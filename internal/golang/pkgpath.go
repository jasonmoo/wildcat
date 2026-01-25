package golang

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"golang.org/x/tools/go/packages"
)

type Project struct {
	Module   *packages.Module
	Packages []*Package

	resolveCache map[string]resolvedPackage
	resovleMe    sync.Mutex
}

type resolvedPackage struct {
	pi  *PackageIdentifier
	err error
}

type Package struct {
	Identifier *PackageIdentifier
	Package    *packages.Package
	Files      []*PackageFile
	Symbols    []*PackageSymbol
	Imports    []*FileImports
}

type PackageFile struct {
	FilePath   string
	LineCount  int
	Exported   int
	Unexported int
	File       *ast.File
}

type FileImports struct {
	FilePath string
	Imports  []*PackageImport
}

type PackageImport struct {
	Path    string         // import path e.g. "github.com/foo/bar"
	Name    string         // alias if renamed, "" otherwise
	Pos     token.Position // file:line
	Package *Package       // resolved package, nil if external/not loaded
}

func NewProject(m *packages.Module, ps []*Package) *Project {
	return &Project{
		Module:       m,
		Packages:     ps,
		resolveCache: make(map[string]resolvedPackage),
	}
}

// reservedPatterns are Go toolchain patterns that expand to multiple packages.
// These are not actual importable packages - they're special patterns used by
// go list, go build, etc. We reject them to avoid unexpected behavior where
// packages.Load("cmd") would load all cmd/* packages from the Go toolchain.
// See: go help packages
var reservedPatterns = []string{"all", "cmd", "main", "std", "tool"}

// ResolvePackageName is a helpful package resolver
// tries to resolve short packages (ie. internal to the module)
// excludes test packages
func (p *Project) ResolvePackageName(ctx context.Context, name string) (*PackageIdentifier, error) {

	p.resovleMe.Lock()
	rp, exists := p.resolveCache[name]
	p.resovleMe.Unlock()
	if exists {
		if rp.err != nil {
			return nil, rp.err
		}
		return rp.pi, nil
	}

	pi, err := func() (*PackageIdentifier, error) {
		if slices.Contains(reservedPatterns, name) {
			return nil, fmt.Errorf("cannot resolve %q: reserved Go pattern %v (expands to multiple packages); use ./%s for local package", name, reservedPatterns, name)
		}
		// first try to load as given
		pkgs, err := packages.Load(&packages.Config{
			Context: ctx,
			Mode:    packages.NeedName | packages.NeedModule,
			Dir:     p.Module.Dir,
		}, name)
		if err != nil {
			return nil, fmt.Errorf("failed to load package %q: %w", name, err)
		}
		if len(pkgs) == 1 && len(pkgs[0].Errors) == 0 {
			return newPackageIdentifier(pkgs[0]), nil
		}
		for _, pkg := range pkgs {
			for _, e := range pkg.Errors {
				// if is relative and not stdlib, try to load it as module/pkg
				if strings.Contains(e.Msg, "is not in std") {
					return p.ResolvePackageName(ctx, path.Join(p.Module.Path, name))
				}
			}
		}
		return nil, fmt.Errorf("unable to resolve package %q to stdlib or %q", name, p.Module.Dir)
	}()
	p.resovleMe.Lock()
	p.resolveCache[name] = resolvedPackage{
		pi:  pi,
		err: err,
	}
	p.resovleMe.Unlock()
	return pi, err
}

type PackageIdentifier struct {
	Name         string
	PkgShortPath string
	PkgPath      string
	PkgDir       string
	ModulePath   string
	ModuleDir    string
	IsStd        bool
}

func newPackageIdentifier(p *packages.Package) *PackageIdentifier {
	pi := &PackageIdentifier{
		Name:    p.Name,
		PkgPath: p.PkgPath,
		IsStd:   p.Module == nil,
	}
	if p.Module != nil {
		pi.PkgShortPath = strings.TrimPrefix(p.PkgPath, p.Module.Path+"/")
		pi.PkgDir = filepath.Join(p.Module.Dir, pi.PkgShortPath)
		pi.ModulePath = p.Module.Path
		pi.ModuleDir = p.Module.Dir
	}
	return pi
}

func (pi *PackageIdentifier) IsInternal() bool {
	return strings.Contains(pi.PkgPath, "/internal/") ||
		strings.HasPrefix(pi.PkgPath, "internal/") ||
		strings.HasSuffix(pi.PkgPath, "/internal")
}

func LoadStdlibPackages(ctx context.Context, goroot string) ([]*Package, error) {
	pkgs, err := packages.Load(&packages.Config{
		Context: ctx,
		Mode:    packages.LoadAllSyntax,
		Dir:     goroot,
	}, "std")
	if err != nil {
		return nil, err
	}

	result := make([]*Package, len(pkgs))
	for i, pkg := range pkgs {
		ident := newPackageIdentifier(pkg)
		symbols := loadPackageSymbols(pkg)
		setSymbolIdentifiers(symbols, ident)
		result[i] = &Package{
			Identifier: ident,
			Package:    pkg,
			Files:      loadFiles(pkg, symbols),
			Symbols:    symbols,
			// Imports not needed for stdlib
		}
	}
	return result, nil
}

type LoadPackagesOpt func(*packages.Config) error

func LoadModulePackages(ctx context.Context, srcDir string, opt LoadPackagesOpt) (*Project, error) {
	c := &packages.Config{
		Context: ctx,
		Mode:    packages.NeedModule,
		Dir:     srcDir,
	}
	if opt != nil {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	ps, err := packages.Load(c, ".")
	if err != nil {
		return nil, err
	}
	if len(ps) == 0 {
		return nil, fmt.Errorf("no packages found at location %q", srcDir)
	}
	mp := ps[0]
	if len(mp.Errors) > 0 {
		var errs []error
		for _, e := range mp.Errors {
			errs = append(errs, e)
		}
		return nil, fmt.Errorf("errors while trying to load module info in %q: %w", srcDir, errors.Join(errs...))
	}
	if mp.Module == nil {
		return nil, fmt.Errorf("unable to parse module at location %q", srcDir)
	}
	// load packages for entire module
	ps, err = packages.Load(&packages.Config{
		Context: ctx,
		// TODO: pare these flags down
		Mode: packages.LoadAllSyntax,
		Dir:  mp.Module.Dir,
		// ParseFile with comments so we can detect //go:embed directives
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, src, parser.ParseComments)
		},
		// If Tests is set, the loader includes not just the packages
		// matching a particular pattern but also any related test packages,
		// including test-only variants of the package and the test executable.
		//
		// For example, when using the go command, loading "fmt" with Tests=true
		// returns four packages, with IDs "fmt" (the standard package),
		// "fmt [fmt.test]" (the package as compiled for the test),
		// "fmt_test" (the test functions from source files in package fmt_test),
		// and "fmt.test" (the test binary).
		//
		// In build systems with explicit names for tests,
		// setting Tests may have no effect.
		// Tests: true,
		// // Logf is the logger for the config.
		// // If the user provides a logger, debug logging is enabled.
		// // If the GOPACKAGESDEBUG environment variable is set to true,
		// // but the logger is nil, default to log.Printf.
		// Logf func(format string, args ...any)
		// // Fset provides source position information for syntax trees and types.
		// // If Fset is nil, Load will use a new fileset, but preserve Fset's value.
		// Fset *token.FileSet
		// // ParseFile is called to read and parse each file
		// // when preparing a package's type-checked syntax tree.
		// // It must be safe to call ParseFile simultaneously from multiple goroutines.
		// // If ParseFile is nil, the loader will uses parser.ParseFile.
		// //
		// // ParseFile should parse the source from src and use filename only for
		// // recording position information.
		// //
		// // An application may supply a custom implementation of ParseFile
		// // to change the effective file contents or the behavior of the parser,
		// // or to modify the syntax tree. For example, selectively eliminating
		// // unwanted function bodies can significantly accelerate type checking.
		// ParseFile func(fset *token.FileSet, filename string, src []byte) (*ast.File, error)
	}, "./...")

	// First pass: create all packages and build lookup map
	pkgs := make([]*Package, len(ps))
	pkgMap := make(map[string]*Package)
	for i, pkg := range ps {
		pkg.Module = mp.Module
		ident := newPackageIdentifier(pkg)
		symbols := loadPackageSymbols(pkg)
		setSymbolIdentifiers(symbols, ident)
		pkgs[i] = &Package{
			Identifier: ident,
			Package:    pkg,
			Files:      loadFiles(pkg, symbols),
			Symbols:    symbols,
		}
		pkgMap[pkg.PkgPath] = pkgs[i]
	}

	// Second pass: resolve imports
	for _, p := range pkgs {
		p.Imports = loadImports(p.Package, pkgMap)
	}

	return NewProject(mp.Module, pkgs), nil
}

// loadFiles collects file info for all files in a package.
func loadFiles(pkg *packages.Package, ss []*PackageSymbol) []*PackageFile {
	var files []*PackageFile
	for _, f := range pkg.Syntax {
		fsetFile := pkg.Fset.File(f.Pos())
		pf := &PackageFile{
			FilePath:  fsetFile.Name(),
			LineCount: fsetFile.LineCount(),
			File:      f,
		}
		for _, s := range ss {
			if pkg.Fset.Position(s.Object.Pos()).Filename == pf.FilePath {
				if ast.IsExported(s.Name) {
					pf.Exported++
				} else {
					pf.Unexported++
				}
			}
		}
		files = append(files, pf)
	}
	return files
}

// loadImports collects imports from all files in a package, grouped by file.
func loadImports(pkg *packages.Package, pkgMap map[string]*Package) []*FileImports {
	var fileImports []*FileImports

	for _, f := range pkg.Syntax {
		filename := pkg.Fset.Position(f.Pos()).Filename

		var imports []*PackageImport
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			pi := &PackageImport{
				Path:    path,
				Pos:     pkg.Fset.Position(imp.Pos()),
				Package: pkgMap[path], // nil if external
			}
			if imp.Name != nil {
				pi.Name = imp.Name.Name
			}
			imports = append(imports, pi)
		}

		if len(imports) > 0 {
			fileImports = append(fileImports, &FileImports{
				FilePath: filename,
				Imports:  imports,
			})
		}
	}

	return fileImports
}
