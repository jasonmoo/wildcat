package golang

import (
	"go/ast"
	"go/types"
)

// ResolveCallExpr resolves a call expression to the called function.
// Returns nil if the call target cannot be resolved (e.g., function literal, builtin).
func ResolveCallExpr(info *types.Info, call *ast.CallExpr) *types.Func {
	var obj types.Object

	switch fun := call.Fun.(type) {
	case *ast.Ident:
		// Direct call: foo()
		obj = info.Uses[fun]
	case *ast.SelectorExpr:
		// Method or qualified call: obj.Method() or pkg.Func()
		if sel, ok := info.Selections[fun]; ok {
			// Method call
			obj = sel.Obj()
		} else {
			// Package-qualified call
			obj = info.Uses[fun.Sel]
		}
	}

	if fn, ok := obj.(*types.Func); ok {
		return fn
	}
	return nil
}

// FuncInfo holds information about a resolved function.
type FuncInfo struct {
	Func     *types.Func
	Decl     *ast.FuncDecl
	Pkg      *Package
	Filename string
	Receiver string // empty for functions, type name for methods
}

// QualifiedName returns the qualified name like "pkg.Func" or "pkg.Type.Method"
func (fi *FuncInfo) QualifiedName() string {
	name := fi.Func.Name()
	if fi.Receiver != "" {
		name = fi.Receiver + "." + name
	}
	return fi.Pkg.Identifier.Name + "." + name
}

// FindFuncInfo locates the AST and package for a types.Func within the given packages.
// Returns nil if the function is not found (e.g., external dependency not loaded).
func FindFuncInfo(pkgs []*Package, fn *types.Func) *FuncInfo {
	if fn == nil || fn.Pkg() == nil {
		return nil
	}

	fnPos := fn.Pos()
	pkgPath := fn.Pkg().Path()

	for _, pkg := range pkgs {
		if pkg.Identifier.PkgPath != pkgPath {
			continue
		}

		for _, file := range pkg.Package.Syntax {
			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				if fd.Name.Pos() == fnPos {
					info := &FuncInfo{
						Func:     fn,
						Decl:     fd,
						Pkg:      pkg,
						Filename: pkg.Package.Fset.Position(file.Pos()).Filename,
					}
					// Extract receiver type for methods
					if fd.Recv != nil && len(fd.Recv.List) > 0 {
						info.Receiver = ReceiverTypeName(fd.Recv.List[0].Type)
					}
					return info
				}
			}
		}
	}

	return nil
}

// ReceiverFromFunc extracts the receiver type name from a types.Func if it's a method.
// Returns empty string for regular functions.
func ReceiverFromFunc(fn *types.Func) string {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return ""
	}

	recvType := sig.Recv().Type()
	if ptr, ok := recvType.(*types.Pointer); ok {
		recvType = ptr.Elem()
	}
	if named, ok := recvType.(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}

// Call represents a function call site.
type Call struct {
	Package    *Package      // package containing the call
	Caller     *ast.FuncDecl // function containing the call
	CallerFile string        // file path
	CallExpr   *ast.CallExpr // the call expression
	Called     *types.Func   // resolved callee (nil if unresolvable)
	Line       int           // line number of the call
}

// CallerName returns the qualified name of the calling function.
func (c *Call) CallerName() string {
	name := c.Package.Identifier.Name + "."
	if c.Caller.Recv != nil && len(c.Caller.Recv.List) > 0 {
		name += ReceiverTypeName(c.Caller.Recv.List[0].Type) + "."
	}
	name += c.Caller.Name.Name
	return name
}

// CalledName returns the qualified name of the called function.
func (c *Call) CalledName() string {
	if c.Called == nil || c.Called.Pkg() == nil {
		return ""
	}
	name := c.Called.Pkg().Name() + "."
	if recv := ReceiverFromFunc(c.Called); recv != "" {
		name += recv + "."
	}
	name += c.Called.Name()
	return name
}

// CallVisitor is called for each call found. Return false to stop walking.
type CallVisitor func(call Call) bool

// WalkCalls walks all function calls in the given packages.
// Pass project.Packages for all packages, or a subset for filtering.
func WalkCalls(pkgs []*Package, visitor CallVisitor) {
	for _, pkg := range pkgs {
		if !walkPackageCalls(pkg, visitor) {
			return
		}
	}
}

// walkPackageCalls walks calls in a single package. Returns false if visitor wants to stop.
func walkPackageCalls(pkg *Package, visitor CallVisitor) bool {
	for _, file := range pkg.Package.Syntax {
		filename := pkg.Package.Fset.Position(file.Pos()).Filename

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			if !walkFuncCalls(pkg, fn, filename, visitor) {
				return false
			}
		}
	}
	return true
}

// WalkCallsInFunc walks all function calls within a specific function.
func WalkCallsInFunc(pkg *Package, fn *ast.FuncDecl, visitor CallVisitor) {
	if fn.Body == nil {
		return
	}
	filename := pkg.Package.Fset.Position(fn.Pos()).Filename
	walkFuncCalls(pkg, fn, filename, visitor)
}

// walkFuncCalls walks calls in a function body. Returns false if visitor wants to stop.
func walkFuncCalls(pkg *Package, fn *ast.FuncDecl, filename string, visitor CallVisitor) bool {
	continueWalk := true

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if !continueWalk {
			return false
		}

		callExpr, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		pos := pkg.Package.Fset.Position(callExpr.Pos())
		called := ResolveCallExpr(pkg.Package.TypesInfo, callExpr)

		call := Call{
			Package:    pkg,
			Caller:     fn,
			CallerFile: filename,
			CallExpr:   callExpr,
			Called:     called,
			Line:       pos.Line,
		}

		if !visitor(call) {
			continueWalk = false
			return false
		}
		return true
	})

	return continueWalk
}
