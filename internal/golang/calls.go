package golang

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// ShortPkgName extracts the short package name from a full import path.
// e.g., "github.com/user/repo/internal/lsp" -> "lsp"
func ShortPkgName(pkgPath string) string {
	if idx := strings.LastIndex(pkgPath, "/"); idx >= 0 {
		return pkgPath[idx+1:]
	}
	return pkgPath
}

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
	Pkg      *packages.Package
	Filename string
	Receiver string // empty for functions, type name for methods
}

// QualifiedName returns the qualified name like "pkg.Func" or "pkg.Type.Method"
func (fi *FuncInfo) QualifiedName() string {
	name := fi.Func.Name()
	if fi.Receiver != "" {
		name = fi.Receiver + "." + name
	}
	return ShortPkgName(fi.Func.Pkg().Path()) + "." + name
}

// FindFuncInfo locates the AST and package for a types.Func within the given packages.
// Returns nil if the function is not found (e.g., external dependency not loaded).
func FindFuncInfo(pkgs []*packages.Package, fn *types.Func) *FuncInfo {
	if fn == nil || fn.Pkg() == nil {
		return nil
	}

	fnPos := fn.Pos()
	pkgPath := fn.Pkg().Path()

	for _, pkg := range pkgs {
		if pkg.PkgPath != pkgPath {
			continue
		}

		for _, file := range pkg.Syntax {
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
						Filename: pkg.Fset.Position(file.Pos()).Filename,
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
