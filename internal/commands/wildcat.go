package commands

import (
	"context"
	"slices"
	"strings"

	"github.com/jasonmoo/wildcat/internal/golang"
	"golang.org/x/tools/go/packages"
)

type Wildcat struct {
	Project *golang.Project
	Stdlib  []*packages.Package
	Index   *golang.SymbolIndex
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

func (wc *Wildcat) Suggestions(symbol string, opt *golang.SearchOptions) []Suggestion {
	// Fetch all fuzzy matches (no limit), then apply limit after deduplication
	limit := 5
	if opt != nil && opt.Limit > 0 {
		limit = opt.Limit
	}
	searchOpt := &golang.SearchOptions{
		Limit:   0, // no limit - fuzzy matcher has its own threshold
		Kinds:   nil,
		Exclude: nil,
	}
	if opt != nil {
		searchOpt.Kinds = opt.Kinds
		searchOpt.Exclude = opt.Exclude
	}

	// Search with full query
	results := wc.Index.Search(symbol, searchOpt)

	// If query is package-qualified, also search by just the base name
	// e.g., for "model.Task", also search for "Task" to find "TaskNamespace", etc.
	if lastDot := strings.LastIndex(symbol, "."); lastDot >= 0 {
		baseName := symbol[lastDot+1:]
		if baseName != "" {
			baseResults := wc.Index.Search(baseName, searchOpt)
			// Merge results, keeping best score for each symbol
			resultMap := make(map[string]golang.SearchResult)
			for _, r := range results {
				key := r.Symbol.Package.Identifier.PkgPath + "." + r.Symbol.Name
				if existing, ok := resultMap[key]; !ok || r.Score > existing.Score {
					resultMap[key] = r
				}
			}
			for _, r := range baseResults {
				key := r.Symbol.Package.Identifier.PkgPath + "." + r.Symbol.Name
				if existing, ok := resultMap[key]; !ok || r.Score > existing.Score {
					resultMap[key] = r
				}
			}
			// Rebuild results slice sorted by score descending
			results = make([]golang.SearchResult, 0, len(resultMap))
			for _, r := range resultMap {
				results = append(results, r)
			}
			slices.SortFunc(results, func(a, b golang.SearchResult) int {
				return b.Score - a.Score
			})
		}
	}

	// First pass: identify all types in the results
	typeSet := make(map[string]bool) // "pkg.TypeName" -> true
	for _, res := range results {
		if res.Symbol.Kind == golang.SymbolKindType || res.Symbol.Kind == golang.SymbolKindInterface {
			key := res.Symbol.Package.Identifier.Name + "." + res.Symbol.Name
			typeSet[key] = true
		}
	}

	// Second pass: filter out methods whose receiver type is already in results
	var ret []Suggestion
	for _, res := range results {
		fullName := res.Symbol.Package.Identifier.Name + "." + res.Symbol.Name

		// If it's a method, check if its receiver type is in the results
		if res.Symbol.Kind == golang.SymbolKindMethod {
			// Method names are "ReceiverType.MethodName", extract the type
			if dotIdx := strings.Index(res.Symbol.Name, "."); dotIdx > 0 {
				receiverType := res.Symbol.Name[:dotIdx]
				typeKey := res.Symbol.Package.Identifier.Name + "." + receiverType
				if typeSet[typeKey] {
					continue // skip this method, its type is already suggested
				}
			}
		}

		ret = append(ret, Suggestion{
			Symbol: fullName,
			Kind:   string(res.Symbol.Kind),
		})
		if len(ret) >= limit {
			break
		}
	}

	return ret
}

func (wc *Wildcat) NewSymbolNotFoundErrorResponse(symbol string) *ErrorResult {
	e := NewErrorResultf("symbol_not_found", "%q not found", symbol)
	for _, s := range wc.Suggestions(symbol, &golang.SearchOptions{Limit: 5, Exclude: []string{symbol}}) {
		e.Suggestions = append(e.Suggestions, s.Symbol)
	}
	return e
}

func (wc *Wildcat) NewFuncNotFoundErrorResponse(symbol string) *ErrorResult {
	e := NewErrorResultf("function_not_found", "%q not found", symbol)
	for _, s := range wc.Suggestions(symbol, &golang.SearchOptions{
		Limit:   5,
		Kinds:   []golang.SymbolKind{golang.SymbolKindFunc},
		Exclude: []string{symbol},
	}) {
		e.Suggestions = append(e.Suggestions, s.Symbol)
	}
	return e
}
