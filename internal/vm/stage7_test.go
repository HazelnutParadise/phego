package vm

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/bytecode"
)

// The class and object fixtures, compiled as TestReferenceFixturesCompile
// compiles them, survive Build, Encode and Load unchanged: the class table,
// the function flags, return and parameter types, and every instruction of
// real compiler output round-trip through format 1.7.
func TestObjectFixturesRoundTripThroughTheWireFormat(t *testing.T) {
	files, err := filepath.Glob("../../tests/vm/w17/cases/*.php")
	if err != nil {
		t.Fatal(err)
	}
	prefixes := []string{"class_", "object_", "method_", "prop_", "type_", "magic_method_"}
	var picked []string
	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".php")
		for _, p := range prefixes {
			if strings.HasPrefix(name, p) {
				picked = append(picked, path)
				break
			}
		}
	}
	if len(picked) < 50 {
		t.Fatalf("only %d class and object fixtures", len(picked))
	}
	withClasses := 0
	for _, width := range []int{64, 32} {
		for _, path := range picked {
			name := strings.TrimSuffix(filepath.Base(path), ".php")
			t.Run(name+"/"+itoa(width), func(t *testing.T) {
				src, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				unit, err := lowerSource(string(src), width, "/w/cases/"+name+".php")
				if err != nil {
					t.Fatalf("lower: %v", err)
				}
				img, err := Compile(context.Background(), unit, src)
				if err != nil {
					t.Fatalf("Compile: %v", err)
				}
				if len(img.Classes) > 0 {
					withClasses++
				}
				p, err := bytecode.Build(context.Background(), img, buildOptions())
				if err != nil {
					t.Fatalf("Build: %v\n%s", err, Disassemble(img))
				}
				data, err := p.Encode(context.Background())
				if err != nil {
					t.Fatalf("Encode: %v", err)
				}
				q, err := bytecode.Load(context.Background(), data, bytecode.LoadOptions{Options: buildOptions(), Expected: img.Identity, TrustedDigest: sha256.Sum256(data)})
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				requireSameProgram(t, img, q)
				again, err := q.Encode(context.Background())
				if err != nil || string(again) != string(data) {
					t.Fatalf("the reloaded program encodes differently (%v)", err)
				}
			})
		}
	}
	if withClasses == 0 {
		t.Fatal("no fixture declared a class")
	}
}

// requireSameProgram checks that q holds exactly the class table, functions
// and code of img.
func requireSameProgram(t *testing.T, img bytecode.Image, q *bytecode.Program) {
	t.Helper()
	if q.ClassCount() != len(img.Classes) {
		t.Fatalf("ClassCount = %d, want %d", q.ClassCount(), len(img.Classes))
	}
	for ci, want := range img.Classes {
		got, ok := q.Class(uint32(ci))
		// %+v prints nil and empty slices alike, the only difference the
		// encoding does not keep.
		if !ok || fmt.Sprintf("%+v", got) != fmt.Sprintf("%+v", want) {
			t.Fatalf("class %d = %+v, want %+v", ci, got, want)
		}
	}
	if q.FunctionCount() != len(img.Functions) || q.Entry() != img.Entry {
		t.Fatalf("FunctionCount %d entry %d, want %d and %d", q.FunctionCount(), q.Entry(), len(img.Functions), img.Entry)
	}
	str := func(i uint32) string {
		if i == 0 {
			return ""
		}
		return img.Constants[i-1].Bytes
	}
	for fi, f := range img.Functions {
		code, _ := q.Instructions(uint32(fi), nil)
		if fmt.Sprint(code) != fmt.Sprint(f.Code) {
			t.Fatalf("function %d %q: code differs after the round trip", fi, f.Name)
		}
		v, _ := q.Function(uint32(fi))
		if v.Flags() != f.Flags || v.ReturnType() != str(f.ReturnType) || v.ParameterCount() != len(f.Parameters) {
			t.Fatalf("function %d %q: flags %d return %q params %d, want %d %q %d", fi, f.Name, v.Flags(), v.ReturnType(), v.ParameterCount(), f.Flags, str(f.ReturnType), len(f.Parameters))
		}
		for pi, p := range f.Parameters {
			name, flags, ok := v.Parameter(uint32(pi))
			if !ok || name != img.Constants[p.Name].Bytes || flags != p.Flags || v.ParameterType(uint32(pi)) != str(p.Type) {
				t.Fatalf("function %d %q parameter %d differs after the round trip", fi, f.Name, pi)
			}
		}
	}
	for i, c := range img.Constants {
		if got, ok := q.Constant(uint32(i)); !ok || got != c {
			t.Fatalf("constant %d = %+v, want %+v", i, got, c)
		}
	}
}
