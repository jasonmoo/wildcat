package golang

import (
	"fmt"
	"go/types"
	"strings"

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
}

// AnalyzeDeadCode performs reachability analysis using Rapid Type Analysis (RTA).
// It finds all functions reachable from entry points (main, init, and optionally tests).
func AnalyzeDeadCode(project *Project, includeTests bool) (*DeadCodeResult, error) {
	// Collect packages for SSA conversion
	var pkgs []*packages.Package
	for _, p := range project.Packages {
		pkgs = append(pkgs, p.Package)
	}

	// Convert to SSA form
	// InstantiateGenerics is required for RTA to work correctly with generics
	prog, ssaPkgs := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)

	// Build SSA with panic recovery - the SSA builder can panic on certain
	// edge cases involving generics or variadic parameters
	if err := buildSSAWithRecovery(prog); err != nil {
		return nil, err
	}

	// Find root functions (entry points)
	var roots []*ssa.Function
	for _, pkg := range ssaPkgs {
		if pkg == nil {
			continue
		}

		// main function
		if fn := pkg.Func("main"); fn != nil {
			roots = append(roots, fn)
		}

		// init function (synthesized)
		if fn := pkg.Func("init"); fn != nil {
			roots = append(roots, fn)
		}

		// Test/Benchmark/Example/Fuzz functions
		if includeTests {
			for _, mem := range pkg.Members {
				fn, ok := mem.(*ssa.Function)
				if !ok {
					continue
				}
				name := fn.Name()
				if strings.HasPrefix(name, "Test") ||
					strings.HasPrefix(name, "Benchmark") ||
					strings.HasPrefix(name, "Example") ||
					strings.HasPrefix(name, "Fuzz") {
					roots = append(roots, fn)
				}
			}
		}
	}

	if len(roots) == 0 {
		// No entry points found - return empty result
		return &DeadCodeResult{
			Program:      prog,
			Reachable:    make(map[*ssa.Function]bool),
			ReachablePos: make(map[string]bool),
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
		Program:      prog,
		Reachable:    reachable,
		ReachablePos: reachablePos,
	}, nil
}

// posKey creates a filename:line key for position matching.
func posKey(filename string, line int) string {
	return fmt.Sprintf("%s:%d", filename, line)
}

// buildSSAWithRecovery wraps prog.Build() with panic recovery.
// The SSA builder can panic on certain edge cases involving generics or variadic parameters.
func buildSSAWithRecovery(prog *ssa.Program) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("ssa_build_panic: SSA builder panicked: %v", r)
		}
	}()
	prog.Build()
	return nil
}

// IsReachable checks if a symbol is reachable from entry points.
func (r *DeadCodeResult) IsReachable(sym *Symbol) bool {
	if r == nil || r.Program == nil {
		return true // assume reachable if no analysis
	}

	// Use the symbol's own package Fset to get position
	pos := sym.Package.Package.Fset.Position(sym.Node().Pos())
	if !pos.IsValid() {
		return true // assume reachable if position unknown
	}

	// Check if this filename:line is in the reachable set
	key := posKey(pos.Filename, pos.Line)
	return r.ReachablePos[key]
}

// IsTypeReachable checks if a type has any reachable methods or constructors.
func (r *DeadCodeResult) IsTypeReachable(pkg *Package, typeName string) bool {
	if r == nil || r.Program == nil {
		return true
	}

	// Find SSA package
	ssaPkg := r.Program.Package(pkg.Package.Types)
	if ssaPkg == nil {
		return true
	}

	// Check if any method of this type is reachable
	for fn := range r.Reachable {
		if fn.Pkg != ssaPkg {
			continue
		}
		sig := fn.Signature
		if sig.Recv() == nil {
			continue
		}
		// Get receiver type name
		recvType := sig.Recv().Type()
		if ptr, ok := recvType.(*types.Pointer); ok {
			recvType = ptr.Elem()
		}
		if named, ok := recvType.(*types.Named); ok {
			if named.Obj().Name() == typeName {
				return true
			}
		}
	}

	return false
}

// DeadFunctions returns all functions that are not reachable.
// It filters to only include source-level functions (not synthetic wrappers).
func (r *DeadCodeResult) DeadFunctions(project *Project) []*ssa.Function {
	if r == nil || r.Program == nil {
		return nil
	}

	var dead []*ssa.Function

	for _, pkg := range project.Packages {
		ssaPkg := r.Program.Package(pkg.Package.Types)
		if ssaPkg == nil {
			continue
		}

		for _, mem := range ssaPkg.Members {
			fn, ok := mem.(*ssa.Function)
			if !ok {
				continue
			}

			// Skip synthetic functions
			if fn.Synthetic != "" {
				continue
			}

			// Skip if reachable
			if r.Reachable[fn] {
				continue
			}

			// Skip if position is reachable (handles test deduplication)
			if fn.Pos().IsValid() {
				pos := r.Program.Fset.Position(fn.Pos())
				if r.ReachablePos[posKey(pos.Filename, pos.Line)] {
					continue
				}
			}

			dead = append(dead, fn)
		}

		// Also check methods
		for _, mem := range ssaPkg.Members {
			typ, ok := mem.(*ssa.Type)
			if !ok {
				continue
			}

			// Get methods via method set
			mset := r.Program.MethodSets.MethodSet(typ.Type())
			for i := 0; i < mset.Len(); i++ {
				sel := mset.At(i)
				fn := r.Program.MethodValue(sel)
				if fn == nil {
					continue
				}

				// Skip synthetic
				if fn.Synthetic != "" {
					continue
				}

				// Skip if reachable
				if r.Reachable[fn] {
					continue
				}

				// Skip if position is reachable
				if fn.Pos().IsValid() {
					pos := r.Program.Fset.Position(fn.Pos())
					if r.ReachablePos[posKey(pos.Filename, pos.Line)] {
						continue
					}
				}

				dead = append(dead, fn)
			}

			// Also check pointer receiver methods
			ptrMset := r.Program.MethodSets.MethodSet(types.NewPointer(typ.Type()))
			for i := 0; i < ptrMset.Len(); i++ {
				sel := ptrMset.At(i)
				fn := r.Program.MethodValue(sel)
				if fn == nil {
					continue
				}

				if fn.Synthetic != "" {
					continue
				}

				if r.Reachable[fn] {
					continue
				}

				if fn.Pos().IsValid() {
					pos := r.Program.Fset.Position(fn.Pos())
					if r.ReachablePos[posKey(pos.Filename, pos.Line)] {
						continue
					}
				}

				dead = append(dead, fn)
			}
		}
	}

	return dead
}
