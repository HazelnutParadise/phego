package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// vmOracleRow is one observation of testdata/vm-oracle/tables.php.
type vmOracleRow struct {
	R []string `json:"r"`
	E []string `json:"e"`
	D [][2]any `json:"d"`
}

type vmOracleTables struct {
	PHP          string                   `json:"php"`
	IntSize      int                      `json:"int_size"`
	Values       []string                 `json:"values"`
	IncDecValues []string                 `json:"incdec_values"`
	Keys         []string                 `json:"keys"`
	ReadArray    string                   `json:"read_array"`
	Tables       map[string][]vmOracleRow `json:"-"`
}

func loadVMOracle(t *testing.T, width int) vmOracleTables {
	t.Helper()
	raw, err := os.ReadFile(fmt.Sprintf("testdata/vm-oracle/tables-%d.json", width))
	if err != nil {
		t.Fatal(err)
	}
	var o vmOracleTables
	if err := json.Unmarshal(raw, &o); err != nil {
		t.Fatal(err)
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	o.Tables = map[string][]vmOracleRow{}
	for k, v := range all {
		var rows []vmOracleRow
		if json.Unmarshal(v, &rows) == nil {
			o.Tables[k] = rows
		}
	}
	if o.PHP != "8.5.12" || o.IntSize*8 != width {
		t.Fatalf("oracle is PHP %s with %d-byte ints", o.PHP, o.IntSize)
	}
	return o
}

// vmObserve runs build, whose returned register is the result (an array of
// two for pair observations), and records it in the oracle's form. ok is
// false when the VM reports the construct as unsupported.
func vmObserve(t *testing.T, width int, pair bool, locals []string, build func(m *vmFn) uint32) (row vmOracleRow, unsupported bool) {
	t.Helper()
	a := newAsm(t, width)
	m := a.fn("{main}", 0, 2, locals...)
	m.at(vmOpLine)
	r := build(m)
	m.at(9).ret(r)
	cfg := vmConfig(t, width)
	cfg.DisplayErrors = false
	res := a.program().Run(cfg)
	switch res.End {
	case Completed:
		if pair {
			entries, ok := res.Return.([]SnapshotEntry)
			if !ok || len(entries) != 2 {
				t.Fatalf("pair result %v", res.Return)
			}
			row.R = []string{encodeSnapshot(entries[0].Value), encodeSnapshot(entries[1].Value)}
		} else {
			row.R = []string{encodeSnapshot(res.Return)}
		}
	case UncaughtThrowable:
		row.E = []string{res.Throwable.Class, res.Throwable.Message}
	case Unsupported:
		return row, true
	default:
		t.Fatalf("run ended %v: %v", res.End, res.Err)
	}
	for _, d := range res.Diagnostics {
		if d.Level == "Fatal error" {
			continue
		}
		if d.File != vmTestFile || d.Line != vmOpLine {
			t.Errorf("diagnostic %q at %s:%d", d.Message, d.File, d.Line)
		}
		row.D = append(row.D, [2]any{float64(diagnosticLevelBit(d.Level)), d.Message})
	}
	return row, false
}

func sameOracleRow(want, got vmOracleRow) bool {
	if len(want.R) != len(got.R) {
		return false
	}
	for i := range want.R {
		if canonicalNaN(want.R[i]) != canonicalNaN(got.R[i]) {
			return false
		}
	}
	return slices.Equal(want.E, got.E) && slices.EqualFunc(want.D, got.D, func(x, y [2]any) bool { return x == y })
}

// coalesce emits r ?? dflt as the compiler does after a quiet read.
func (f *vmFn) coalesce(r uint32, dflt string) uint32 {
	res := f.reg(bytecode.Value)
	br := f.branch(f.bin(bytecode.Identical, r, f.k(nil)))
	yes := f.pc()
	f.emit(bytecode.OpMove, 0, res, f.k(dflt), 0)
	j := f.jump(0)
	no := f.pc()
	f.emit(bytecode.OpMove, 0, res, r, 0)
	f.patch(br, yes, no)
	f.patch(j, f.pc(), 0)
	return res
}

func isStringEnc(enc string) bool { return strings.HasPrefix(enc, "s:") }

func TestVMOracleTables(t *testing.T) {
	for _, width := range []int{64, 32} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			o := loadVMOracle(t, width)
			checked := 0
			check := func(table string, i int, label string, pair bool, locals []string, unsupportedOK bool, build func(m *vmFn) uint32) {
				t.Helper()
				want := o.Tables[table][i]
				got, unsupported := vmObserve(t, width, pair, locals, build)
				checked++
				if unsupported {
					if !unsupportedOK {
						t.Errorf("%s %s: unexpectedly unsupported", table, label)
					}
					return
				}
				if !sameOracleRow(want, got) {
					t.Errorf("%s %s\n  php:   %v\n  phego: %v", table, label, want, got)
				}
			}
			n := len(o.Values)
			for i, a := range o.Values {
				for j, b := range o.Values {
					check("xor", i*n+j, a+" xor "+b, false, nil, false, func(m *vmFn) uint32 {
						return m.bin(bytecode.BoolXor, m.operand(t, a), m.operand(t, b))
					})
				}
				check("cast_array", i, a, false, nil, false, func(m *vmFn) uint32 { return m.cast(bytecode.CastArray, m.operand(t, a)) })
			}
			incdec := map[string]bytecode.IncDecOp{"pre_inc": bytecode.PreInc, "pre_dec": bytecode.PreDec, "post_inc": bytecode.PostInc, "post_dec": bytecode.PostDec}
			for table, op := range incdec {
				for i, v := range o.IncDecValues {
					check(table, i, v, true, []string{"x"}, false, func(m *vmFn) uint32 {
						m.store("x", m.operand(t, v))
						r := m.incdec(op, "x")
						return m.arrayOf(r, m.load("x"))
					})
				}
			}
			for i, op := range []bytecode.IncDecOp{bytecode.PreInc, bytecode.PreDec, bytecode.PostInc, bytecode.PostDec} {
				check("undefined_inc", i, fmt.Sprint(op), true, []string{"u"}, false, func(m *vmFn) uint32 {
					r := m.incdec(op, "u")
					return m.arrayOf(r, m.load("u"))
				})
			}
			for i, k := range o.Keys {
				check("key_write", i, k, false, []string{"a"}, false, func(m *vmFn) uint32 {
					m.store("a", m.newArray())
					m.set("a", m.operand(t, k), m.k("v"))
					return m.load("a")
				})
				read := func(m *vmFn) { m.store("b", m.operand(t, o.ReadArray)) }
				check("key_read", i, k, false, []string{"b"}, false, func(m *vmFn) uint32 {
					read(m)
					return m.get(m.load("b"), m.operand(t, k), false)
				})
				check("key_read_quiet", i, k, false, []string{"b"}, false, func(m *vmFn) uint32 {
					read(m)
					return m.coalesce(m.get(m.loadQuiet("b"), m.operand(t, k), true), "dflt")
				})
				check("key_isset", i, k, false, []string{"b"}, false, func(m *vmFn) uint32 {
					read(m)
					return m.issetKey(m.loadQuiet("b"), m.operand(t, k))
				})
				check("key_unset", i, k, false, []string{"b"}, false, func(m *vmFn) uint32 {
					read(m)
					m.unsetKey("b", m.operand(t, k))
					return m.load("b")
				})
				check("string_isset", i, k, false, []string{"s"}, false, func(m *vmFn) uint32 {
					m.store("s", m.k("abc"))
					return m.issetKey(m.loadQuiet("s"), m.operand(t, k))
				})
			}
			for i, c := range o.Values {
				str := isStringEnc(c)
				x := []string{"x"}
				init := func(m *vmFn) { m.store("x", m.operand(t, c)) }
				check("container_read", i, c, false, x, str, func(m *vmFn) uint32 {
					init(m)
					return m.get(m.load("x"), m.k("k"), false)
				})
				check("container_read_quiet", i, c, false, x, str, func(m *vmFn) uint32 {
					init(m)
					return m.coalesce(m.get(m.loadQuiet("x"), m.k("k"), true), "dflt")
				})
				check("container_read_array_key", i, c, false, x, str, func(m *vmFn) uint32 {
					init(m)
					return m.get(m.load("x"), m.arrayOf(), false)
				})
				check("container_set", i, c, false, x, str, func(m *vmFn) uint32 {
					init(m)
					m.set("x", m.k("k"), m.k("v"))
					return m.load("x")
				})
				check("container_append", i, c, false, x, false, func(m *vmFn) uint32 {
					init(m)
					m.push("x", m.k("v"))
					return m.load("x")
				})
				check("container_unset", i, c, false, x, false, func(m *vmFn) uint32 {
					init(m)
					m.unsetKey("x", m.k("k"))
					return m.load("x")
				})
				check("container_isset", i, c, false, x, false, func(m *vmFn) uint32 {
					init(m)
					return m.issetKey(m.loadQuiet("x"), m.k("k"))
				})
				check("isset_var", i, c, false, x, false, func(m *vmFn) uint32 {
					init(m)
					return m.isset("x")
				})
			}
			u := []string{"u"}
			check("undefined_container", 0, "read", false, u, false, func(m *vmFn) uint32 { return m.get(m.load("u"), m.k("k"), false) })
			check("undefined_container", 1, "quiet", false, u, false, func(m *vmFn) uint32 {
				return m.coalesce(m.get(m.loadQuiet("u"), m.k("k"), true), "dflt")
			})
			check("undefined_container", 2, "set", false, u, false, func(m *vmFn) uint32 { m.set("u", m.k("k"), m.k("v")); return m.load("u") })
			check("undefined_container", 3, "append", false, u, false, func(m *vmFn) uint32 { m.push("u", m.k("v")); return m.load("u") })
			check("undefined_container", 4, "unset", false, u, false, func(m *vmFn) uint32 { m.unsetKey("u", m.k("k")); return m.k(1) })
			check("undefined_container", 5, "isset", false, u, false, func(m *vmFn) uint32 { return m.issetKey(m.loadQuiet("u"), m.k("k")) })
			t.Logf("%d observations checked", checked)
		})
	}
}
