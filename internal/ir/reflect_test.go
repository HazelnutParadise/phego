package ir

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/profile"
)

// reflectParam and the types below mirror the JSON that
// tests/semantics/w11-hir/reflect.php prints from PHP's Reflection API.
type reflectParam struct {
	Name     string  `json:"name"`
	Type     *string `json:"type"`
	ByRef    bool    `json:"byRef"`
	Variadic bool    `json:"variadic"`
	Optional bool    `json:"optional"`
	Default  bool    `json:"default"`
	Promoted bool    `json:"promoted"`
}

type reflectFunc struct {
	Name       string         `json:"name"`
	ReturnType *string        `json:"returnType"`
	ByRef      bool           `json:"byRef"`
	Generator  bool           `json:"generator"`
	Variadic   bool           `json:"variadic"`
	Statics    []string       `json:"statics"`
	Params     []reflectParam `json:"params"`
	Static     *bool          `json:"static,omitempty"`
}

type reflectClass struct {
	Name    string        `json:"name"`
	Methods []reflectFunc `json:"methods"`
}

type reflectDump struct {
	Functions []reflectFunc  `json:"functions"`
	Classes   []reflectClass `json:"classes"`
	Closures  []reflectFunc  `json:"closures"`
	Values    [][2]string    `json:"values"`
}

// TestLowerMatchesReflection compares the names, signatures, captures and
// compile-time values in HIR with what PHP 8.5.12 reports for the same
// fixtures through Reflection. The reference JSON is produced by
// tests/semantics/w11-hir/reflect.php in the pinned image; the fixtures
// are compiled under the path the reference saw them at.
func TestLowerMatchesReflection(t *testing.T) {
	prof, err := profile.Resolve("8.5.12", 64, false)
	if err != nil {
		t.Fatal(err)
	}
	fixtures, err := filepath.Glob("../../tests/semantics/w11-hir/reflect/*.php")
	if err != nil || len(fixtures) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, path := range fixtures {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path[:len(path)-4] + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var want reflectDump
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			tree, err := frontend.Parse(src, frontend.LexOptions{IntWidth: 64, MaxBytes: len(src) + 1})
			if err != nil {
				t.Fatal(err)
			}
			unit, err := Lower(tree, prof, Options{File: "/w/reflect/" + name})
			if err != nil {
				t.Fatal(err)
			}
			got := dumpReflection(unit)
			compare := func(kind string, g, w []any) {
				if len(g) != len(w) {
					t.Errorf("%s: got %d, want %d", kind, len(g), len(w))
				}
				for i := 0; i < len(g) && i < len(w); i++ {
					if !reflect.DeepEqual(g[i], w[i]) {
						gj, _ := json.Marshal(g[i])
						wj, _ := json.Marshal(w[i])
						t.Errorf("%s[%d]:\ngot  %s\nwant %s", kind, i, gj, wj)
					}
				}
			}
			compare("functions", anys(got.Functions), anys(want.Functions))
			compare("classes", anys(got.Classes), anys(want.Classes))
			compare("closures", anys(got.Closures), anys(want.Closures))
			compare("values", anys(got.Values), anys(want.Values))
		})
	}
}

func anys[T any](s []T) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

func dumpReflection(u *Unit) reflectDump {
	var d reflectDump
	for _, f := range u.Functions {
		d.Functions = append(d.Functions, reflectFunction(f))
	}
	for _, c := range u.Classes {
		rc := reflectClass{Name: c.Name, Methods: []reflectFunc{}}
		for _, m := range c.Methods {
			rf := reflectFunction(m)
			static := m.Static
			rf.Static = &static
			rc.Methods = append(rc.Methods, rf)
		}
		d.Classes = append(d.Classes, rc)
	}
	// $closures[] = ..., $GLOBALS['closures'][] = ... and the same for
	// $values, in source order, as the fixtures build them.
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Op == OpAssign {
			if list := appendTarget(n.Kids[0]); list != "" {
				v := n.Kids[1]
				switch {
				case list == "closures" && (v.Op == OpClosure || v.Op == OpArrowFn):
					rf := reflectFunction(v.Func)
					static := v.Func.Static
					rf.Static = &static
					d.Closures = append(d.Closures, rf)
				case list == "values":
					d.Values = append(d.Values, reflectValue(v))
				}
			}
		}
		for _, k := range n.Kids {
			walk(k)
		}
		if n.Op == OpClosure || n.Op == OpArrowFn || n.Op == OpFuncDecl {
			for _, s := range n.Func.Body {
				walk(s)
			}
		}
	}
	var walkFunc func(f *Func)
	walkFunc = func(f *Func) {
		for _, s := range f.Body {
			walk(s)
		}
	}
	for _, f := range u.Functions {
		walkFunc(f)
	}
	for _, c := range u.Classes {
		for _, m := range c.Methods {
			walkFunc(m)
		}
	}
	walkFunc(u.Main)
	return d
}

// appendTarget names the list an assignment appends to, or "".
func appendTarget(n *Node) string {
	if n.Op != OpDim || n.Kids[1] != nil {
		return ""
	}
	base := n.Kids[0]
	switch {
	case base.Op == OpVar:
		return base.Var.Name
	case base.Op == OpDim && base.Kids[0].Op == OpSuperGlobal && base.Kids[0].Text == "GLOBALS" && lit(base.Kids[1]) != nil:
		return lit(base.Kids[1]).Str
	}
	return ""
}

func reflectValue(n *Node) [2]string {
	v := lit(n)
	if v == nil {
		return [2]string{"other", string(n.Op)}
	}
	switch v.Kind {
	case LitString:
		return [2]string{"string", hex.EncodeToString([]byte(v.Str))}
	case LitInt:
		return [2]string{"int", strconv.FormatInt(v.Int, 10)}
	case LitFloat:
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], math.Float64bits(v.Float))
		return [2]string{"float", hex.EncodeToString(b[:])}
	case LitBool:
		return [2]string{"bool", strconv.FormatBool(v.Bool)}
	}
	return [2]string{"null", ""}
}

func reflectFunction(f *Func) reflectFunc {
	rf := reflectFunc{Name: f.Name, ByRef: f.ByRef, Generator: f.Generator, Variadic: f.Variadic, Statics: []string{}, Params: []reflectParam{}}
	if f.Return != nil {
		s := f.Return.String
		rf.ReturnType = &s
	}
	for _, c := range f.Captures {
		// PHP binds an implicit capture only when the defining scope has
		// the variable; the fixtures define every variable they assign.
		if c.Implicit {
			if v := f.Parent.Var(c.Name); v == nil || v.Flags&(VarWritten|VarParam|VarCaptured) == 0 {
				continue
			}
		}
		rf.Statics = append(rf.Statics, c.Name)
	}
	for _, v := range f.Vars {
		if v.Flags&VarStatic != 0 {
			rf.Statics = append(rf.Statics, v.Name)
		}
	}
	required := -1
	for i, p := range f.Params {
		if p.Default == nil && !p.Variadic {
			required = i
		}
	}
	for i, p := range f.Params {
		rp := reflectParam{Name: p.Name, ByRef: p.ByRef, Variadic: p.Variadic, Default: p.Default != nil, Promoted: len(p.Promoted) > 0}
		rp.Optional = p.Variadic || p.Default != nil && i > required
		if p.Type != nil {
			s := p.Type.String
			rp.Type = &s
		}
		rf.Params = append(rf.Params, rp)
	}
	return rf
}
