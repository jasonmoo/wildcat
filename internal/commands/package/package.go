package package_cmd

import (
	"context"
	"go/ast"
	"go/token"

	"github.com/jasonmoo/wildcat/internal/commands"
	"github.com/jasonmoo/wildcat/internal/golang"
	"github.com/kr/pretty"
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

func (c *PackageCommand) Execute(ctx context.Context, wc *commands.Wildcat, opts ...func(*PackageCommand) error) (commands.Result, *commands.Error) {

	// handle opts
	for _, o := range opts {
		if err := o(c); err != nil {
			return nil, commands.NewErrorf("opts_error", "failed to apply opt: %w", err)
		}
	}

	pi, err := wc.Project.ResolvePackageName(ctx, c.pkgPath)
	if err != nil {
		// Suggestions: []string, TODO
		return nil, commands.NewErrorf("package_not_found", "failed to resolve package: %w", err)
	}

	// for _, _ := range golang.ProjectModule.Packages {
	// 	// p.Errors
	// 	// p.Syntax[0].Decls
	// 	// p.Types
	// }

	// client, err := lsp.NewClient(ctx, lsp.ServerConfig{
	// 	Command: "gopls",
	// 	Args:    []string{"serve"},
	// 	WorkDir: c.workDir,
	// })
	// if err != nil {
	// 	return nil, commands.NewErrorf("lsp_error", "%w", err)
	// }
	// defer client.Close()

	pkg, err := wc.FindPackage(ctx, pi)
	if err != nil {
		return nil, commands.NewErrorf("find_package_error", "%w", err)
	}

	type symbol struct {
		kind ast.ObjKind
		pos  token.Position
		d    ast.Decl
		sig  string
	}

	var ss []symbol
	for _, f := range pkg.Syntax {
		for _, d := range f.Decls {
			sigs, err := golang.FormatDecl(d)
			if err != nil {
				return nil, commands.NewErrorf("formatting_error", "%w", err)
			}
			for _, sig := range sigs {
				ss = append(ss, symbol{
					// kind:
					pos: pkg.Fset.Position(d.Pos()),
					// d:   d,
					sig: sig,
				})
			}
		}
	}

	pretty.Println(ss)

	return nil, nil

	// // Collect symbols from all Go files
	// collector := newPackageCollector(pi.PkgDir)

	// // Enrich types with interface relationships
	// collector.enrichWithInterfaces(ctx, client)

	// // Collect file info (line counts)
	// var fileInfos []output.FileInfo
	// for _, file := range files {
	// 	fullPath := filepath.Join(pkg.Dir, file)
	// 	lineCount := countLines(fullPath)
	// 	fileInfos = append(fileInfos, output.FileInfo{
	// 		Name:      file,
	// 		LineCount: lineCount,
	// 	})
	// }

	// // Organize into godoc order
	// result := collector.build(pkg.ImportPath, pkg.Name, pkg.Dir)
	// result.Files = fileInfos

	// // Add imports with locations
	// for _, imp := range pkg.Imports {
	// 	location := findImportLocation(pkg.Dir, pkg.GoFiles, imp)
	// 	result.Imports = append(result.Imports, output.DepResult{
	// 		Package:  imp,
	// 		Location: location,
	// 	})
	// }

	// // Add imported_by with locations
	// importedBy, err := findImportedBy(c.workDir, pkg.ImportPath)
	// if err == nil {
	// 	result.ImportedBy = importedBy
	// }

	// // Set query info
	// result.Query = output.QueryInfo{
	// 	Command: "package",
	// 	Target:  c.pkgPath,
	// }

	// // Update summary
	// result.Summary.Imports = len(result.Imports)
	// result.Summary.ImportedBy = len(result.ImportedBy)

	// return result, nil
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
