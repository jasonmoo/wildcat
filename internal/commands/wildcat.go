package commands

import (
	"context"

	"github.com/jasonmoo/wildcat/internal/golang"
	"golang.org/x/tools/go/packages"
)

type Wildcat struct {
	Project *golang.Project
	Stdlib  []*packages.Package
	Index   *golang.SymbolIndex
}

type WildcatConfig struct {
	IncludeTests bool
}

func LoadWildcat(ctx context.Context, srcDir string) (*Wildcat, error) {
	p, err := golang.LoadModulePackages(ctx, srcDir, nil)
	if err != nil {
		return nil, err
	}
	stdps, err := golang.LoadStdlibPackages(ctx)
	if err != nil {
		return nil, err
	}
	return &Wildcat{
		Project: p,
		Stdlib:  stdps,
		Index:   golang.CollectSymbols(p.Packages),
	}, nil
}

func (wc *Wildcat) Package(pi *golang.PackageIdentifier) *golang.Package {
	for _, p := range wc.Project.Packages {
		if pi.PkgPath == p.Identifier.PkgPath {
			return p
		}
	}
	panic("this should never happen")
}

func (wc *Wildcat) Suggestions(symbol string, opt *golang.SearchOptions) []string {
	results := wc.Index.Search(symbol, opt)
	ret := make([]string, len(results))
	for i, res := range results {
		ret[i] = res.Symbol.Package.Identifier.Name + "." + res.Symbol.Name
	}
	return ret
}

func (wc *Wildcat) NewSymbolNotFoundErrorResponse(symbol string) *ErrorResult {
	e := NewErrorResultf("symbol_not_found", "%q not found", symbol)
	e.Suggestions = wc.Suggestions(symbol, &golang.SearchOptions{Limit: 5})
	return e
}

func (wc *Wildcat) NewFuncNotFoundErrorResponse(symbol string) *ErrorResult {
	e := NewErrorResultf("function_not_found", "%q not found", symbol)
	e.Suggestions = wc.Suggestions(symbol, &golang.SearchOptions{
		Limit: 5,
		Kinds: []golang.SymbolKind{golang.SymbolKindFunc},
	})
	return e
}
