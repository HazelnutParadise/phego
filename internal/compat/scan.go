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

// lastArgument marks a callback passed as the final argument.
const lastArgument = -1

// callbackArgument maps functions that call a callback to the argument
// position of that callback.
var callbackArgument = map[string]int{
	"call_user_func": 0, "call_user_func_array": 0, "forward_static_call": 0,
	"forward_static_call_array": 0, "array_map": 0, "register_shutdown_function": 0,
	"spl_autoload_register": 0, "set_error_handler": 0, "set_exception_handler": 0,
	"ob_start": 0, "register_tick_function": 0, "header_register_callback": 0,
	"preg_replace_callback_array": 0,
	"array_filter":                1, "array_walk": 1, "array_walk_recursive": 1, "array_reduce": 1,
	"usort": 1, "uasort": 1, "uksort": 1, "iterator_apply": 1,
	"preg_replace_callback": 1, "mb_ereg_replace_callback": 1,
	"array_udiff": lastArgument, "array_udiff_assoc": lastArgument, "array_udiff_uassoc": lastArgument,
	"array_uintersect": lastArgument, "array_uintersect_assoc": lastArgument,
	"array_uintersect_uassoc": lastArgument, "array_diff_ukey": lastArgument,
	"array_diff_uassoc": lastArgument, "array_intersect_ukey": lastArgument,
	"array_intersect_uassoc": lastArgument,
}

// processFunctions start operating system processes. mail() runs the
// configured sendmail program.
var processFunctions = map[string]bool{
	"exec": true, "shell_exec": true, "system": true, "passthru": true,
	"popen": true, "proc_open": true, "pcntl_exec": true, "mail": true,
}

// writeFunctions write to the file system; the value is the path argument.
var writeFunctions = map[string]int{
	"file_put_contents": 0, "mkdir": 0, "rmdir": 0, "unlink": 0, "touch": 0,
	"rename": 1, "copy": 1, "move_uploaded_file": 1, "tempnam": 0, "chmod": 0,
	"chown": 0, "chgrp": 0, "lchown": 0, "lchgrp": 0, "symlink": 1, "link": 1,
}

// remoteSchemes are include targets that read over the network.
var remoteSchemes = []string{"http://", "https://", "ftp://", "ftps://"}

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
	// namespace is the current namespace, lower-cased, and functions maps
	// the lower-cased alias of each "use function" import to its target.
	namespace string
	functions map[string]string
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
	case "namespace":
		s.namespace = strings.ToLower(textOf(child(n, 0), ""))
		s.functions = nil
	case "use":
		if n.Text == "function" {
			s.importFunctions(n)
		}
	case "call":
		s.call(n)
	case "method_call":
		if member := child(n, 0); member != nil && !isName(child(member, 1)) {
			s.add(n, "dynamic-method", Unknown, "", source(member), "the method name is computed at run time; confirm every reachable method exists")
		}
	case "static_call":
		member := child(n, 0)
		if member != nil && (!isName(child(member, 0)) || !isName(child(member, 1))) {
			s.add(n, "dynamic-static-call", Unknown, "", source(member), "the class or method is computed at run time; confirm every reachable target exists")
		} else if member != nil && strings.EqualFold(strings.TrimPrefix(child(member, 0).Text, `\`), "closure") &&
			strings.EqualFold(child(member, 1).Text, "fromCallable") && !staticCallable(argument(child(n, 1), 0)) {
			s.add(n, "dynamic-callback", Unknown, "", "Closure::fromCallable("+source(argument(child(n, 1), 0))+")", "the callback is computed at run time; confirm every reachable callable exists")
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

// importFunctions records the aliases of a "use function" statement.
func (s *scanner) importFunctions(n *frontend.Node) {
	if s.functions == nil {
		s.functions = map[string]string{}
	}
	record := func(prefix string, item *frontend.Node) {
		target := strings.TrimPrefix(textOf(child(item, 0), ""), `\`)
		if prefix != "" {
			target = prefix + `\` + target
		}
		alias := textOf(child(item, 1), target[strings.LastIndexByte(target, '\\')+1:])
		s.functions[strings.ToLower(alias)] = strings.ToLower(target)
	}
	for _, c := range n.Children {
		switch {
		case c == nil:
		case c.Kind == "use_item":
			record("", c)
		case c.Kind == "group_use":
			prefix := strings.TrimPrefix(textOf(child(c, 0), ""), `\`)
			for _, item := range c.Children[1:] {
				if item != nil && item.Kind == "use_item" {
					record(prefix, item)
				}
			}
		}
	}
}

// globalFunction resolves a called name to the global function PHP would
// call, or "" when the call cannot reach a global function. An unqualified
// name outside the global namespace falls back to the global function unless
// the namespace defines one, which this scan assumes it does not.
func (s *scanner) globalFunction(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, `\`):
		lower = lower[1:]
	case strings.ContainsRune(lower, '\\'):
		if s.namespace == "" {
			break
		}
		return ""
	default:
		if target, ok := s.functions[lower]; ok {
			lower = target
		}
	}
	if strings.ContainsRune(lower, '\\') {
		return ""
	}
	return lower
}

// callbackFunction resolves a function named by a callback string. Such a
// name is always fully qualified: neither the namespace nor "use function"
// applies.
func callbackFunction(name string) string {
	name = strings.ToLower(strings.TrimPrefix(name, `\`))
	if strings.ContainsRune(name, '\\') {
		return ""
	}
	return name
}

func (s *scanner) call(n *frontend.Node) {
	callee := child(n, 0)
	if callee == nil || callee.Kind != "name" {
		s.add(n, "dynamic-call", Unknown, "", source(callee), "the function is computed at run time; confirm every reachable function exists")
		return
	}
	name := s.globalFunction(callee.Text)
	args := child(n, 1)
	if args != nil && args.Kind == "callable_args" {
		return // first-class callable syntax creates a closure without calling
	}
	if pos, ok := callbackArgument[name]; ok {
		if pos == lastArgument {
			pos = len(args.Children) - 1
		}
		cb := argument(args, pos)
		switch {
		case cb == nil && args != nil && pos < len(args.Children):
			s.add(n, "dynamic-callback", Unknown, "", callee.Text+"(...)", "the callback is computed at run time; confirm every reachable callable exists")
		case cb == nil, isNull(cb):
			// No callback is passed, so none is called.
		case isStringLiteral(cb):
			s.function(n, callbackFunction(unquote(cb.Text)), nil, callee.Text+"("+cb.Text+")")
		case !staticCallable(cb):
			s.add(n, "dynamic-callback", Unknown, "", callee.Text+"("+source(cb)+")", "the callback is computed at run time; confirm every reachable callable exists")
		}
	}
	if name == "extract" {
		s.add(n, "extract", Unknown, "", callee.Text+"("+source(argument(args, 0))+")", "creates variables whose names come from data at run time")
	}
	s.function(n, name, args, callee.Text+"()")
}

// function reports the capability a call to the global function name needs.
// args is nil when the function is named as a callback, so its arguments are
// unknown.
func (s *scanner) function(n *frontend.Node, name string, args *frontend.Node, evidence string) {
	if processFunctions[name] {
		s.add(n, "process", Capability, "process", evidence, "starts an operating system process; requires the process capability")
	}
	if name == "fopen" {
		mode := argument(args, 1)
		switch {
		case args == nil || !isStringLiteral(mode):
			s.add(n, "file-write", Unknown, "filesystem.write", evidence, "opens a file with a mode computed at run time; it may write")
		case strings.ContainsAny(unquote(mode.Text), "waxc+"):
			s.writeFinding(n, name, argument(args, 0), evidence)
		}
	}
	if pos, ok := writeFunctions[name]; ok {
		if args == nil {
			s.add(n, "file-write", Unknown, "filesystem.write", evidence, "writes to a path computed at run time; the writable directories cannot be confirmed")
		} else {
			s.writeFinding(n, name, argument(args, pos), evidence)
		}
	}
}

func (s *scanner) writeFinding(n *frontend.Node, name string, path *frontend.Node, evidence string) {
	if isStringLiteral(path) {
		s.add(n, "file-write", Capability, "filesystem.write", name+"("+path.Text+")", "writes to the file system; the target directory must be granted as writable")
		return
	}
	s.add(n, "file-write", Unknown, "filesystem.write", name+"("+source(path)+")", "writes to a path computed at run time; the writable directories cannot be confirmed")
}

func (s *scanner) include(n *frontend.Node) {
	target := child(n, 0)
	for target != nil && target.Kind == "paren" {
		target = child(target, 0)
	}
	if path, ok := staticPath(target); ok {
		for _, scheme := range remoteSchemes {
			if strings.HasPrefix(strings.ToLower(unquote(path)), scheme) {
				s.add(n, "remote-include", Capability, "network", n.Text+" "+path, "reads source code over the network; requires the network capability")
				return
			}
		}
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

// textOf returns the text of n, or def when n is absent.
func textOf(n *frontend.Node, def string) string {
	if n == nil {
		return def
	}
	return n.Text
}

func isNull(n *frontend.Node) bool {
	return n != nil && n.Kind == "name" && strings.EqualFold(n.Text, "null")
}

// unquote strips the quotes of a simple string literal.
func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
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
