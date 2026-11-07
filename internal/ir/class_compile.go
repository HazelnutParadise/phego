package ir

import (
	"fmt"
	"strings"

	"github.com/HazelnutParadise/phego/internal/frontend"
)

// checkClass checks declaration-local rules with source locations. Linking
// classes whose parents are known is separate from local declaration checks.
func (l *lowerer) checkClass(cl *Class, source *frontend.Node) {
	if cl.Kind == KindEnum {
		if cl.Backing != nil && cl.Backing.zend != "int" && cl.Backing.zend != "string" {
			l.fail(cl.Line, "Enum backing type must be int or string, %s given", cl.Backing.zend)
		}
		for _, c := range cl.Cases {
			if cl.Backing == nil && c.Value != nil {
				l.fail(c.Line, "Case %s of non-backed enum %s must not have a value", c.Name, cl.Name)
			}
			if cl.Backing != nil && c.Value == nil {
				l.fail(c.Line, "Case %s of backed enum %s must have a value", c.Name, cl.Name)
			}
		}
	}
	if cl.Kind == KindEnum && len(cl.Props) != 0 {
		l.fail(sourceMemberLine(source, "property", cl.Props[0].Line), "Enum %s cannot include properties", cl.Name)
	}
	props, methods, constants := map[string]*Prop{}, map[string]*Func{}, map[string]*ClassConst{}
	for _, p := range cl.Props {
		key := p.Name
		if prev := props[key]; prev != nil {
			l.fail(p.Line, "Cannot redeclare %s::$%s", cl.Name, p.Name)
		}
		props[key] = p
		if p.Type != nil {
			for _, bad := range []string{"void", "never", "callable"} {
				if typeContainsAtom(p.Type.zend, bad) {
					l.fail(p.Line, "Property %s::$%s cannot have type %s", cl.Name, p.Name, p.Type.zend)
				}
			}
		}
		propertyReadonly := effectivePropertyReadonly(p)
		if hasModifier(p.Modifiers, "static") && propertyReadonly {
			l.fail(p.Line, "Static property %s::$%s cannot be readonly", cl.Name, p.Name)
		}
		if (p.HasHooks || len(p.Hooks) != 0) && propertyReadonly {
			l.fail(p.Line, "Hooked properties cannot be readonly")
		}
		if hasModifier(p.Modifiers, "abstract") && !p.HasHooks {
			l.fail(p.Line, "Only hooked properties may be declared abstract")
		}
		if hasModifier(p.Modifiers, "final") && hasModifier(p.Modifiers, "private") {
			l.fail(p.Line, "Property cannot be both final and private")
		}
		if cl.Kind == KindInterface {
			if hasModifier(p.Modifiers, "final") {
				l.fail(p.Line, "Property in interface cannot be final")
			}
			if hasModifier(p.Modifiers, "abstract") {
				l.fail(p.Line, "Property in interface cannot be explicitly abstract. All interface members are implicitly abstract")
			}
			if hasModifier(p.Modifiers, "protected") || hasModifier(p.Modifiers, "private") {
				l.fail(p.Line, "Property in interface cannot be protected or private")
			}
		}
		if setVisibilityRank(p.Modifiers) > visibilityRank(p.Modifiers) {
			l.fail(p.Line, "Visibility of property %s::$%s must not be weaker than set visibility", cl.Name, p.Name)
		}
		if hasAsymmetricVisibility(p.Modifiers) && p.Type == nil {
			l.fail(p.Line, "Property with asymmetric visibility %s::$%s must have type", cl.Name, p.Name)
		}
		if propertyReadonly && p.Default != nil && !p.Promoted {
			l.fail(p.Line, "Readonly property %s::$%s cannot have default value", cl.Name, p.Name)
		}
		if p.HasHooks && hasModifier(p.Modifiers, "static") {
			l.fail(p.Line, "Cannot declare hooks for static property")
		}
		if !p.Promoted && p.Type != nil && p.Default != nil && constKind(p.Default) == "null" && !l.typeAllowsNull(p.Type.zend) {
			if strings.Contains(p.Type.zend, "&") {
				l.fail(p.Line, "Cannot use null as default value for property %s::$%s of type %s", cl.Name, p.Name, p.Type.zend)
			}
			if strings.Contains(p.Type.zend, "|") {
				l.fail(p.Line, "Default value for property of type %s may not be null. Use the nullable type %s|null to allow null default value", p.Type.zend, p.Type.zend)
			}
			l.fail(p.Line, "Default value for property of type %s may not be null. Use the nullable type ?%s to allow null default value", p.Type.zend, p.Type.zend)
		}
		if !p.Promoted && p.Type != nil && p.Default != nil {
			kind := constKind(p.Default)
			if kind != "" && kind != "null" && !validDefault(p.Type.zend, p.Default.Value, kind) {
				l.fail(p.Line, "Cannot use %s as default value for property %s::$%s of type %s", kind, cl.Name, p.Name, p.Type.zend)
			}
		}
		seenHooks := map[string]bool{}
		for _, h := range p.Hooks {
			name := strings.ToLower(h.Name)
			if seenHooks[name] {
				l.fail(h.Line, "Cannot redeclare property hook \"%s\"", hookName(h))
			}
			seenHooks[name] = true
			abstractHook := !h.HasBody || hasModifier(h.Modifiers, "abstract")
			if hasModifier(h.Modifiers, "final") && hasModifier(p.Modifiers, "private") {
				l.fail(h.Line, "Property hook cannot be both final and private")
			}
			if abstractHook && hasModifier(p.Modifiers, "private") {
				l.fail(h.Line, "Property hook cannot be both abstract and private")
			}
			if abstractHook && hasModifier(h.Modifiers, "final") {
				l.fail(h.Line, "Property hook cannot be both abstract and final")
			}
			if strings.HasSuffix(name, "::get") && len(h.Params) != 0 {
				l.fail(h.Line, "get hook of property %s::$%s must not have a parameter list", cl.Name, p.Name)
			}
			if strings.HasSuffix(name, "::set") {
				if len(h.Params) > 1 {
					l.fail(h.Line, "set hook of property %s::$%s must accept exactly one parameters", cl.Name, p.Name)
				}
				if len(h.Params) == 1 {
					param := h.Params[0]
					if param.ByRef {
						l.fail(h.Line, "Parameter $%s of set hook %s::$%s must not be pass-by-reference", param.Name, cl.Name, p.Name)
					}
					if param.Variadic {
						l.fail(h.Line, "Parameter $%s of set hook %s::$%s must not be variadic", param.Name, cl.Name, p.Name)
					}
					if param.Default != nil {
						l.fail(h.Line, "Parameter $%s of set hook %s::$%s must not have a default value", param.Name, cl.Name, p.Name)
					}
				}
			}
			if strings.HasSuffix(name, "::get") && len(h.Params) == 0 && h.Return != nil {
				l.fail(h.Line, "get hook of property %s::$%s cannot declare a return type", cl.Name, p.Name)
			}
			if !strings.HasSuffix(name, "::get") && !strings.HasSuffix(name, "::set") {
				l.fail(h.Line, "Unknown hook %q for property %s::$%s, expected \"get\" or \"set\"", h.Name[strings.LastIndex(h.Name, "::")+2:], cl.Name, p.Name)
			}
			if strings.HasSuffix(h.Name, "::set") && len(h.Params) == 1 && ((p.Type == nil) != (h.Params[0].Type == nil) || p.Type != nil && h.Params[0].Type != nil && !l.parameterTypeCompatible(h.Params[0].Type.zend, p.Type.zend)) {
				l.fail(h.Line, "Type of parameter $%s of hook %s::$%s::set must be compatible with property type", h.Params[0].Name, cl.Name, p.Name)
			}
		}
		if cl.Parent == nil {
			l.verifyHookedProperty(cl, p, p.Backed, lastHookLine(p))
		}
		if propertyReadonly && p.Type == nil {
			l.fail(p.Line, "Readonly property %s::$%s must have type", cl.Name, p.Name)
		}
	}
	for _, c := range cl.Consts {
		if strings.EqualFold(c.Name, "class") {
			l.fail(c.Line, "A class constant must not be called 'class'; it is reserved for class name fetching")
		}
		key := c.Name
		if prev := constants[key]; prev != nil {
			l.fail(c.Line, "Cannot redefine class constant %s::%s", cl.Name, c.Name)
		}
		constants[key] = c
		if c.Type != nil {
			switch c.Type.zend {
			case "void", "never", "callable":
				l.fail(c.Line, "Class constant %s::%s cannot have type %s", cl.Name, c.Name, c.Type.zend)
			}
			kind := constKind(c.Value)
			if kind != "" && !validDefault(c.Type.zend, c.Value.Value, kind) {
				l.fail(c.Line, "Cannot use %s as value for class constant %s::%s of type %s", kind, cl.Name, c.Name, c.Type.zend)
			}
		}
		if hasModifier(c.Modifiers, "final") && hasModifier(c.Modifiers, "private") {
			l.fail(c.Line, "Private constant %s::%s cannot be final as it is not visible to other classes", cl.Name, c.Name)
		}
	}
	abstract := 0
	var abstractHookNames []string
	for _, p := range cl.Props {
		for _, h := range p.Hooks {
			if !h.HasBody {
				abstract++
				abstractHookNames = append(abstractHookNames, cl.Name+"::"+h.Name)
			}
		}
	}
	for _, f := range cl.Methods {
		key := strings.ToLower(f.Name)
		if prev := methods[key]; prev != nil {
			l.fail(f.Line, "Cannot redeclare %s::%s()", cl.Name, f.Name)
		}
		methods[key] = f
		if cl.Kind == KindEnum && hasModifier(f.Modifiers, "abstract") {
			l.fail(f.Line, "Enum method %s::%s() must not be abstract", cl.Name, f.Name)
		}
		if cl.Kind == KindInterface {
			if hasModifier(f.Modifiers, "abstract") {
				l.fail(f.Line, "Interface method %s::%s() must not be abstract", cl.Name, f.Name)
			}
			if hasModifier(f.Modifiers, "final") {
				l.fail(f.Line, "Interface method %s::%s() must not be final", cl.Name, f.Name)
			}
		}
		if hasModifier(f.Modifiers, "abstract") {
			abstract++
		}
		if f.Kind == FuncHook && !f.HasBody {
			abstract++
			abstractHookNames = append(abstractHookNames, cl.Name+"::"+f.Name)
		}
		if cl.Kind != KindInterface && !hasModifier(f.Modifiers, "abstract") && !f.HasBody {
			l.fail(f.Line, "Non-abstract method %s::%s() must contain body", cl.Name, f.Name)
		}
		if cl.Kind != KindTrait && hasModifier(f.Modifiers, "abstract") && hasModifier(f.Modifiers, "private") {
			l.fail(f.Line, "Abstract function %s::%s() cannot be declared private", cl.Name, f.Name)
		}
		if cl.Kind == KindClass && hasModifier(f.Modifiers, "abstract") {
			if !hasModifier(cl.Modifiers, "abstract") {
				if cl.Anonymous {
					l.fail(f.Line, "Anonymous class method %s() must not be abstract", f.Name)
				}
				l.fail(f.Line, "Class %s declares abstract method %s() and must therefore be declared abstract", cl.Name, f.Name)
			}
		}
		l.checkMagicMethod(cl, f)
		if hasClassMarker(f.Attributes, "NoDiscard") && (f.Return != nil && (strings.EqualFold(f.Return.zend, "void") || strings.EqualFold(f.Return.zend, "never"))) {
			if strings.EqualFold(f.Return.zend, "void") {
				l.fail(f.Line, "A void method does not return a value, but #[\\NoDiscard] requires a return value")
			}
			l.fail(f.Line, "A never returning method does not return a value, but #[\\NoDiscard] requires a return value")
		}
		if hasModifier(f.Modifiers, "final") && hasModifier(f.Modifiers, "private") && !strings.EqualFold(f.Name, "__construct") {
			l.diag(f.Line, "Warning", "Private methods cannot be final as they are never overridden by other classes")
		}
	}
	if cl.Kind == KindClass && abstract > 0 && !hasModifier(cl.Modifiers, "abstract") {
		if len(abstractHookNames) != 0 {
			l.fail(cl.Line, "Class %s contains %d abstract method%s and must therefore be declared abstract or implement the remaining method%s (%s)", cl.Name, abstract, plural(abstract), plural(abstract), strings.Join(abstractHookNames, ", "))
		}
		l.fail(cl.Line, "Class %s contains %d abstract method%s and must therefore be declared abstract", cl.Name, abstract, plural(abstract))
	}
	for _, r := range cl.TraitRules {
		if r.Modifier != "" && r.Modifier != "public" && r.Modifier != "protected" && r.Modifier != "private" && r.Modifier != "final" {
			l.fail(r.Line, "Cannot use %q as method modifier in trait alias", r.Modifier)
		}
	}
	for _, p := range cl.Props {
		if p.HasHooks && len(p.Hooks) == 0 {
			l.fail(p.Line, "Property hook list must not be empty")
		}
	}
}

func hasAsymmetricVisibility(mods []string) bool {
	return hasModifier(mods, "public(set)") || hasModifier(mods, "protected(set)") || hasModifier(mods, "private(set)")
}

func sourceMemberLine(source *frontend.Node, kind string, fallback int) int {
	for _, m := range source.Children[len(source.Children)-1].Children {
		if m.Kind == kind {
			return m.Line
		}
	}
	return fallback
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func hasPropertyHook(p *Prop, name string) bool {
	for _, h := range p.Hooks {
		if strings.HasSuffix(strings.ToLower(h.Name), "::"+name) {
			return true
		}
	}
	return false
}

func typeContainsAtom(typ, want string) bool {
	for _, clause := range typeDNF(typ) {
		for _, atom := range clause {
			if strings.EqualFold(strings.TrimPrefix(atom, "\\"), want) {
				return true
			}
		}
	}
	return false
}

func (l *lowerer) typeAllowsNull(typ string) bool {
	if strings.EqualFold(typ, "mixed") {
		return true
	}
	for _, clause := range typeDNF(typ) {
		for _, atom := range clause {
			if atom == "null" {
				return true
			}
		}
	}
	return false
}

func (l *lowerer) checkMagicMethod(cl *Class, f *Func) {
	name := strings.ToLower(f.Name)
	display := cl.Name + "::" + f.Name + "()"
	if !strings.HasPrefix(name, "__") {
		return
	}
	params := f.Params
	paramType := func(i int) string {
		if i >= len(params) || params[i].Type == nil {
			return ""
		}
		return params[i].Type.zend
	}
	returnType := ""
	if f.Return != nil {
		returnType = f.Return.zend
	}
	checkArgs := func(want int) {
		if len(params) != want {
			if want == 0 {
				l.fail(f.Line, "Method %s::%s() cannot take arguments", cl.Name, f.Name)
			}
			l.fail(f.Line, "Method %s::%s() must take exactly %d argument%s", cl.Name, f.Name, want, plural(want))
		}
		for _, p := range params {
			if p.ByRef {
				l.fail(f.Line, "Method %s::%s() cannot take arguments by reference", cl.Name, f.Name)
			}
		}
	}
	checkPublic := func() {
		if hasModifier(f.Modifiers, "private") || hasModifier(f.Modifiers, "protected") {
			l.diag(f.Line, "Warning", fmt.Sprintf("The magic method %s must have public visibility", display))
		}
	}
	checkStatic := func(want bool) {
		if f.Static != want {
			if want {
				l.fail(f.Line, "Method %s::%s() must be static", cl.Name, f.Name)
			}
			l.fail(f.Line, "Method %s::%s() cannot be static", cl.Name, f.Name)
		}
	}
	checkNoReturn := func() {
		if f.Return != nil {
			l.fail(f.Line, "Method %s::%s() cannot declare a return type", cl.Name, f.Name)
		}
		if hasClassMarker(f.Attributes, "NoDiscard") {
			l.fail(f.Line, "Method %s::%s cannot be #[\\NoDiscard]", cl.Name, f.Name)
		}
	}
	checkParam := func(i int, want string) {
		if got := paramType(i); got != "" && !l.parameterTypeCompatible(got, want) {
			p := params[i]
			l.fail(f.Line, "%s: Parameter #%d ($%s) must be of type %s when declared", display, i+1, p.Name, want)
		}
	}
	checkReturn := func(want string) {
		if hasClassMarker(f.Attributes, "NoDiscard") && want == "void" {
			l.fail(f.Line, "Method %s::%s cannot be #[\\NoDiscard]", cl.Name, f.Name)
		}
		if returnType != "" && !magicReturnAllowed(returnType, want) {
			l.fail(f.Line, "%s: Return type must be %s when declared", display, want)
		}
	}
	switch name {
	case "__construct":
		checkStatic(false)
		checkNoReturn()
	case "__destruct":
		checkArgs(0)
		checkStatic(false)
		checkNoReturn()
	case "__clone":
		checkArgs(0)
		checkStatic(false)
		checkReturn("void")
	case "__get":
		checkArgs(1)
		checkStatic(false)
		checkPublic()
		checkParam(0, "string")
	case "__set":
		checkArgs(2)
		checkStatic(false)
		checkPublic()
		checkParam(0, "string")
		checkReturn("void")
	case "__isset":
		checkArgs(1)
		checkStatic(false)
		checkPublic()
		checkParam(0, "string")
		checkReturn("bool")
	case "__unset":
		checkArgs(1)
		checkStatic(false)
		checkPublic()
		checkParam(0, "string")
		checkReturn("void")
	case "__call", "__callstatic":
		checkArgs(2)
		checkStatic(name == "__callstatic")
		checkPublic()
		checkParam(0, "string")
		checkParam(1, "array")
	case "__tostring":
		checkArgs(0)
		checkStatic(false)
		checkPublic()
		checkReturn("string")
	case "__debuginfo":
		checkArgs(0)
		checkStatic(false)
		checkPublic()
		checkReturn("?array")
	case "__sleep":
		checkArgs(0)
		checkStatic(false)
		checkPublic()
		checkReturn("array")
	case "__unserialize":
		checkArgs(1)
		checkStatic(false)
		checkPublic()
		checkParam(0, "array")
		checkReturn("void")
	case "__serialize":
		checkArgs(0)
		checkStatic(false)
		checkPublic()
		checkReturn("array")
	case "__set_state":
		checkArgs(1)
		checkStatic(true)
		checkPublic()
		checkParam(0, "array")
		checkReturn("object")
	case "__wakeup":
		checkArgs(0)
		checkStatic(false)
		checkPublic()
		checkReturn("void")
	case "__invoke":
		checkStatic(false)
		checkPublic()
	}
}

func lastHookLine(p *Prop) int {
	if len(p.Hooks) != 0 {
		return p.Hooks[len(p.Hooks)-1].Line
	}
	return p.Line
}

func (l *lowerer) verifyHookedProperty(cl *Class, p *Prop, backed bool, line int) {
	if !p.HasHooks {
		return
	}
	if !backed && p.Default != nil && !p.Promoted {
		l.fail(line, "Cannot specify default value for virtual hooked property %s::$%s", cl.Name, p.Name)
	}
	abstractError := hasModifier(p.Modifiers, "abstract")
	for _, h := range p.Hooks {
		if hookName(h) == "get" && h.ByRef && backed && hasPropertyHook(p, "set") {
			l.fail(line, "Get hook of backed property %s::%s with set hook may not return by reference", cl.Name, p.Name)
		}
		if !h.HasBody {
			abstractError = false
		}
	}
	if abstractError {
		l.fail(line, "Abstract property %s::$%s must specify at least one abstract hook", cl.Name, p.Name)
	}
	if !backed && hasAsymmetricVisibility(p.Modifiers) && (!hasPropertyHook(p, "get") || !hasPropertyHook(p, "set")) {
		prefix := "Read-only"
		if !hasPropertyHook(p, "get") {
			prefix = "Write-only"
		}
		l.fail(line, "%s virtual property %s::$%s must not specify asymmetric visibility", prefix, cl.Name, p.Name)
	}
}

// magicReturnAllowed mirrors zend_check_magic_method_return_type: never is
// always legal, every builtin type must be one the method may return, and a
// class type, including static, is allowed only where an object is required.
func magicReturnAllowed(declared, required string) bool {
	allowed := map[string]bool{}
	for _, clause := range typeDNF(required) {
		allowed[clause[0]] = true
	}
	for _, clause := range typeDNF(declared) {
		if len(clause) > 1 {
			if !allowed["object"] {
				return false
			}
			continue
		}
		switch atom := strings.TrimPrefix(clause[0], "\\"); atom {
		case "never":
			return true
		case "bool":
			if !allowed["bool"] {
				return false
			}
		case "true", "false":
			if !allowed[atom] && !allowed["bool"] {
				return false
			}
		case "null", "int", "float", "string", "array", "object", "callable", "void", "mixed", "iterable":
			// mixed and iterable add builtin bits no magic method allows alone.
			if !allowed[atom] || atom == "mixed" || atom == "iterable" {
				return false
			}
		default:
			if !allowed["object"] {
				return false
			}
		}
	}
	return true
}
