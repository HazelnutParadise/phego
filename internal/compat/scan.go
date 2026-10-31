// Package compat finds the parts of a PHP program whose deployment behavior
// cannot be confirmed from source alone, or that need a host capability.
// It never reports a dynamic construct as safe: a target that is only known
// at run time is an Unknown finding.
package compat

import (
	"sort"
	"strings"

	"github.com/HazelnutParadise/phego/internal/frontend"
)

// Class separates why a finding matters for deployment.
type Class uint8

const (
	// Unknown means the target is computed at run time, so its function,
	// file, or class cannot be checked before deployment.
	Unknown Class = iota
	// Capability means the code needs a host capability that Phego denies
	// unless the deployment grants it. Denial is a policy difference, not
	// transparent compatibility.
	Capability
)

func (c Class) String() string {
	if c == Capability {
		return "capability"
	}
	return "unknown"
}

// Finding is one located compatibility observation.
type Finding struct {
	File string
	Line int
	// Kind names the construct, such as "dynamic-call" or "include".
	Kind  string
	Class Class
	// Capability is the host capability the construct needs, if any.
	Capability string
	// Evidence is the source text that identifies the construct.
	Evidence string
	// Impact says what the deployment must do or confirm.
	Impact string
}

// callbackArgument maps functions that call a callback to the argument
// position of that callback.
var callbackArgument = map[string]int{
	"call_user_func": 0, "call_user_func_array": 0, "forward_static_call": 0,
	"forward_static_call_array": 0, "array_map": 0, "register_shutdown_function": 0,
	"spl_autoload_register": 0, "set_error_handler": 0, "set_exception_handler": 0,
	"array_filter": 1, "array_walk": 1, "array_walk_recursive": 1, "array_reduce": 1,
	"usort": 1, "uasort": 1, "uksort": 1, "iterator_apply": 1,
	"preg_replace_callback": 1,
}

// processFunctions start operating system processes.
var processFunctions = map[string]bool{
	"exec": true, "shell_exec": true, "system": true, "passthru": true,
	"popen": true, "proc_open": true, "pcntl_exec": true,
}

// writeFunctions write to the file system; the value is the path argument.
var writeFunctions = map[string]int{
	"file_put_contents": 0, "mkdir": 0, "rmdir": 0, "unlink": 0, "touch": 0,
	"rename": 1, "copy": 1, "move_uploaded_file": 1, "tempnam": 0, "chmod": 0,
}

// Scan reports the findings of one parsed file in source order.
func Scan(file string, tree *frontend.Node) []Finding {
	s := scanner{file: file}
	s.walk(tree)
	sort.SliceStable(s.out, func(i, j int) bool { return s.out[i].Line < s.out[j].Line })
	return s.out
}

type scanner struct {
	file string
	out  []Finding
}

func (s *scanner) add(n *frontend.Node, kind string, class Class, capability, evidence, impact string) {
	s.out = append(s.out, Finding{File: s.file, Line: n.Line, Kind: kind, Class: class,
		Capability: capability, Evidence: evidence, Impact: impact})
}

func (s *scanner) walk(n *frontend.Node) {
	if n == nil {
		return
	}
	switch n.Kind {
	case "call":
		s.call(n)
	case "method_call":
		if member := child(n, 0); member != nil && !isName(child(member, 1)) {
			s.add(n, "dynamic-method", Unknown, "", source(member), "the method name is computed at run time; confirm every reachable method exists")
		}
	case "static_call":
		if member := child(n, 0); member != nil && (!isName(child(member, 0)) || !isName(child(member, 1))) {
			s.add(n, "dynamic-static-call", Unknown, "", source(member), "the class or method is computed at run time; confirm every reachable target exists")
		}
	case "new":
		if class := child(n, 0); class != nil && class.Kind != "name" && class.Kind != "new_class" {
			s.add(n, "dynamic-new", Unknown, "", source(class), "the class is computed at run time; confirm every reachable class can be loaded")
		}
	case "include":
		s.include(n)
	case "builtin":
		if strings.EqualFold(n.Text, "eval") {
			s.add(n, "eval", Unknown, "", "eval("+source(child(n, 0))+")", "evaluated code is only known at run time and is not checked")
		}
	case "interpolated":
		if n.Text == "`" {
			s.add(n, "shell", Capability, "process", "`...`", "runs a shell command; requires the process capability")
		}
	case "variable_variable":
		s.add(n, "variable-variable", Unknown, "", source(n), "the variable name is computed at run time")
	}
	for _, c := range n.Children {
		s.walk(c)
	}
}

func (s *scanner) call(n *frontend.Node) {
	callee := child(n, 0)
	if callee == nil || callee.Kind != "name" {
		s.add(n, "dynamic-call", Unknown, "", source(callee), "the function is computed at run time; confirm every reachable function exists")
		return
	}
	name := strings.ToLower(strings.TrimPrefix(callee.Text, `\`))
	if i := strings.LastIndexByte(name, '\\'); i >= 0 {
		// A namespaced call falls back to the global function at run time.
		name = name[i+1:]
	}
	args := child(n, 1)
	if pos, ok := callbackArgument[name]; ok {
		if cb := argument(args, pos); !staticCallable(cb) {
			s.add(n, "dynamic-callback", Unknown, "", callee.Text+"("+source(cb)+")", "the callback is computed at run time; confirm every reachable callable exists")
		}
	}
	if processFunctions[name] {
		s.add(n, "process", Capability, "process", callee.Text+"()", "starts an operating system process; requires the process capability")
	}
	if pos, ok := writeFunctions[name]; ok {
		path := argument(args, pos)
		if isStringLiteral(path) {
			s.add(n, "file-write", Capability, "filesystem.write", callee.Text+"("+path.Text+")", "writes to the file system; the target directory must be granted as writable")
		} else {
			s.add(n, "file-write", Unknown, "filesystem.write", callee.Text+"("+source(path)+")", "writes to a path computed at run time; the writable directories cannot be confirmed")
		}
	}
}

func (s *scanner) include(n *frontend.Node) {
	target := child(n, 0)
	if path, ok := staticPath(target); ok {
		s.add(n, "include", Capability, "filesystem.read", n.Text+" "+path, "reads a source file; the file must be inside the granted source tree")
		return
	}
	s.add(n, "dynamic-include", Unknown, "filesystem.read", n.Text+" "+source(target), "the included file is computed at run time and is not checked")
}

// staticPath recognizes a literal path, optionally prefixed by __DIR__.
func staticPath(n *frontend.Node) (string, bool) {
	if isStringLiteral(n) {
		return n.Text, true
	}
	if n != nil && n.Kind == "binary" && n.Text == "." {
		l, r := child(n, 0), child(n, 1)
		if l != nil && l.Kind == "magic_const" && strings.EqualFold(l.Text, "__DIR__") && isStringLiteral(r) {
			return "__DIR__." + r.Text, true
		}
	}
	return "", false
}

func child(n *frontend.Node, i int) *frontend.Node {
	if n == nil || i >= len(n.Children) {
		return nil
	}
	return n.Children[i]
}

func argument(args *frontend.Node, pos int) *frontend.Node {
	a := child(args, pos)
	if a != nil && a.Kind == "spread" {
		return nil
	}
	return a
}

// staticCallable recognizes a callback written in the source: a function
// name string or a closure.
func staticCallable(n *frontend.Node) bool {
	return isStringLiteral(n) || (n != nil && (n.Kind == "closure" || n.Kind == "arrow_function"))
}

func isName(n *frontend.Node) bool { return n != nil && (n.Kind == "name" || n.Kind == "identifier") }

func isStringLiteral(n *frontend.Node) bool {
	return n != nil && n.Kind == "literal" && n.Token == frontend.TConstantEncapsedString
}

// source gives a short description of an expression for evidence.
func source(n *frontend.Node) string {
	switch {
	case n == nil:
		return "..."
	case n.Text != "" && len(n.Children) == 0:
		return n.Text
	case n.Kind == "variable_variable":
		return "$" + source(child(n, 0))
	}
	return n.Kind + " expression"
}
