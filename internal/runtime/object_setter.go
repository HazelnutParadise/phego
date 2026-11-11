package runtime

import "strings"

// REQ-018/020; T-035/036/039/040. Setter arguments are passed by value and
// validated independently of the backing property's declared type.
func (h *heap) prepareSetterArgument(o *object, p *propertyInfo, v value, strict bool, site methodCallSite) (value, error) {
	parameter, explicit := p.owner.schema.setterParameters[p.name]
	if !explicit {
		parameter = hookParameter{name: "value", typeName: p.typeName}
	}
	m := methodSpec{owner: p.owner}
	converted, ok, err := h.coerceTypedValue(&m, o.class, parameter.typeName, v, strict)
	if err != nil {
		return value{}, err
	}
	if !ok {
		m.name = "$" + p.name + "::set"
		argument := methodParameter{name: parameter.name, typeName: setterArgumentTypeName(parameter.typeName, p.owner)}
		return value{}, h.methodArgumentTypeError(&m, &argument, 0, v, site)
	}
	if err := h.revalidatePropertyReceiver(o, p); err != nil {
		return value{}, err
	}
	return converted, nil
}

// PHP displays named types before the builtin mask, independently of source
// union order. This is an error-path view; the immutable coercion type stays
// unchanged. Declaration legality belongs to the frontend.
func setterArgumentTypeName(typ string, owner *class) string {
	var named []string
	builtins := make(map[string]bool)
	if strings.HasPrefix(typ, "?") {
		typ = typ[1:] + "|null"
	}
	const order = "static|callable|object|array|string|int|float|bool|false|true|void|never|null"
	for part := range splitMethodType(typ, '|') {
		part = strings.TrimSpace(part)
		lower := phpNameLower(part)
		if lower == "mixed" {
			return "mixed"
		}
		if lower == "iterable" {
			named = append(named, "Traversable")
			builtins["array"] = true
			continue
		}
		if methodTypeHas(order, lower) {
			builtins[lower] = true
			continue
		}
		intersection := strings.TrimSuffix(strings.TrimPrefix(part, "("), ")")
		var names []string
		for name := range splitMethodType(intersection, '&') {
			name = strings.TrimPrefix(strings.TrimSpace(name), "\\")
			if phpNameEqual(name, "self") && owner != nil {
				name = owner.name
			} else if phpNameEqual(name, "parent") && owner != nil && owner.schema.parent != nil {
				name = owner.schema.parent.name
			}
			names = append(names, name)
		}
		named = append(named, strings.Join(names, "&"))
	}
	parts := named
	for name := range splitMethodType(order, '|') {
		if builtins[name] {
			parts = append(parts, name)
		}
	}
	if len(parts) == 2 && parts[1] == "null" && !strings.Contains(parts[0], "&") {
		return "?" + parts[0]
	}
	if len(parts) > 1 {
		for i, part := range parts {
			if strings.Contains(part, "&") {
				parts[i] = "(" + part + ")"
			}
		}
	}
	return strings.Join(parts, "|")
}
