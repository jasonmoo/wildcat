package golang

import (
	"fmt"

	"golang.org/x/tools/go/callgraph/rta"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// DeadCodeResult contains the results of dead code analysis.
type DeadCodeResult struct {
	Program   *ssa.Program
	Reachable map[*ssa.Function]bool
	// ReachablePos maps filename:line to reachability for deduplication
	// (multiple SSA functions can represent the same source function in test mode)
	// Key format: "filename:line"
	ReachablePos map[string]bool
	// HasEntryPoints indicates whether main/init entry points were found.
	// When false, exported functions were used as roots instead, and
	// only unexported unreachable symbols should be reported as dead.
	HasEntryPoints bool
}

// AnalyzeDeadCode performs reachability analysis using Rapid Type Analysis (RTA).
//
// Entry points are main and init functions only. If none are found (library),
// exported functions are used as roots instead. In library mode, only unexported
// unreachable symbols should be reported as dead (exported symbols may have
// external callers).
func AnalyzeDeadCode(project *Project) (*DeadCodeResult, error) {
	// Collect packages for SSA conversion
	var pkgs []*packages.Package
	for _, p := range project.Packages {
		pkgs = append(pkgs, p.Package)
	}

	// Convert to SSA form
	// InstantiateGenerics is required for RTA to work correctly with generics.
	// BuildSerially ensures Build() runs in the main goroutine so panics can be recovered.
	prog, ssaPkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics|ssa.BuildSerially)

	// Build SSA with panic recovery.
	// The SSA builder can panic on certain generic patterns (e.g., go-json-experiment/json).
	if err := buildSSA(prog); err != nil {
		return nil, err
	}

	// Find entry points: main and init functions only
	var roots []*ssa.Function
	for _, pkg := range ssaPkgs {
		if pkg == nil {
			continue
		}

		if fn := pkg.Func("main"); fn != nil {
			roots = append(roots, fn)
		}

		if fn := pkg.Func("init"); fn != nil {
			roots = append(roots, fn)
		}
	}

	hasEntryPoints := len(roots) > 0

	// If no entry points (library), use exported functions as roots
	if !hasEntryPoints {
		for _, pkg := range ssaPkgs {
			if pkg == nil {
				continue
			}
			for _, mem := range pkg.Members {
				fn, ok := mem.(*ssa.Function)
				if !ok || fn.Signature.Recv() != nil {
					continue // skip non-functions and methods (methods handled via their types)
				}
				// Check if exported (starts with uppercase)
				if len(fn.Name()) > 0 && fn.Name()[0] >= 'A' && fn.Name()[0] <= 'Z' {
					roots = append(roots, fn)
				}
			}
		}
	}

	if len(roots) == 0 {
		// No roots at all - can't analyze anything
		return &DeadCodeResult{
			Program:        prog,
			Reachable:      make(map[*ssa.Function]bool),
			ReachablePos:   make(map[string]bool),
			HasEntryPoints: false,
		}, nil
	}

	// Run RTA analysis
	rtaResult := rta.Analyze(roots, false)

	// Build reachable set
	reachable := make(map[*ssa.Function]bool)
	reachablePos := make(map[string]bool)

	for fn := range rtaResult.Reachable {
		reachable[fn] = true

		// Track by filename:line for matching with AST symbols
		// (SSA positions point to function name, AST positions point to func keyword)
		if fn.Pos().IsValid() {
			pos := prog.Fset.Position(fn.Pos())
			key := posKey(pos.Filename, pos.Line)
			reachablePos[key] = true
		}
	}

	return &DeadCodeResult{
		Program:        prog,
		Reachable:      reachable,
		ReachablePos:   reachablePos,
		HasEntryPoints: hasEntryPoints,
	}, nil
}

// posKey creates a filename:line key for position matching.
func posKey(filename string, line int) string {
	return fmt.Sprintf("%s:%d", filename, line)
}

// buildSSA builds the SSA program with panic recovery.
func buildSSA(prog *ssa.Program) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf(`deadcode analysis failed due to an upstream bug in golang.org/x/tools/go/ssa.

This occurs with certain generic type patterns (e.g., github.com/go-json-experiment/json).
A fix is expected in Go 1.26. See: https://github.com/golang/go/issues/73871

Panic: %v

We apologize for the inconvenience. This will work automatically once the fix is released.`, r)
		}
	}()

	prog.Build()
	return nil
}

// IsReachable checks if a symbol is reachable from entry points.
// Returns (reachable, analyzed) where analyzed indicates if the symbol
// could actually be checked. When analyzed is false, reachable defaults
// to true (conservative: assume used when uncertain).
func (r *DeadCodeResult) IsReachable(sym *Symbol) (reachable, analyzed bool) {
	if r == nil || r.Program == nil {
		return true, false // assume reachable if no analysis
	}

	// Use the symbol's own package Fset to get position
	pos := sym.Package.Fset.Position(sym.Node.Pos())
	if !pos.IsValid() {
		return true, false // assume reachable if position unknown
	}

	// Check if this filename:line is in the reachable set
	key := posKey(pos.Filename, pos.Line)
	return r.ReachablePos[key], true
}
