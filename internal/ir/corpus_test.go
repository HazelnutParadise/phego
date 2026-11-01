package ir

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/profile"
)

// TestLowerMatchesCompileReference compares lowering with what PHP 8.5.12
// reports while compiling each corpus file (php -l), as recorded by
// tests/semantics/w11-hir/compile_dump.php. PHEGO_LEXER_CORPUS names the
// directory holding corpus/ and compile-reference.jsonl (see
// tools/frontend_corpus.py). Files the front end rejects are the parser's
// concern and are skipped here.
//
// Every file PHP compiles must lower, and every compile error must be
// reported with PHP's message and line. Errors owned by later work packages
// (the object model, attributes, the built-in function table) are counted
// by gapPrefix; the test fails when their number changes.
func TestLowerMatchesCompileReference(t *testing.T) {
	dir := os.Getenv("PHEGO_LEXER_CORPUS")
	if dir == "" {
		t.Skip("PHEGO_LEXER_CORPUS is not set")
	}
	f, err := os.Open(filepath.Join(dir, "compile-reference.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	prof, err := profile.Resolve("8.5.12", 64, false)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	var falseRejects, wrong, missing, panics, diags []string
	gaps := map[string]int{}
	files, matched, warned := 0, 0, 0
	for scanner.Scan() {
		var rec []json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil || len(rec) != 2 {
			t.Fatalf("malformed reference record: %.120s", scanner.Bytes())
		}
		var name string
		var events [][]any
		if json.Unmarshal(rec[0], &name) != nil || json.Unmarshal(rec[1], &events) != nil {
			t.Fatalf("malformed reference record: %.120s", scanner.Bytes())
		}
		src, err := os.ReadFile(filepath.Join(dir, "corpus", name))
		if err != nil {
			t.Fatal(err)
		}
		var fatal []any
		parseError := false
		for _, e := range events {
			switch e[0] {
			case "Fatal error":
				fatal = e
			case "Parse error":
				parseError = true
			}
		}
		tree, err := frontend.Parse(src, frontend.LexOptions{IntWidth: 64, MaxBytes: len(src) + 1})
		if err != nil || parseError {
			continue
		}
		files++
		unit, lerr := lowerRecovered(tree, prof, "/corpus/"+name)
		if pe, ok := lerr.(panicError); ok {
			panics = append(panics, name+": "+string(pe))
			continue
		}
		var ce *CompileError
		if lerr != nil && !errors.As(lerr, &ce) {
			t.Fatalf("%s: unexpected error %v", name, lerr)
		}
		if fatal == nil && ce == nil {
			var want, got []string
			for _, e := range events {
				if e[0] == "Warning" || e[0] == "Deprecated" || e[0] == "Notice" {
					want = append(want, fmt.Sprintf("%s: %s on line %v", e[0], e[1], e[2]))
				}
			}
			for _, d := range unit.Diagnostics {
				got = append(got, fmt.Sprintf("%s: %s on line %d", d.Level, d.Message, d.Line))
			}
			if len(want) > 0 {
				warned++
			}
			if strings.Join(want, "\n") != strings.Join(got, "\n") {
				if g := diagnosticGap(name, want); g != "" {
					gaps[g]++
				} else {
					diags = append(diags, fmt.Sprintf("%s:\n  got  %q\n  want %q", name, got, want))
				}
			}
		}
		switch {
		case fatal == nil && ce != nil:
			falseRejects = append(falseRejects, fmt.Sprintf("%s:%d: %s", name, ce.Line, ce.Message))
		case fatal != nil && ce == nil:
			msg := fatal[1].(string)
			if prefix := gapPrefix(name, msg); prefix != "" {
				gaps[prefix]++
			} else {
				missing = append(missing, fmt.Sprintf("%s:%v: %s", name, fatal[2], msg))
			}
		case fatal != nil:
			matched++
			if ce.Message != fatal[1].(string) || float64(ce.Line) != fatal[2].(float64) {
				wrong = append(wrong, fmt.Sprintf("%s: got %d %q, want %v %q", name, ce.Line, ce.Message, fatal[2], fatal[1]))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d files lowered, %d compile errors raised, %d files with compile warnings", files, matched, warned)
	report := func(kind string, list []string) {
		if len(list) == 0 {
			return
		}
		sort.Strings(list)
		t.Errorf("%d %s:\n%s", len(list), kind, strings.Join(list, "\n"))
	}
	report("files that make lowering panic", panics)
	report("files PHP compiles but lowering rejects", falseRejects)
	report("compile errors with a different message or line", wrong)
	report("compile errors lowering does not raise", missing)
	report("files with different compile warnings", diags)
	if os.Getenv("PHEGO_IR_GAPS") != "" {
		var keys []string
		for k := range gaps {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			t.Logf("gap %4d %s", gaps[k], k)
		}
	}
	total := 0
	for _, n := range gaps {
		total += n
	}
	if total != knownGapCount {
		t.Errorf("compile errors left to later work packages: %d, recorded %d", total, knownGapCount)
	}
}

type panicError string

func (p panicError) Error() string { return string(p) }

// lowerRecovered reports a panic as an error so one file cannot hide the
// results of the rest of the corpus.
func lowerRecovered(tree *frontend.Node, prof profile.Profile, file string) (unit *Unit, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = panicError(fmt.Sprint(r))
		}
	}()
	return Lower(tree, prof, Options{File: file})
}
