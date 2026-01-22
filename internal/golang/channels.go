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
	Package  *Package      // package containing the operation
	File     string        // file path
	Line     int           // line number
	Kind     ChannelOpKind // type of operation
	ElemType string        // channel element type
	Node     ast.Node      // the AST node
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

	// First pass: identify all select statement channel operations
	ast.Inspect(file, func(n ast.Node) bool {
		if !continueWalk {
			return false
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
				if elemType := ChannelElemType(pkg.Package.TypesInfo, node.Chan); elemType != "" {
					pos := pkg.Package.Fset.Position(node.Pos())
					op := ChannelOp{
						Package:  pkg,
						File:     filename,
						Line:     pos.Line,
						Kind:     ChannelOpSelectSend,
						ElemType: elemType,
						Node:     node,
					}
					if !visitor(op) {
						continueWalk = false
						return false
					}
				}

			case *ast.ExprStmt:
				if recv, ok := node.X.(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
					selectNodes[recv] = true
					if elemType := ChannelElemType(pkg.Package.TypesInfo, recv.X); elemType != "" {
						pos := pkg.Package.Fset.Position(recv.Pos())
						op := ChannelOp{
							Package:  pkg,
							File:     filename,
							Line:     pos.Line,
							Kind:     ChannelOpSelectReceive,
							ElemType: elemType,
							Node:     recv,
						}
						if !visitor(op) {
							continueWalk = false
							return false
						}
					}
				}

			case *ast.AssignStmt:
				if len(node.Rhs) == 1 {
					if recv, ok := node.Rhs[0].(*ast.UnaryExpr); ok && recv.Op == token.ARROW {
						selectNodes[recv] = true
						if elemType := ChannelElemType(pkg.Package.TypesInfo, recv.X); elemType != "" {
							pos := pkg.Package.Fset.Position(recv.Pos())
							op := ChannelOp{
								Package:  pkg,
								File:     filename,
								Line:     pos.Line,
								Kind:     ChannelOpSelectReceive,
								ElemType: elemType,
								Node:     recv,
							}
							if !visitor(op) {
								continueWalk = false
								return false
							}
						}
					}
				}
			}
		}
		return true
	})

	if !continueWalk {
		return false
	}

	// Second pass: collect non-select channel operations
	ast.Inspect(file, func(n ast.Node) bool {
		if !continueWalk {
			return false
		}

		switch node := n.(type) {
		case *ast.SendStmt:
			if selectNodes[node] {
				return true
			}
			if elemType := ChannelElemType(pkg.Package.TypesInfo, node.Chan); elemType != "" {
				pos := pkg.Package.Fset.Position(node.Pos())
				op := ChannelOp{
					Package:  pkg,
					File:     filename,
					Line:     pos.Line,
					Kind:     ChannelOpSend,
					ElemType: elemType,
					Node:     node,
				}
				if !visitor(op) {
					continueWalk = false
					return false
				}
			}

		case *ast.UnaryExpr:
			if selectNodes[node] {
				return true
			}
			if node.Op == token.ARROW {
				if elemType := ChannelElemType(pkg.Package.TypesInfo, node.X); elemType != "" {
					pos := pkg.Package.Fset.Position(node.Pos())
					op := ChannelOp{
						Package:  pkg,
						File:     filename,
						Line:     pos.Line,
						Kind:     ChannelOpReceive,
						ElemType: elemType,
						Node:     node,
					}
					if !visitor(op) {
						continueWalk = false
						return false
					}
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
					if elemType := ChannelElemType(pkg.Package.TypesInfo, node.Args[0]); elemType != "" {
						pos := pkg.Package.Fset.Position(node.Pos())
						op := ChannelOp{
							Package:  pkg,
							File:     filename,
							Line:     pos.Line,
							Kind:     ChannelOpClose,
							ElemType: elemType,
							Node:     node,
						}
						if !visitor(op) {
							continueWalk = false
							return false
						}
					}
				}

			case "make":
				if t := pkg.Package.TypesInfo.TypeOf(node); t != nil {
					if ch, ok := t.Underlying().(*types.Chan); ok {
						elemType := ch.Elem().String()
						pos := pkg.Package.Fset.Position(node.Pos())
						op := ChannelOp{
							Package:  pkg,
							File:     filename,
							Line:     pos.Line,
							Kind:     ChannelOpMake,
							ElemType: elemType,
							Node:     node,
						}
						if !visitor(op) {
							continueWalk = false
							return false
						}
					}
				}
			}

		case *ast.RangeStmt:
			if elemType := ChannelElemType(pkg.Package.TypesInfo, node.X); elemType != "" {
				pos := pkg.Package.Fset.Position(node.Pos())
				op := ChannelOp{
					Package:  pkg,
					File:     filename,
					Line:     pos.Line,
					Kind:     ChannelOpRange,
					ElemType: elemType,
					Node:     node,
				}
				if !visitor(op) {
					continueWalk = false
					return false
				}
			}
		}
		return true
	})

	return continueWalk
}
