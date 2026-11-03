package runtime

import "fmt"

// Shared intrinsic interfaces contain immutable program metadata only.
var stringableBuiltin = &class{name: "Stringable", schema: &classSchema{classSpec: classSpec{name: "Stringable", isInterface: true}}}
var unitEnumBuiltin = &class{name: "UnitEnum", schema: &classSchema{classSpec: classSpec{name: "UnitEnum", isInterface: true}}}
var backedEnumBuiltin = &class{name: "BackedEnum", schema: &classSchema{classSpec: classSpec{name: "BackedEnum", isInterface: true, interfaces: []*class{unitEnumBuiltin}}}}

func normalizeEnumSpec(spec *classSpec) error {
	if spec.enumKind == enumNone && (len(spec.enumCases) != 0 || len(spec.enumValues) != 0) {
		spec.enumKind = enumUnit
		if len(spec.enumValues) != 0 {
			switch spec.enumValues[0].(type) {
			case int64:
				spec.enumKind = enumInt
			case []byte:
				spec.enumKind = enumString
			default:
				return fmt.Errorf("invalid enum backing type")
			}
		}
	}
	if spec.enumKind > enumString {
		return fmt.Errorf("invalid enum kind")
	}
	if spec.enumKind == enumNone {
		return nil
	}
	if spec.parent != nil || spec.isInterface || spec.isTrait || spec.abstract || spec.readonly {
		return fmt.Errorf("invalid enum class descriptor")
	}
	if spec.enumKind == enumUnit {
		if len(spec.enumValues) != 0 {
			return fmt.Errorf("unit enum cannot have backing values")
		}
	} else {
		if len(spec.enumValues) != len(spec.enumCases) {
			return fmt.Errorf("enum backing value count differs from case count")
		}
		for _, v := range spec.enumValues {
			_, integer := v.(int64)
			_, text := v.([]byte)
			if (spec.enumKind == enumInt && !integer) || (spec.enumKind == enumString && !text) {
				return fmt.Errorf("inconsistent enum backing type")
			}
		}
	}
	spec.final, spec.noDynamic = true, true
	return nil
}

func addIntrinsicInterfaces(c *class) {
	s := c.schema
	if s.methods["__tostring"] != nil && !classMatchesAnyName(c, "Stringable") {
		s.interfaces = append(s.interfaces, stringableBuiltin)
	}
	if s.enumKind != enumNone {
		iface := unitEnumBuiltin
		if s.enumKind != enumUnit {
			iface = backedEnumBuiltin
		}
		if !classMatchesAnyName(c, iface.name) {
			s.interfaces = append(s.interfaces, iface)
		}
	}
}
