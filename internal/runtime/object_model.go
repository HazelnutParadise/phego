package runtime

import (
	"fmt"
	"maps"
	"slices"
)

// REQ-012/018/020/021; T-023/024/035/036/039-042.
// Descriptions are immutable after buildClass. Callbacks contain program code;
// the heap argument supplies request state. They are trusted host code.
type access uint8

const (
	publicAccess access = iota
	protectedAccess
	privateAccess
)

type objectHook func(*heap, value, value) (value, error)
type hookParameter struct {
	name, typeName string
}

type propertySpec struct {
	name, typeName string
	visibility     access
	setVisibility  access
	readonly       bool
	static         bool
	virtual        bool
	hasDefault     bool
	initial        any // first build copies the snapshot; trait imports share it immutably
	get, set       objectHook
	hookStrict     bool
	getReference   bool
}

type methodSpec struct {
	owner            *class
	parameters       []methodParameter
	returnType       string
	returnStrict     bool // strict_types belongs to the declaring source file
	returnsReference bool
	name             string
	visibility       access
	static           bool
	final            bool
	abstract         bool
	call             func(*heap, value, []value) (value, error)
}

type classSpec struct {
	name                                  string
	parent                                *class
	interfaces, traits                    []*class
	properties                            []propertySpec
	methods                               []methodSpec
	setterParameters                      map[string]hookParameter
	final, abstract, isInterface, isTrait bool
	readonly, allowDynamic                bool
	noDynamic                             bool
	enumKind                              enumKind
	enumCases                             []string
	enumValues                            []any
	destructor                            func(*heap, value) error
}

type enumKind uint8

const (
	enumNone enumKind = iota
	enumUnit
	enumInt
	enumString
)

type propertyInfo struct {
	propertySpec
	owner          *class
	key            string
	prototypeOwner *class
}

type classSchema struct {
	classSpec
	props   []propertyInfo
	layout  *propertyLayout
	methods map[string]*methodSpec // descriptors are frozen after class construction
}

type lazyObject struct {
	initialize  func(*heap, value) (value, error)
	initializer value // logical callable owner, supplied by the language adapter
	proxy       bool
	running     bool
	target      value
	nonLazy     map[string]struct{} // raw operations mark locations, not value payloads
	remaining   int                 // running proxy raw writes clear flags without decrementing
}

type propertySource struct {
	slot *value
	info *propertyInfo
}

func buildClass(spec classSpec) (*class, error) {
	if spec.name == "" {
		return nil, fmt.Errorf("class name is required")
	}
	if err := normalizeEnumSpec(&spec); err != nil {
		return nil, err
	}
	if spec.parent != nil && spec.parent.schema != nil && spec.parent.schema.final {
		return nil, &Throwable{"Error", "Class " + spec.name + " cannot extend final class " + spec.parent.name}
	}
	c := &class{name: spec.name, destructor: spec.destructor}
	s := &classSchema{classSpec: spec, methods: make(map[string]*methodSpec)}
	c.schema = s
	if spec.destructor != nil {
		c.destructorOwner = c
		destroy := spec.destructor
		c.destructor = func(h *heap, this value) error {
			_, err := h.invokeObjectInScope(func() (value, error) { return nullValue(), destroy(h, this) }, false, this.object.class)
			return err
		}
	}
	s.interfaces = slices.Clone(spec.interfaces)
	s.traits = slices.Clone(spec.traits)
	s.properties = nil
	s.classSpec.methods = nil
	s.setterParameters = nil
	if len(spec.setterParameters) != 0 {
		s.setterParameters = maps.Clone(spec.setterParameters)
	}
	for name, parameter := range s.setterParameters {
		if parameter.name == "" || !slices.ContainsFunc(spec.properties, func(p propertySpec) bool { return p.name == name && p.set != nil }) {
			return nil, fmt.Errorf("invalid setter parameter for %s::$%s", spec.name, name)
		}
	}
	s.enumCases = slices.Clone(spec.enumCases)
	s.enumValues = slices.Clone(spec.enumValues)
	for i, v := range s.enumValues {
		var err error
		s.enumValues[i], err = copyObjectDefault(v, 0)
		if err != nil {
			return nil, err
		}
	}
	if spec.parent != nil && spec.parent.schema != nil {
		s.props = slices.Clone(spec.parent.schema.props)
		maps.Copy(s.methods, spec.parent.schema.methods)
		if c.destructor == nil {
			c.destructor = spec.parent.destructor
			c.destructorOwner = spec.parent.destructorOwner
		}
		s.allowDynamic = s.allowDynamic || spec.parent.schema.allowDynamic
	}
	properties := slices.Clone(spec.properties)
	var methods []methodSpec
	for _, trait := range spec.traits {
		if trait == nil || trait.schema == nil || !trait.schema.isTrait {
			return nil, fmt.Errorf("invalid trait")
		}
		for _, p := range trait.schema.props {
			properties = append(properties, p.propertySpec)
			if parameter, ok := p.owner.schema.setterParameters[p.name]; ok {
				if _, exists := s.setterParameters[p.name]; !exists {
					if s.setterParameters == nil {
						s.setterParameters = make(map[string]hookParameter)
					}
					s.setterParameters[p.name] = parameter
				}
			}
		}
		for _, name := range slices.Sorted(maps.Keys(trait.schema.methods)) {
			if slices.ContainsFunc(spec.methods, func(m methodSpec) bool { return phpNameEqual(m.name, name) }) {
				continue
			}
			methods = append(methods, *trait.schema.methods[name])
		}
	}
	traitEnd := len(properties)
	traitMethodCount := len(methods)
	methods = append(methods, spec.methods...)
	if spec.enumKind != enumNone {
		if len(properties) != 0 {
			return nil, fmt.Errorf("enum cannot declare properties")
		}
		properties = append(properties, propertySpec{name: "name", typeName: "string", readonly: true})
		if spec.enumKind != enumUnit {
			typ := "string"
			if spec.enumKind == enumInt {
				typ = "int"
			}
			properties = append(properties, propertySpec{name: "value", typeName: typ, readonly: true})
		}
	}
	for propertyIndex, p := range properties {
		fromTrait := propertyIndex >= len(spec.properties) && propertyIndex < traitEnd
		if p.name == "" {
			return nil, fmt.Errorf("property name is required")
		}
		if s.readonly {
			if fromTrait && !p.readonly {
				return nil, fmt.Errorf("readonly class cannot import non-readonly trait property %s", p.name)
			}
			p.readonly = true
		}
		if p.readonly && p.setVisibility == publicAccess {
			p.setVisibility = protectedAccess
		}
		if p.hasDefault {
			if integer, ok := p.initial.(int64); ok && methodTypeHas(p.typeName, "float") && !methodTypeHas(p.typeName, "int") {
				p.initial = float64(integer)
			}
			if !fromTrait {
				var err error
				p.initial, err = copyObjectDefault(p.initial, 0)
				if err != nil {
					return nil, err
				}
			}
		}
		key := p.name
		if p.visibility == privateAccess {
			key = "\x00" + spec.name + "\x00" + key
		} else if p.visibility == protectedAccess {
			key = "\x00*\x00" + key
		}
		info := propertyInfo{propertySpec: p, owner: c, key: key, prototypeOwner: c}
		replaced := false
		for i, old := range s.props {
			if fromTrait && old.name == p.name && old.owner == c {
				if !compatibleTraitProperty(old, info) {
					return nil, fmt.Errorf("incompatible trait property %s::$%s", spec.name, p.name)
				}
				replaced = true
				break
			}
			if old.name == p.name && old.visibility != privateAccess {
				if p.visibility > old.visibility {
					return nil, &Throwable{"Error", "Access level to " + spec.name + "::$" + p.name + " must be " + accessName(old.visibility) + " (as in class " + old.owner.name + ")"}
				}
				if !old.owner.schema.isInterface {
					info.prototypeOwner = old.prototypeOwner
				}
				s.props[i] = info
				replaced = true
				break
			}
		}
		if !replaced {
			s.props = append(s.props, info)
		}
	}
	for methodIndex, m := range methods {
		if phpNameEqual(m.name, "__toString") && m.returnType == "" {
			m.returnType = "string"
		}
		m.owner = c
		if methodIndex >= traitMethodCount {
			m.parameters = slices.Clone(m.parameters)
			for i := range m.parameters {
				if m.parameters[i].hasDefault {
					var err error
					m.parameters[i].initial, err = copyObjectDefault(m.parameters[i].initial, 0)
					if err != nil {
						return nil, err
					}
				}
			}
		}
		key := phpNameLower(m.name)
		if old, ok := s.methods[key]; ok && old.final {
			return nil, &Throwable{"Error", "Cannot override final method " + old.owner.name + "::" + m.name + "()"}
		}
		s.methods[key] = &m
	}
	var names []string
	addIntrinsicInterfaces(c)
	for _, p := range s.props {
		if !p.static && !p.virtual {
			names = append(names, p.key)
		}
	}
	s.layout = newPropertyLayout(names)
	return c, nil
}

func copyObjectDefault(input any, depth int) (any, error) {
	if depth > 1024 {
		return nil, fmt.Errorf("class default exceeds depth limit")
	}
	switch v := input.(type) {
	case nil, bool, int64, float64:
		return v, nil
	case []byte:
		return slices.Clone(v), nil
	case []SnapshotEntry:
		out := slices.Clone(v)
		for i := range out {
			var err error
			out[i].Value, err = copyObjectDefault(out[i].Value, depth+1)
			if err != nil {
				return nil, err
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("invalid class default %T", input)
}

func (h *heap) initializeProperties(o *object) error {
	for _, p := range o.class.schema.props {
		if p.static || p.virtual {
			continue
		}
		slot := o.props.slot(p.key)
		if o.lazy != nil {
			if _, nonLazy := o.lazy.nonLazy[p.key]; nonLazy {
				continue
			}
		}
		if p.hasDefault {
			v, err := h.importSnapshotPHP8512(p.initial, 1024, 1<<20)
			if err != nil {
				return err
			}
			*slot = v
		} else if p.typeName == "" {
			*slot = nullValue()
		}
	}
	return nil
}

func (h *heap) instantiate(c *class) (value, error) {
	if c == nil {
		return value{}, fmt.Errorf("class is required")
	}
	if c == weakMapBuiltin {
		return h.newWeakMapObject()
	}
	if c.schema != nil {
		if err := h.registerClass(c); err != nil {
			return value{}, err
		}
	}
	if s := c.schema; s != nil {
		kind := ""
		if s.isInterface {
			kind = "interface"
		} else if s.isTrait {
			kind = "trait"
		} else if s.abstract {
			kind = "abstract class"
		} else if s.enumKind != enumNone {
			kind = "enum"
		}
		if kind != "" {
			return value{}, &Throwable{"Error", "Cannot instantiate " + kind + " " + c.name}
		}
	}
	return h.newObject(c)
}

// Class names and aliases are resolved within a request; descriptors are shared.
func (h *heap) registerClass(c *class) error {
	if err := h.runtimeCheck(); err != nil {
		return err
	}
	key := phpNameLower(c.name)
	if old := h.classes[key]; old != nil {
		if old == c {
			// Only a fully validated immutable chain is ever published.
			return nil
		}
		return &Throwable{"Error", "Cannot declare class " + c.name + ", because the name is already in use"}
	}
	pending := make(map[string]*class)
	for node := c; node != nil; {
		if err := h.runtimeCheck(); err != nil {
			return err
		}
		name := phpNameLower(node.name)
		if previous := pending[name]; previous != nil {
			if previous == node {
				return fmt.Errorf("cyclic class inheritance")
			}
			return &Throwable{"Error", "Cannot declare class " + node.name + ", because the name is already in use"}
		}
		if old := h.classes[name]; old != nil {
			if old != node {
				return &Throwable{"Error", "Cannot declare class " + node.name + ", because the name is already in use"}
			}
			// Published immutable ancestors have already been fully validated.
			break
		}
		pending[name] = node
		if node.schema == nil {
			break
		}
		node = node.schema.parent
	}
	if h.classes == nil {
		h.classes = make(map[string]*class, len(pending))
	}
	maps.Copy(h.classes, pending)
	return nil
}

func accessName(v access) string {
	switch v {
	case privateAccess:
		return "private"
	case protectedAccess:
		return "protected"
	}
	return "public"
}
func (p *propertyInfo) label() string { return p.owner.name + "::$" + p.name }
func classIs(c, want *class) bool {
	for c != nil {
		if c == want {
			return true
		}
		if c.schema == nil {
			return false
		}
		for _, i := range c.schema.interfaces {
			if classIs(i, want) {
				return true
			}
		}
		c = c.schema.parent
	}
	return false
}
func canAccess(scope, owner *class, visibility access) bool {
	return visibility == publicAccess || scope == owner || (visibility == protectedAccess && scope != nil && (classIs(scope, owner) || classIs(owner, scope)))
}

func (h *heap) checkedObject(obj value) (*object, error) {
	if err := h.runtimeCheck(); err != nil {
		return nil, err
	}
	if err := h.claimValues(obj); err != nil {
		return nil, err
	}
	obj = obj.deref()
	if obj.kind != valueObject || obj.object == nil || obj.object.freed || obj.object.owner != h {
		return nil, fmt.Errorf("property access requires a live object owned by this request")
	}
	if err := h.validateObject(obj.object); err != nil {
		return nil, err
	}
	return obj.object, nil
}

func resolveProperty(c *class, name string, scope *class) (*propertyInfo, error) {
	return resolvePropertyIn(c, name, scope, false)
}

func resolvePropertyIn(c *class, name string, scope *class, instance bool) (*propertyInfo, error) {
	if c.schema == nil {
		return nil, nil
	}
	if scope != nil {
		for i := range c.schema.props {
			p := &c.schema.props[i]
			if p.name == name && p.owner == scope && p.visibility == privateAccess {
				return p, nil
			}
		}
	}
	for i := len(c.schema.props) - 1; i >= 0; i-- {
		p := &c.schema.props[i]
		if p.name != name {
			continue
		}
		if instance && p.visibility == privateAccess && p.owner != c {
			continue
		}
		if !canAccess(scope, p.owner, p.visibility) {
			return nil, &Throwable{"Error", "Cannot access " + accessName(p.visibility) + " property " + c.name + "::$" + name}
		}
		return p, nil
	}
	return nil, nil
}

func (h *heap) objectPropertyKeys(obj value) []string {
	o, err := h.checkedObject(obj)
	if err != nil {
		return nil
	}
	out := []string{}
	o.props.each(func(name string, _ *value) { out = append(out, name) })
	return out
}
func (h *heap) reflectObjectProperties(c *class) [][]any {
	out := [][]any{}
	if c != nil && c.schema != nil {
		for _, p := range c.schema.props {
			out = append(out, []any{p.name, p.virtual, p.get != nil, p.set != nil})
		}
	}
	return out
}
