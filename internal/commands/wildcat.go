package commands

import (
	"context"
	"fmt"

	"github.com/jasonmoo/wildcat/internal/golang"
	"golang.org/x/tools/go/packages"
)

type Wildcat struct {
	Project *golang.Project
	Stdlib  []*packages.Package
	Index   *golang.SymbolIndex
}

func LoadWildcat(ctx context.Context, srcDir string) (*Wildcat, error) {
	p, err := golang.LoadModulePackages(ctx, srcDir)
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

func (wc *Wildcat) FindPackage(ctx context.Context, pi *golang.PackageIdentifier) (*golang.Package, error) {
	for _, p := range wc.Project.Packages {
		if pi.PkgPath == p.Identifier.PkgPath {
			return p, nil
		}
	}
	return nil, fmt.Errorf("unable to find package for %q in project", pi.PkgPath)
}
