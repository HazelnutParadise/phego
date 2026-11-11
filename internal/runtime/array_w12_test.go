package runtime

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

// Original differential fixtures: REQ-007/016/017; T-013/014/031/032/033/034.
// The operation input is supplied by PHP; all observations here use Go only.
func w12Encode(v value) any {
	v = v.deref()
	switch v.kind {
	case valueNull, valueUndefined:
		return []any{"n"}
	case valueBool:
		return []any{"b", v.boolean}
	case valueInt:
		return []any{"i", strconv.FormatInt(v.integer, 10)}
	case valueFloat:
		var bits [8]byte
		binary.BigEndian.PutUint64(bits[:], math.Float64bits(v.floating))
		return []any{"f", hex.EncodeToString(bits[:])}
	case valueString:
		return []any{"s", hex.EncodeToString([]byte(v.text))}
	case valueArray:
		entries := []any{}
		pos := 0
		for b, ok := v.array.next(&pos); ok; b, ok = v.array.next(&pos) {
			k := w12Int(b.key.integer)
			if b.key.isString {
				k = str(b.key.text)
			}
			entries = append(entries, []any{w12Encode(k), w12Encode(b.val)})
		}
		return []any{"a", entries}
	}
	panic("unexpected fixture value")
}

func w12Input(x any) any {
	a := x.([]any)
	if len(a) > 0 {
		if tag, ok := a[0].(string); ok {
			switch tag {
			case "n":
				return nil
			case "b":
				return a[1].(bool)
			case "i":
				i, err := strconv.ParseInt(a[1].(string), 10, 64)
				if err != nil {
					panic(err)
				}
				return i
			case "s":
				b, err := hex.DecodeString(a[1].(string))
				if err != nil {
					panic(err)
				}
				return string(b)
			}
		}
	}
	out := make([]any, len(a))
	for i, v := range a {
		out[i] = w12Input(v)
	}
	return out
}

type w12Machine struct {
	h            *heap
	vars         map[string]*value
	events       []any
	emittedDiags int
}

func (m *w12Machine) slot(name any) *value {
	s := name.(string)
	if m.vars[s] == nil {
		m.vars[s] = new(value)
	}
	return m.vars[s]
}
func (m *w12Machine) val(x any) value {
	switch x := x.(type) {
	case nil:
		return nullValue()
	case bool:
		return boolValue(x)
	case int64:
		return w12Int(x)
	case string:
		return str(x)
	case []any:
		switch x[0] {
		case "var":
			return *m.slot(x[1])
		case "max":
			return w12Int(maxInt(m.h.width))
		case "min":
			return w12Int(minInt(m.h.width))
		case "array":
			return m.h.newArrayValue()
		case "object":
			o, err := m.h.newObject(&class{name: "stdClass"})
			if err != nil {
				panic(err)
			}
			return o
		case "float":
			var f float64
			switch x[1] {
			case "inf":
				f = math.Inf(1)
			case "-inf":
				f = math.Inf(-1)
			case "nan":
				f = math.NaN()
			default:
				var err error
				f, err = strconv.ParseFloat(x[1].(string), 64)
				if err != nil {
					panic(err)
				}
			}
			return floatValue(f)
		}
	}
	panic(fmt.Sprintf("bad fixture input %#v", x))
}
func (m *w12Machine) run(ops []any, iteration int) error {
	for _, raw := range ops {
		o := raw.([]any)
		var err error
		switch o[0] {
		case "set":
			err = m.h.setElement(m.slot(o[1]), m.val(o[2]), m.val(o[3]))
		case "append":
			err = m.h.appendValue(m.slot(o[1]), m.val(o[2]))
		case "assign":
			err = m.h.assign(m.slot(o[1]), m.val(o[2]))
		case "copy":
			err = m.h.assign(m.slot(o[1]), *m.slot(o[2]))
		case "bind":
			err = m.h.bind(m.slot(o[1]), m.slot(o[2]))
		case "bind_element":
			var p *value
			p, err = m.h.element(m.slot(o[2]), m.val(o[3]))
			if err == nil {
				err = m.h.bind(m.slot(o[1]), p)
			}
		case "unset":
			err = m.h.unset(m.slot(o[1]))
		case "delete":
			err = m.h.unsetElement(m.slot(o[1]), m.val(o[2]))
		case "get":
			var v value
			v, _, err = m.h.getElement(*m.slot(o[1]), m.val(o[2]))
			if err == nil {
				m.emit(v)
			}
		case "emit":
			m.emit(*m.slot(o[1]))
		case "isset_var":
			k := m.slot(o[1]).deref().kind
			m.emit(boolValue(k != valueUndefined && k != valueNull))
		case "at":
			if int(o[1].(int64)) == iteration {
				err = m.run(o[2].([]any), iteration)
			}
		case "pointer":
			if m.slot(o[1]).deref().kind == valueUndefined {
				err = m.h.assign(m.slot(o[1]), m.h.newArrayValue())
			}
			if err == nil {
				var v value
				v, err = m.h.arrayPointer(m.slot(o[1]), o[2].(string))
				if err == nil {
					m.emit(v)
				}
			}
		case "sort_trace":
			err = m.sortTrace(o)
		case "sort_recursive":
			err = m.sortRecursive(o)
		case "sort":
			once := false
			err = m.h.sortArray(m.slot(o[1]), o[2].(string), o[2] == "uasort", func(a, b value) (int, error) {
				if o[3] == "unset" && !once {
					once = true
					if err := m.h.unset(m.slot(o[1])); err != nil {
						return 0, err
					}
				}
				if o[3] == "rebind" && !once {
					once = true
					if err := m.h.bind(m.slot(o[1]), m.slot("replacement")); err != nil {
						return 0, err
					}
				}
				if o[3] == "throw" {
					if err := m.h.appendValue(m.slot(o[1]), w12Int(99)); err != nil {
						return 0, err
					}
					return 0, &Throwable{"Exception", "comparison failed"}
				}
				if o[3] == true && !once {
					once = true
					if err := m.h.appendValue(m.slot(o[1]), w12Int(99)); err != nil {
						return 0, err
					}
				}
				return m.h.compare(a, b)
			})
		case "foreach_ref", "foreach_value":
			i := 0
			body := func(arrayKey) error {
				if i > 32 {
					return fmt.Errorf("fixture iteration bound")
				}
				n := i
				i++
				return m.run(o[3].([]any), n)
			}
			if o[0] == "foreach_ref" {
				err = m.h.foreachRef(m.slot(o[1]), m.slot(o[2]), body)
			} else {
				err = m.h.foreachValue(*m.slot(o[1]), m.slot(o[2]), body)
			}
		default:
			return fmt.Errorf("unknown operation %v", o[0])
		}
		if err != nil {
			class, msg := "Error", err.Error()
			var th *Throwable
			if errors.As(err, &th) {
				class, msg = th.Class, th.Message
			}
			m.events = append(m.events, []any{"error", class, msg})
		}
		for _, d := range m.h.diagnostics[m.emittedDiags:] {
			m.events = append(m.events, []any{"diagnostic", d.level, d.message})
		}
		m.emittedDiags = len(m.h.diagnostics)
	}
	return nil
}
func (m *w12Machine) emit(v value) { m.events = append(m.events, []any{"value", w12Encode(v)}) }

func TestW12PHP8512Arrays(t *testing.T) {
	// amd64 runs on the 64-bit reference: CI regenerates PHP 8.5.12 on
	// native amd64 and requires byte-identical output before this test.
	for _, profile := range []struct {
		name, arch string
		width      int
	}{{"32", "386", 32}, {"64", "arm64", 64}, {"amd64", "amd64", 64}} {
		width := profile.width
		t.Run(profile.name, func(t *testing.T) {
			dir := os.Getenv("PHEGO_W12_REFERENCE")
			if dir == "" {
				dir = "testdata"
			}
			data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("w12-ref-%d.json", width)))
			if err != nil {
				t.Fatal(err)
			}
			var ref struct {
				PHP     string `json:"php"`
				IntSize int    `json:"int_size"`
				Cases   []struct {
					Name       string `json:"name"`
					Operations []any  `json:"operations"`
					Events     []any  `json:"events"`
				} `json:"cases"`
			}
			if err := json.Unmarshal(data, &ref); err != nil {
				t.Fatal(err)
			}
			if ref.PHP != "8.5.12" || ref.IntSize*8 != width || len(ref.Cases) != 650 {
				t.Fatal("invalid reference profile or missing cases")
			}
			for _, c := range ref.Cases {
				t.Run(c.Name, func(t *testing.T) {
					h, err := newHeapFor(width, profile.arch, "linux")
					if err != nil {
						t.Fatal(err)
					}
					m := w12Machine{h: h, vars: map[string]*value{}, events: []any{}}
					if err := m.run(w12Input(c.Operations).([]any), -1); err != nil {
						t.Fatal(err)
					}
					gotJSON, err := json.Marshal(m.events)
					if err != nil {
						t.Fatal(err)
					}
					var got []any
					if err := json.Unmarshal(gotJSON, &got); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, c.Events) {
						want, _ := json.Marshal(c.Events)
						t.Errorf("Go: %s\nPHP: %s", gotJSON, want)
					}
				})
			}
		})
	}
}

func w12Int(i int64) value { return value{kind: valueInt, integer: i} }
