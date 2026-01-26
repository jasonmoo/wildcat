package golang

import (
	"go/types"
)

// InterfaceInfo holds information about an interface type.
type InterfaceInfo struct {
	Package *Package     // the package containing the interface (nil only for builtins like error)
	Name    string       // interface name
	Named   *types.Named // the named type (may be generic, nil for builtins like error)
}

// Interface returns the underlying *types.Interface.
func (i *InterfaceInfo) Interface() *types.Interface {
	return i.Named.Underlying().(*types.Interface)
}

// CollectInterfaces gathers all interface types from project packages and stdlib.
func CollectInterfaces(project *Project, stdlib []*Package) []InterfaceInfo {
	var ifaces []InterfaceInfo

	// Collect from all packages (project + stdlib)
	allPackages := project.Packages
	allPackages = append(allPackages, stdlib...)

	for _, p := range allPackages {
		for _, name := range p.Package.Types.Scope().Names() {
			obj := p.Package.Types.Scope().Lookup(name)
			if obj == nil {
				continue
			}
			// Only consider type declarations, not variables
			if _, ok := obj.(*types.TypeName); !ok {
				continue
			}
			named, ok := obj.Type().(*types.Named)
			if !ok {
				continue
			}
			if _, ok := named.Underlying().(*types.Interface); ok {
				ifaces = append(ifaces, InterfaceInfo{
					Package: p,
					Name:    name,
					Named:   named,
				})
			}
		}
	}

	return ifaces
}

// IsInterfaceMethod checks if a method is required by an interface that its
// receiver type implements. This is used for dead code analysis: methods that
// implement interfaces should not be reported as dead if the type is used.
func IsInterfaceMethod(sym *Symbol, project *Project, stdlib []*Package) bool {
	if sym.Kind != SymbolKindMethod {
		return false
	}

	// Get the method's function object
	methodFunc, ok := sym.Object.(*types.Func)
	if !ok {
		return false
	}

	// Get the receiver type
	sig := methodFunc.Signature()
	if sig.Recv() == nil {
		return false
	}

	recvType := sig.Recv().Type()
	// Get the base type (strip pointer if present)
	if ptr, ok := recvType.(*types.Pointer); ok {
		recvType = ptr.Elem()
	}

	methodName := methodFunc.Name()

	// Collect all interfaces and check if receiver type implements any with this method
	ifaces := CollectInterfaces(project, stdlib)
	ptrRecvType := types.NewPointer(recvType)

	for _, ifaceInfo := range ifaces {
		iface := ifaceInfo.Interface()
		if iface.NumMethods() == 0 {
			continue
		}

		var implements bool

		// Handle generic interfaces
		if ifaceInfo.Named.TypeParams().Len() > 0 {
			// Try instantiating with T and *T
			if inst, err := types.Instantiate(nil, ifaceInfo.Named, []types.Type{recvType}, false); err == nil {
				if instIface, ok := inst.Underlying().(*types.Interface); ok {
					implements = types.Implements(recvType, instIface) || types.Implements(ptrRecvType, instIface)
				}
			}
			if !implements {
				if inst, err := types.Instantiate(nil, ifaceInfo.Named, []types.Type{ptrRecvType}, false); err == nil {
					if instIface, ok := inst.Underlying().(*types.Interface); ok {
						implements = types.Implements(recvType, instIface) || types.Implements(ptrRecvType, instIface)
					}
				}
			}
		} else {
			// Non-generic interface
			implements = types.Implements(recvType, iface) || types.Implements(ptrRecvType, iface)
		}

		if !implements {
			continue
		}

		// Check if the interface has a method with this name
		for i := 0; i < iface.NumMethods(); i++ {
			if iface.Method(i).Name() == methodName {
				return true
			}
		}
	}

	return false
}

// InterfaceIssue represents an issue encountered during interface relation computation.
type InterfaceIssue struct {
	TypePkgIdent  *PackageIdentifier // package of the type being checked
	TypeName      string             // name of the type
	IfacePkgIdent *PackageIdentifier // package of the interface
	IfaceName     string             // name of the interface
	Error         string             // the error message
}

// ComputeInterfaceRelations populates Satisfies and ImplementedBy on all type symbols
// in project packages. This should be called after loading all packages.
// Returns any issues encountered during generic interface instantiation.
func ComputeInterfaceRelations(project []*Package, stdlib []*Package) []InterfaceIssue {
	var issues []InterfaceIssue
	// Build a map of all type symbols for quick lookup
	// key: pkgPath + "." + name -> *Symbol
	typeSymbols := make(map[string]*Symbol)

	allPkgs := append(project, stdlib...)
	for _, pkg := range allPkgs {
		for _, sym := range pkg.Symbols {
			if sym.Kind != SymbolKindType && sym.Kind != SymbolKindInterface {
				continue
			}
			typeSymbols[sym.Id()] = sym
		}
	}

	// For each type in project packages, compute relationships
	for _, pkg := range project {
		for _, sym := range pkg.Symbols {
			tn, ok := sym.Object.(*types.TypeName)
			if !ok {
				continue
			}

			T := tn.Type()
			ptrT := types.NewPointer(T)

			// Check if this is an interface
			iface, isIface := T.Underlying().(*types.Interface)

			if isIface {
				// Find implementors across all project packages
				if iface.NumMethods() == 0 {
					continue // skip empty interfaces
				}

				// Check if this is a generic interface
				named, _ := T.(*types.Named)
				isGenericIface := named != nil && named.TypeParams().Len() > 0

				for _, otherPkg := range project {
					for _, otherSym := range otherPkg.Symbols {
						// Skip non-types and interfaces (only concrete types can implement)
						if otherSym.Kind != SymbolKindType {
							continue
						}
						// Skip self
						if otherPkg.Identifier.PkgPath == pkg.Identifier.PkgPath && otherSym.Name == sym.Name {
							continue
						}
						otherTn := otherSym.Object.(*types.TypeName)
						otherT := otherTn.Type()
						otherPtrT := types.NewPointer(otherT)

						var implements bool
						if isGenericIface {
							// For generic interfaces like Command[T], try instantiating with the candidate type
							// e.g., check if *SymbolCommand implements Command[SymbolCommandResponse]
							var instErr error
							if inst, err := types.Instantiate(nil, named, []types.Type{otherT}, false); err == nil {
								if instIface, ok := inst.Underlying().(*types.Interface); ok {
									implements = types.Implements(otherT, instIface) || types.Implements(otherPtrT, instIface)
								}
							} else {
								instErr = err
							}
							if !implements {
								if inst, err := types.Instantiate(nil, named, []types.Type{otherPtrT}, false); err == nil {
									if instIface, ok := inst.Underlying().(*types.Interface); ok {
										implements = types.Implements(otherT, instIface) || types.Implements(otherPtrT, instIface)
									}
								} else if instErr != nil {
									// Both instantiations failed - record the issue
									issues = append(issues, InterfaceIssue{
										TypePkgIdent:  otherPkg.Identifier,
										TypeName:      otherSym.Name,
										IfacePkgIdent: pkg.Identifier,
										IfaceName:     sym.Name,
										Error:         instErr.Error(),
									})
								}
							}
						} else {
							implements = types.Implements(otherT, iface) || types.Implements(otherPtrT, iface)
						}

						if implements {
							sym.ImplementedBy = append(sym.ImplementedBy, otherSym)
						}
					}
				}
			} else {
				// Find satisfied interfaces across all packages (project + stdlib)
				// Check builtin error interface first
				errorObj := types.Universe.Lookup("error")
				if errorObj != nil {
					errorIface := errorObj.Type().Underlying().(*types.Interface)
					if types.Implements(T, errorIface) || types.Implements(ptrT, errorIface) {
						// Create a synthetic Symbol for builtin error
						// We'll handle this specially - for now skip it since there's no Package
						// TODO: consider creating a synthetic "builtin" package
					}
				}

				for _, otherPkg := range allPkgs {
					for _, otherSym := range otherPkg.Symbols {
						if otherSym.Kind != SymbolKindInterface {
							continue
						}
						otherIface := otherSym.Object.Type().Underlying().(*types.Interface)
						// Skip empty interfaces
						if otherIface.NumMethods() == 0 {
							continue
						}
						// Skip self
						if otherPkg.Identifier.PkgPath == pkg.Identifier.PkgPath && otherSym.Name == sym.Name {
							continue
						}

						var implements bool

						// Handle generic interfaces
						if named, ok := otherSym.Object.Type().(*types.Named); ok && named.TypeParams().Len() > 0 {
							// Try instantiating with T and *T
							var instErr error
							if inst, err := types.Instantiate(nil, named, []types.Type{T}, false); err == nil {
								if instIface, ok := inst.Underlying().(*types.Interface); ok {
									implements = types.Implements(T, instIface) || types.Implements(ptrT, instIface)
								}
							} else {
								instErr = err
							}
							if !implements {
								if inst, err := types.Instantiate(nil, named, []types.Type{ptrT}, false); err == nil {
									if instIface, ok := inst.Underlying().(*types.Interface); ok {
										implements = types.Implements(T, instIface) || types.Implements(ptrT, instIface)
									}
								} else if instErr != nil {
									// Both instantiations failed - record the issue
									issues = append(issues, InterfaceIssue{
										TypePkgIdent:  pkg.Identifier,
										TypeName:      sym.Name,
										IfacePkgIdent: otherPkg.Identifier,
										IfaceName:     otherSym.Name,
										Error:         instErr.Error(),
									})
								}
							}
						} else {
							// Non-generic interface
							implements = types.Implements(T, otherIface) || types.Implements(ptrT, otherIface)
						}

						if implements {
							sym.Satisfies = append(sym.Satisfies, otherSym)
						}
					}
				}
			}
		}
	}

	// Compute Consumers for interfaces: functions/methods that accept the interface as param
	for _, pkg := range project {
		for _, sym := range pkg.Symbols {
			if sym.Kind != SymbolKindInterface {
				continue
			}
			iface := sym.Object.Type().Underlying().(*types.Interface)
			if iface.NumMethods() == 0 {
				continue
			}

			// Find consumer functions across all project packages
			for _, otherPkg := range project {
				for _, otherSym := range otherPkg.Symbols {
					// Check top-level functions
					if otherSym.Kind == SymbolKindFunc {
						fn := otherSym.Object.(*types.Func)
						if hasIfaceParam(fn.Signature(), sym.Object) {
							sym.Consumers = append(sym.Consumers, otherSym)
						}
					}
					// Check methods on types
					for _, method := range otherSym.Methods {
						fn := method.Object.(*types.Func)
						if hasIfaceParam(fn.Signature(), sym.Object) {
							sym.Consumers = append(sym.Consumers, method)
						}
					}
				}
			}
		}
	}

	return issues
}

// hasIfaceParam checks if a function signature has a parameter of the target interface type.
func hasIfaceParam(sig *types.Signature, targetObj types.Object) bool {
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		paramType := params.At(i).Type()
		// Check direct interface type
		if named, ok := paramType.(*types.Named); ok {
			if named.Obj() == targetObj {
				return true
			}
		}
		// Check pointer to interface
		if ptr, ok := paramType.(*types.Pointer); ok {
			if named, ok := ptr.Elem().(*types.Named); ok {
				if named.Obj() == targetObj {
					return true
				}
			}
		}
	}
	return false
}
