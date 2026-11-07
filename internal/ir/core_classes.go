package ir

import (
	"strings"

	"github.com/HazelnutParadise/phego/internal/profile"
)

// Core inheritance metadata is shared read-only. These PHP 8.5.12 facts come
// from Zend/zend_generators.stub.php and Zend/zend_weakrefs.stub.php. They do
// not provide executable built-in methods or extension-dependent classes.
var coreClassMetadata = func() map[string]*Class {
	typ := func(name string) *Type {
		if name == "" {
			return nil
		}
		return &Type{String: name, zend: name}
	}
	method := func(name, ret string, params ...*Param) *Func {
		return &Func{Name: name, Kind: FuncMethod, HasBody: true, Return: typ(ret), Params: params}
	}
	param := func(name, t string) *Param { return &Param{Name: name, Type: typ(t)} }
	generator := &Class{Name: "Generator", Modifiers: []string{"final"}, Interfaces: []*Name{{Kind: ClassName, Resolved: "Iterator"}}, Methods: []*Func{
		method("rewind", "void"), method("valid", "bool"), method("current", "mixed"), method("key", "mixed"), method("next", "void"),
		method("send", "mixed", param("value", "mixed")), method("throw", "mixed", param("exception", "Throwable")), method("getReturn", "mixed"), method("__debugInfo", "array"),
	}}
	weakReference := &Class{Name: "WeakReference", Modifiers: []string{"final"}, Methods: []*Func{
		method("__construct", ""), method("create", "WeakReference", param("object", "object")), method("get", "?object"),
	}}
	weakReference.Methods[1].Static = true
	weakMap := &Class{Name: "WeakMap", Modifiers: []string{"final"}, Interfaces: []*Name{{Kind: ClassName, Resolved: "ArrayAccess"}, {Kind: ClassName, Resolved: "Countable"}, {Kind: ClassName, Resolved: "IteratorAggregate"}}, Methods: []*Func{
		method("offsetGet", "mixed", param("object", "")), method("offsetSet", "void", param("object", ""), param("value", "mixed")),
		method("offsetExists", "bool", param("object", "")), method("offsetUnset", "void", param("object", "")), method("count", "int"), method("getIterator", "Iterator"),
	}}
	out := make(map[string]*Class, 3)
	for _, cl := range []*Class{generator, weakReference, weakMap} {
		for _, f := range cl.Methods {
			f.Class = cl
		}
		out[strings.ToLower(cl.Name)] = cl
	}
	return out
}()

func builtinCoreClass(prof profile.Profile, name string) *Class {
	if prof.Release != "8.5.12" || prof.IntWidth != 32 && prof.IntWidth != 64 {
		return nil
	}
	return coreClassMetadata[strings.ToLower(strings.TrimPrefix(name, "\\"))]
}
