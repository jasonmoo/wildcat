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

// PkgPath returns the package import path.
func (i *InterfaceInfo) PkgPath() string {
	if i.Package != nil {
		return i.Package.Identifier.PkgPath
	}
	return "" // builtin
}

// QualifiedName returns the fully qualified name (pkgPath.Name).
func (i *InterfaceInfo) QualifiedName() string {
	if path := i.PkgPath(); path != "" {
		return path + "." + i.Name
	}
	return i.Name
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

// ImplementorInfo holds information about a type that implements an interface.
type ImplementorInfo struct {
	Package *Package     // the package containing the type
	Name    string       // type name
	Obj     types.Object // the type object (for position info)
}

// PkgPath returns the package import path.
func (i *ImplementorInfo) PkgPath() string {
	return i.Package.Identifier.PkgPath
}

// QualifiedName returns the fully qualified name (pkgPath.Name).
func (i *ImplementorInfo) QualifiedName() string {
	return i.PkgPath() + "." + i.Name
}

// FindImplementors finds all types in packages that implement the given interface.
// It checks both T and *T for each type.
func FindImplementors(iface *types.Interface, ifacePkgPath, ifaceName string, packages []*Package) []ImplementorInfo {
	var implementors []ImplementorInfo

	for _, pkg := range packages {
		for _, tname := range pkg.Package.Types.Scope().Names() {
			tobj := pkg.Package.Types.Scope().Lookup(tname)
			if tobj == nil {
				continue
			}

			T := tobj.Type()
			ptrT := types.NewPointer(T)

			// Check if T or *T implements the interface
			if types.Implements(T, iface) || types.Implements(ptrT, iface) {
				// Skip the interface itself
				if pkg.Identifier.PkgPath == ifacePkgPath && tname == ifaceName {
					continue
				}

				implementors = append(implementors, ImplementorInfo{
					Package: pkg,
					Name:    tname,
					Obj:     tobj,
				})
			}
		}
	}

	return implementors
}

// IsInterfaceMethod checks if a method is required by an interface that its
// receiver type implements. This is used for dead code analysis: methods that
// implement interfaces should not be reported as dead if the type is used.
func IsInterfaceMethod(sym *Symbol, project *Project, stdlib []*Package) bool {
	if sym.Kind != SymbolKindMethod {
		return false
	}

	// Get the method's function object
	methodObj := GetTypesObject(sym)
	if methodObj == nil {
		return false
	}
	methodFunc, ok := methodObj.(*types.Func)
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

	// Extract just the method name (sym.Name includes "ReceiverType.MethodName")
	methodName := methodFunc.Name()

	// Collect all interfaces
	ifaces := CollectInterfaces(project, stdlib)

	// Check if receiver type implements any interface with this method
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

// FindSatisfiedInterfaces finds all interfaces that a type satisfies.
// It handles:
// - Builtin error interface
// - Generic interface instantiation
// - Skips empty interfaces
func FindSatisfiedInterfaces(T types.Type, tPkgPath, tName string, interfaces []InterfaceInfo) []InterfaceInfo {
	var satisfies []InterfaceInfo
	ptrT := types.NewPointer(T)

	// Check builtin error interface
	errorObj := types.Universe.Lookup("error")
	if errorObj != nil {
		errorIface := errorObj.Type().Underlying().(*types.Interface)
		if types.Implements(T, errorIface) || types.Implements(ptrT, errorIface) {
			satisfies = append(satisfies, InterfaceInfo{
				Package: nil,
				Name:    "error",
				Named:   nil, // builtin, no Named type
			})
		}
	}

	for _, i := range interfaces {
		// Skip self
		if i.PkgPath() == tPkgPath && i.Name == tName {
			continue
		}

		iface := i.Interface()

		// Skip empty interface
		if iface.NumMethods() == 0 {
			continue
		}

		var implements bool

		if i.Named.TypeParams().Len() > 0 {
			// Generic interface - try instantiating with T and *T
			if inst, err := types.Instantiate(nil, i.Named, []types.Type{T}, false); err == nil {
				if instIface, ok := inst.Underlying().(*types.Interface); ok {
					implements = types.Implements(T, instIface) || types.Implements(ptrT, instIface)
				}
			}
			if !implements {
				if inst, err := types.Instantiate(nil, i.Named, []types.Type{ptrT}, false); err == nil {
					if instIface, ok := inst.Underlying().(*types.Interface); ok {
						implements = types.Implements(T, instIface) || types.Implements(ptrT, instIface)
					}
				}
			}
		} else {
			// Non-generic interface
			implements = types.Implements(T, iface) || types.Implements(ptrT, iface)
		}

		if implements {
			satisfies = append(satisfies, i)
		}
	}

	return satisfies
}

// ComputeInterfaceRelations populates Satisfies and ImplementedBy on all type symbols
// in project packages. This should be called after loading all packages.
func ComputeInterfaceRelations(project []*Package, stdlib []*Package) {
	// Build a map of all type symbols for quick lookup
	// key: pkgPath + "." + name -> *PackageSymbol
	typeSymbols := make(map[string]*PackageSymbol)

	allPkgs := append(project, stdlib...)
	for _, pkg := range allPkgs {
		for _, sym := range pkg.Symbols {
			if _, ok := sym.Object.(*types.TypeName); !ok {
				continue
			}
			key := pkg.Identifier.PkgPath + "." + sym.Name
			typeSymbols[key] = sym
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
						otherTn, ok := otherSym.Object.(*types.TypeName)
						if !ok {
							continue
						}
						// Skip self
						if otherPkg.Identifier.PkgPath == pkg.Identifier.PkgPath && otherSym.Name == sym.Name {
							continue
						}
						// Skip other interfaces
						if _, ok := otherTn.Type().Underlying().(*types.Interface); ok {
							continue
						}
						otherT := otherTn.Type()
						otherPtrT := types.NewPointer(otherT)

						var implements bool
						if isGenericIface {
							// For generic interfaces like Command[T], try instantiating with the candidate type
							// e.g., check if *SymbolCommand implements Command[SymbolCommandResponse]
							if inst, err := types.Instantiate(nil, named, []types.Type{otherT}, false); err == nil {
								if instIface, ok := inst.Underlying().(*types.Interface); ok {
									implements = types.Implements(otherT, instIface) || types.Implements(otherPtrT, instIface)
								}
							}
							if !implements {
								if inst, err := types.Instantiate(nil, named, []types.Type{otherPtrT}, false); err == nil {
									if instIface, ok := inst.Underlying().(*types.Interface); ok {
										implements = types.Implements(otherT, instIface) || types.Implements(otherPtrT, instIface)
									}
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
						// Create a synthetic PackageSymbol for builtin error
						// We'll handle this specially - for now skip it since there's no Package
						// TODO: consider creating a synthetic "builtin" package
					}
				}

				for _, otherPkg := range allPkgs {
					for _, otherSym := range otherPkg.Symbols {
						if _, ok := otherSym.Object.(*types.TypeName); !ok {
							continue
						}
						otherIface, ok := otherSym.Object.Type().Underlying().(*types.Interface)
						if !ok {
							continue
						}
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
							if inst, err := types.Instantiate(nil, named, []types.Type{T}, false); err == nil {
								if instIface, ok := inst.Underlying().(*types.Interface); ok {
									implements = types.Implements(T, instIface) || types.Implements(ptrT, instIface)
								}
							}
							if !implements {
								if inst, err := types.Instantiate(nil, named, []types.Type{ptrT}, false); err == nil {
									if instIface, ok := inst.Underlying().(*types.Interface); ok {
										implements = types.Implements(T, instIface) || types.Implements(ptrT, instIface)
									}
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
			tn, ok := sym.Object.(*types.TypeName)
			if !ok {
				continue
			}
			iface, isIface := tn.Type().Underlying().(*types.Interface)
			if !isIface || iface.NumMethods() == 0 {
				continue
			}

			// Find consumer functions across all project packages
			for _, otherPkg := range project {
				for _, otherSym := range otherPkg.Symbols {
					// Check top-level functions
					if fn, ok := otherSym.Object.(*types.Func); ok {
						if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() == nil {
							if hasIfaceParam(sig, sym.Object) {
								sym.Consumers = append(sym.Consumers, otherSym)
							}
						}
					}
					// Check methods on types
					for _, method := range otherSym.Methods {
						if fn, ok := method.Object.(*types.Func); ok {
							if sig, ok := fn.Type().(*types.Signature); ok {
								if hasIfaceParam(sig, sym.Object) {
									sym.Consumers = append(sym.Consumers, method)
								}
							}
						}
					}
				}
			}
		}
	}
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
