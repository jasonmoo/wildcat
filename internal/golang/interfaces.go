package golang

import (
	"go/types"

	"golang.org/x/tools/go/packages"
)

// InterfaceInfo holds information about an interface type.
type InterfaceInfo struct {
	Package *Package     // the package containing the interface (nil for stdlib/builtins)
	Name    string       // interface name
	Named   *types.Named // the named type (may be generic, nil for builtins like error)
	// For stdlib/builtins where Package is nil:
	pkgPath string
	pkgName string
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
	return i.pkgPath
}

// QualifiedName returns the fully qualified name (pkgPath.Name).
func (i *InterfaceInfo) QualifiedName() string {
	if path := i.PkgPath(); path != "" {
		return path + "." + i.Name
	}
	return i.Name
}

// CollectInterfaces gathers all interface types from project packages and stdlib.
func CollectInterfaces(project *Project, stdlib []*packages.Package) []InterfaceInfo {
	var ifaces []InterfaceInfo

	// From project packages
	for _, p := range project.Packages {
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

	// From stdlib
	for _, p := range stdlib {
		for _, name := range p.Types.Scope().Names() {
			obj := p.Types.Scope().Lookup(name)
			if obj == nil {
				continue
			}
			if _, ok := obj.(*types.TypeName); !ok {
				continue
			}
			named, ok := obj.Type().(*types.Named)
			if !ok {
				continue
			}
			if _, ok := named.Underlying().(*types.Interface); ok {
				ifaces = append(ifaces, InterfaceInfo{
					Package: nil,
					Name:    name,
					Named:   named,
					pkgPath: p.PkgPath,
					pkgName: p.Name,
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
func IsInterfaceMethod(sym *Symbol, project *Project, stdlib []*packages.Package) bool {
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
