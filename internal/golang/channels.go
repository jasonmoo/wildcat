package golang

import (
	"go/ast"
	"go/token"
	"go/types"
)

// ChannelOpKind represents the type of channel operation.
type ChannelOpKind string

const (
	ChannelOpSend          ChannelOpKind = "send"
	ChannelOpReceive       ChannelOpKind = "receive"
	ChannelOpClose         ChannelOpKind = "close"
	ChannelOpMake          ChannelOpKind = "make"
	ChannelOpRange         ChannelOpKind = "range"
	ChannelOpSelectSend    ChannelOpKind = "select_send"
	ChannelOpSelectReceive ChannelOpKind = "select_receive"
)

// ChannelOp represents a channel operation.
type ChannelOp struct {
	Package       *Package       // package containing the operation
	File          string         // file path
	Line          int            // line number
	Kind          ChannelOpKind  // type of operation
	ElemType      string         // channel element type
	Node          ast.Node       // the AST node
	EnclosingFunc *ast.FuncDecl  // enclosing function (nil if at package level)
}

// ChannelOpVisitor is called for each channel operation found. Return false to stop walking.
type ChannelOpVisitor func(op ChannelOp) bool

// WalkChannelOps walks all channel operations in the given packages.
// Pass project.Packages for all packages, or a subset for filtering.
func WalkChannelOps(pkgs []*Package, visitor ChannelOpVisitor) {
	for _, pkg := range pkgs {
		if !walkPackageChannelOps(pkg, visitor) {
			return
		}
	}
}

// walkPackageChannelOps walks channel ops in a single package. Returns false if visitor wants to stop.
func walkPackageChannelOps(pkg *Package, visitor ChannelOpVisitor) bool {
	for _, file := range pkg.Package.Syntax {
		filename := pkg.Package.Fset.Position(file.Pos()).Filename

		if !walkFileChannelOps(pkg, file, filename, visitor) {
			return false
		}
	}
	return true
}

// walkFileChannelOps walks channel ops in a single file. Returns false if visitor wants to stop.
func walkFileChannelOps(pkg *Package, file *ast.File, filename string, visitor ChannelOpVisitor) bool {
	// Track nodes that are part of select cases so we don't double-count
	selectNodes := make(map[ast.Node]bool)
	continueWalk := true

	// Track current enclosing function during walk
	var funcStack []*ast.FuncDecl

	currentFunc := func() *ast.FuncDecl {
		if len(funcStack) > 0 {
			return funcStack[len(funcStack)-1]
		}
		return nil
	}

	emitOp := func(kind ChannelOpKind, elemType string, node ast.Node, pos token.Position) bool {
		op := ChannelOp{
			Package:       pkg,
			File:          filename,
			Line:          pos.Line,
			Kind:          kind,
			ElemType:      elemType,
			Node:          node,
			EnclosingFunc: currentFunc(),
		}
		if !visitor(op) {
			continueWalk = false
			return false
		}
		return true
	}

	// First pass: identify all select statement channel operations
	var inspectSelect func(n ast.Node) bool
	inspectSelect = func(n ast.Node) bool {
		if !continueWalk {
			return false
		}

		// Track function boundaries
		if fn, ok := n.(*ast.FuncDecl); ok {
			if fn.Body == nil {
				return false // external function, no body to inspect
			}
			funcStack = append(funcStack, fn)
			for _, stmt := range fn.Body.List {
				ast.Inspect(stmt, inspectSelect)
			}
			funcStack = funcStack[:len(funcStack)-1]
			return false // don't descend again
		}

		sel, ok := n.(*ast.SelectStmt)
		if !ok {
			return true
		}

		for _, stmt := range sel.Body.List {
			comm, ok := stmt.(*ast.CommClause)
			if !ok || comm.Comm == nil {
				continue
			}

			switch node := comm.Comm.(type) {
			case *ast.SendStmt:
				selectNodes[node] = true
				elemType := ChannelElemType(pkg.Package.TypesInfo, node.Chan)
				pos := pkg.Package.Fset.Position(node.Pos())
				if !emitOp(ChannelOpSelectSend, elemType, node, pos) {
					return false
				}

			case *ast.ExprStmt:
				if recv, ok := node.X.(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
					selectNodes[recv] = true
					elemType := ChannelElemType(pkg.Package.TypesInfo, recv.X)
					pos := pkg.Package.Fset.Position(recv.Pos())
					if !emitOp(ChannelOpSelectReceive, elemType, recv, pos) {
						return false
					}
				}

			case *ast.AssignStmt:
				if len(node.Rhs) == 1 {
					if recv, ok := node.Rhs[0].(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
						selectNodes[recv] = true
						elemType := ChannelElemType(pkg.Package.TypesInfo, recv.X)
						pos := pkg.Package.Fset.Position(recv.Pos())
						if !emitOp(ChannelOpSelectReceive, elemType, recv, pos) {
							return false
						}
					}
				}
			}
		}
		return true
	}

	for _, decl := range file.Decls {
		ast.Inspect(decl, inspectSelect)
		if !continueWalk {
			return false
		}
	}

	// Reset func stack for second pass
	funcStack = nil

	// Second pass: collect non-select channel operations
	var inspectOps func(n ast.Node) bool
	inspectOps = func(n ast.Node) bool {
		if !continueWalk {
			return false
		}

		// Track function boundaries
		if fn, ok := n.(*ast.FuncDecl); ok {
			funcStack = append(funcStack, fn)
			if fn.Body != nil {
				for _, stmt := range fn.Body.List {
					ast.Inspect(stmt, inspectOps)
				}
			}
			funcStack = funcStack[:len(funcStack)-1]
			return false // don't descend again
		}

		switch node := n.(type) {
		case *ast.SendStmt:
			if selectNodes[node] {
				return true
			}
			elemType := ChannelElemType(pkg.Package.TypesInfo, node.Chan)
			pos := pkg.Package.Fset.Position(node.Pos())
			if !emitOp(ChannelOpSend, elemType, node, pos) {
				return false
			}

		case *ast.UnaryExpr:
			if selectNodes[node] {
				return true
			}
			if node.Op == token.ARROW {
				elemType := ChannelElemType(pkg.Package.TypesInfo, node.X)
				pos := pkg.Package.Fset.Position(node.Pos())
				if !emitOp(ChannelOpReceive, elemType, node, pos) {
					return false
				}
			}

		case *ast.CallExpr:
			ident, ok := node.Fun.(*ast.Ident)
			if !ok {
				return true
			}

			switch ident.Name {
			case "close":
				if len(node.Args) > 0 {
					elemType := ChannelElemType(pkg.Package.TypesInfo, node.Args[0])
					pos := pkg.Package.Fset.Position(node.Pos())
					if !emitOp(ChannelOpClose, elemType, node, pos) {
						return false
					}
				}

			case "make":
				if len(node.Args) == 0 {
					return true
				}
				// Try type info first
				if t := pkg.Package.TypesInfo.TypeOf(node); t != nil {
					if ch, ok := t.Underlying().(*types.Chan); ok {
						elemType := ch.Elem().String()
						pos := pkg.Package.Fset.Position(node.Pos())
						if !emitOp(ChannelOpMake, elemType, node, pos) {
							return false
						}
					}
				} else if _, ok := node.Args[0].(*ast.ChanType); ok {
					// Type info unavailable but AST shows it's a channel make
					pos := pkg.Package.Fset.Position(node.Pos())
					if !emitOp(ChannelOpMake, "<unknown type>", node, pos) {
						return false
					}
				}
			}

		case *ast.RangeStmt:
			elemType := ChannelElemType(pkg.Package.TypesInfo, node.X)
			pos := pkg.Package.Fset.Position(node.Pos())
			if !emitOp(ChannelOpRange, elemType, node, pos) {
				return false
			}
		}
		return true
	}

	for _, decl := range file.Decls {
		ast.Inspect(decl, inspectOps)
		if !continueWalk {
			return false
		}
	}

	return continueWalk
}
