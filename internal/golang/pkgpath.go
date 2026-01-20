package golang

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

type Project struct {
	Module   *packages.Module
	Packages []*packages.Package
}

// var ProjectModule = func() Project {
// 	p, err := LoadModulePackages(context.Background(), ".")
// 	if err != nil {
// 		panic(err)
// 	}
// 	return Project{
// 		Module:   m,
// 		Packages: ps,
// 	}
// }()

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
		panic(err)
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
}

// ResolvePackagePath attempts to resolve a user-provided package path to its
// full module-qualified import path. It handles:
//   - Full import paths: github.com/user/repo/pkg -> as-is
//   - Relative paths: ./internal/lsp -> resolved to full import path
//   - Bare paths: internal/lsp -> tries ./internal/lsp if exists locally
//   - Short names: fmt, errors -> stdlib
//
// Returns an error for:
//   - Empty paths
//   - Wildcard patterns (containing "...")
//   - Reserved Go patterns (all, cmd, main, std, tool)
//
// TODO: path <> srcDir swap order
func ResolvePackagePath(path, srcDir string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("cannot resolve package: empty path")
	}
	if strings.Contains(path, "...") {
		return "", fmt.Errorf("cannot resolve package %q: wildcard patterns not supported", path)
	}
	if slices.Contains(reservedPatterns, path) {
		return "", fmt.Errorf("cannot resolve %q: reserved Go pattern %v (expands to multiple packages); use ./%s for local package", path, reservedPatterns, path)
	}

	// Try go list first - it returns the canonical module-qualified import path
	if ps, err := GoListPackages(path, srcDir); err == nil && len(ps) > 0 {
		return ps[0].ImportPath, nil
	}

	// If path doesn't start with "." but exists locally, try with "./" prefix
	// This handles bare paths like "internal/lsp" -> "./internal/lsp"
	if !strings.HasPrefix(path, ".") {
		localPath := filepath.Join(srcDir, path)
		if _, statErr := os.Stat(localPath); statErr == nil {
			if ps, err := GoListPackages("./"+path, srcDir); err == nil && len(ps) > 0 {
				return ps[0].ImportPath, nil
			}
		}
	}

	return "", fmt.Errorf("cannot resolve package %q", path)
}

type GoListPackageResult struct {
	Dir            string   `json:"Dir"`            // directory containing package sources
	ImportPath     string   `json:"ImportPath"`     // import path of package in dir
	ImportComment  string   `json:"ImportComment"`  // path in import comment on package statement
	Name           string   `json:"Name"`           // package name
	Doc            string   `json:"Doc"`            // package documentation string
	Target         string   `json:"Target"`         // install path
	Shlib          string   `json:"Shlib"`          // the shared library that contains this package (only set when -linkshared)
	Goroot         bool     `json:"Goroot"`         // is this package in the Go root?
	Standard       bool     `json:"Standard"`       // is this package part of the standard Go library?
	Stale          bool     `json:"Stale"`          // would 'go install' do anything for this package?
	StaleReason    string   `json:"StaleReason"`    // explanation for Stale==true
	Root           string   `json:"Root"`           // Go root or Go path dir containing this package
	ConflictDir    string   `json:"ConflictDir"`    // this directory shadows Dir in $GOPATH
	BinaryOnly     bool     `json:"BinaryOnly"`     // binary-only package (no longer supported)
	ForTest        string   `json:"ForTest"`        // package is only for use in named test
	Export         string   `json:"Export"`         // file containing export data (when using -export)
	BuildID        string   `json:"BuildID"`        // build ID of the compiled package (when using -export)
	Module         *Module  `json:"Module"`         // info about package's containing module, if any (can be nil)
	Match          []string `json:"Match"`          // command-line patterns matching this package
	DepOnly        bool     `json:"DepOnly"`        // package is only a dependency, not explicitly listed
	DefaultGODEBUG string   `json:"DefaultGODEBUG"` // default GODEBUG setting, for main packages

	// Source files
	GoFiles           []string `json:"GoFiles"`           // .go source files (excluding CgoFiles, TestGoFiles, XTestGoFiles)
	CgoFiles          []string `json:"CgoFiles"`          // .go source files that import "C"
	CompiledGoFiles   []string `json:"CompiledGoFiles"`   // .go files presented to compiler (when using -compiled)
	IgnoredGoFiles    []string `json:"IgnoredGoFiles"`    // .go source files ignored due to build constraints
	IgnoredOtherFiles []string `json:"IgnoredOtherFiles"` // non-.go source files ignored due to build constraints
	CFiles            []string `json:"CFiles"`            // .c source files
	CXXFiles          []string `json:"CXXFiles"`          // .cc, .cxx and .cpp source files
	MFiles            []string `json:"MFiles"`            // .m source files
	HFiles            []string `json:"HFiles"`            // .h, .hh, .hpp and .hxx source files
	FFiles            []string `json:"FFiles"`            // .f, .F, .for and .f90 Fortran source files
	SFiles            []string `json:"SFiles"`            // .s source files
	SwigFiles         []string `json:"SwigFiles"`         // .swig files
	SwigCXXFiles      []string `json:"SwigCXXFiles"`      // .swigcxx files
	SysoFiles         []string `json:"SysoFiles"`         // .syso object files to add to archive
	TestGoFiles       []string `json:"TestGoFiles"`       // _test.go files in package
	XTestGoFiles      []string `json:"XTestGoFiles"`      // _test.go files outside package

	// Embedded files
	EmbedPatterns      []string `json:"EmbedPatterns"`      // //go:embed patterns
	EmbedFiles         []string `json:"EmbedFiles"`         // files matched by EmbedPatterns
	TestEmbedPatterns  []string `json:"TestEmbedPatterns"`  // //go:embed patterns in TestGoFiles
	TestEmbedFiles     []string `json:"TestEmbedFiles"`     // files matched by TestEmbedPatterns
	XTestEmbedPatterns []string `json:"XTestEmbedPatterns"` // //go:embed patterns in XTestGoFiles
	XTestEmbedFiles    []string `json:"XTestEmbedFiles"`    // files matched by XTestEmbedPatterns

	// Cgo directives
	CgoCFLAGS    []string `json:"CgoCFLAGS"`    // cgo: flags for C compiler
	CgoCPPFLAGS  []string `json:"CgoCPPFLAGS"`  // cgo: flags for C preprocessor
	CgoCXXFLAGS  []string `json:"CgoCXXFLAGS"`  // cgo: flags for C++ compiler
	CgoFFLAGS    []string `json:"CgoFFLAGS"`    // cgo: flags for Fortran compiler
	CgoLDFLAGS   []string `json:"CgoLDFLAGS"`   // cgo: flags for linker
	CgoPkgConfig []string `json:"CgoPkgConfig"` // cgo: pkg-config names

	// Dependency information
	Imports      []string          `json:"Imports"`      // import paths used by this package
	ImportMap    map[string]string `json:"ImportMap"`    // map from source import to ImportPath (identity entries omitted)
	Deps         []string          `json:"Deps"`         // all (recursively) imported dependencies
	TestImports  []string          `json:"TestImports"`  // imports from TestGoFiles
	XTestImports []string          `json:"XTestImports"` // imports from XTestGoFiles

	// Error information
	Incomplete bool            `json:"Incomplete"` // this package or a dependency has an error
	Error      *PackageError   `json:"Error"`      // error loading package
	DepsErrors []*PackageError `json:"DepsErrors"` // errors loading dependencies
}

type PackageError struct {
	ImportStack []string `json:"ImportStack"` // shortest path from package named on command line to this one
	Pos         string   `json:"Pos"`         // position of error (if present, file:line:col)
	Err         string   `json:"Err"`         // the error itself
}

type Module struct {
	Path       string       `json:"Path"`       // module path
	Query      string       `json:"Query"`      // version query corresponding to this version
	Version    string       `json:"Version"`    // module version
	Versions   []string     `json:"Versions"`   // available module versions
	Replace    *Module      `json:"Replace"`    // replaced by this module
	Time       *time.Time   `json:"Time"`       // time version was created
	Update     *Module      `json:"Update"`     // available update (with -u)
	Main       bool         `json:"Main"`       // is this the main module?
	Indirect   bool         `json:"Indirect"`   // module is only indirectly needed by main module
	Dir        string       `json:"Dir"`        // directory holding local copy of files, if any
	GoMod      string       `json:"GoMod"`      // path to go.mod file describing module, if any
	GoVersion  string       `json:"GoVersion"`  // go version used in module
	Retracted  []string     `json:"Retracted"`  // retraction information, if any (with -retracted or -u)
	Deprecated string       `json:"Deprecated"` // deprecation message, if any (with -u)
	Error      *ModuleError `json:"Error"`      // error loading module
	Sum        string       `json:"Sum"`        // checksum for path, version (as in go.sum)
	GoModSum   string       `json:"GoModSum"`   // checksum for go.mod (as in go.sum)
	Origin     any          `json:"Origin"`     // provenance of module
	Reuse      bool         `json:"Reuse"`      // reuse of old module info is safe
}

type ModuleError struct {
	Err string `json:"Err"` // the error itself
}

func GoListPackages(srcDir, pattern string, flags ...string) ([]*GoListPackageResult, error) {
	args := append([]string{"list", "-json", pattern}, flags...)
	cmd := exec.Command("go", args...)
	cmd.Dir = srcDir
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var pkgs []*GoListPackageResult
	if err := json.Unmarshal(out, &pkgs); err != nil {
		return nil, err
	}
	return pkgs, nil
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

func LoadStdlibPackages(ctx context.Context) ([]*packages.Package, error) {
	return packages.Load(&packages.Config{
		Context: ctx,
		Mode:    packages.LoadTypes,
		Dir:     GOROOT(),
	}, "std")
}

func LoadModulePackages(ctx context.Context, srcDir string) (*Project, error) {
	// load module first
	ps, err := packages.Load(&packages.Config{
		Context: ctx,
		Mode:    packages.NeedModule,
		Dir:     srcDir,
	}, ".")
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
	return &Project{
		Module:   mp.Module,
		Packages: ps,
	}, nil
}
