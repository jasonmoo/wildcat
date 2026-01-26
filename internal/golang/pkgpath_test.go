package golang

import (
	"fmt"
	"testing"
	"time"
)

func TestLoadPackages(t *testing.T) {

	// pi, err := ResolvePackageName(t.Context(), ".", "./...")
	// if err != nil {
	// 	t.Error(err)
	// }

	start := time.Now()
	// ps, err := LoadPackages(t.Context(), "/home/jason/go/src/github.com/jasonmoo/wildcat", "./...")
	// _, err = LoadPackages(t.Context(), pi.ModuleDir, pi.PkgPath)
	p, err := LoadModulePackages(t.Context(), ".", nil)
	if err != nil {
		t.Error(err)
	}
	fmt.Println("done", time.Since(start))

	for _, pkg := range p.Packages {
		fmt.Println(pkg.Identifier.PkgPath, pkg.Package.Errors, pkg.Package.TypeErrors) //slices.Collect(maps.Keys(p.Imports)))
		// pretty.Println(p)
		// pretty.Println(p.Module)
		// // fmt.Printf("%#v", p)
		// pretty.Println(p.Types.Scope().Lookup("Client").String())
	}

}

// func TestNewResolve(t *testing.T) {

// 	p, err := ProjectModule.ResolvePackageName(context.Background(), "internal/lsp")
// 	if err != nil {
// 		t.Error(err)
// 	}
// 	pretty.Println(p)

// }
