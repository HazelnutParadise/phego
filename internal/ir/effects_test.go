package ir

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/HazelnutParadise/phego/internal/frontend"
	"github.com/HazelnutParadise/phego/internal/profile"
)

// TestEffectsCoverObservedBehaviour runs the effects fixtures' reference,
// recorded from PHP 8.5.12 by tests/semantics/w11-hir/effects.php, and
// checks that HIR claims every effect PHP showed on each snippet line: a
// user callback needs MayCallUser, a warning MayWarn and an exception
// MayThrow. An optimizer may only drop or move code whose effects allow
// it, so a missing claim would let it remove observable behaviour.
func TestEffectsCoverObservedBehaviour(t *testing.T) {
	prof, err := profile.Resolve("8.5.12", 64, false)
	if err != nil {
		t.Fatal(err)
	}
	fixtures, _ := filepath.Glob("../../tests/semantics/w11-hir/effects/*.php")
	if len(fixtures) == 0 {
		t.Fatal("no effects fixtures")
	}
	for _, path := range fixtures {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path[:len(path)-4] + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var observed map[string][]string
			if err := json.Unmarshal(raw, &observed); err != nil {
				t.Fatal(err)
			}
			tree, err := frontend.Parse(src, frontend.LexOptions{IntWidth: 64, MaxBytes: len(src) + 1})
			if err != nil {
				t.Fatal(err)
			}
			unit, err := Lower(tree, prof, Options{File: "/w/effects/" + filepath.Base(path)})
			if err != nil {
				t.Fatal(err)
			}
			claimed := map[int]Effect{}
			for _, s := range unit.Main.Body {
				if s.Op != OpTry {
					continue
				}
				for _, k := range s.Kids[0].Kids {
					claimed[s.Line] |= k.Effects
				}
			}
			need := map[string]Effect{"call": MayCallUser, "warn": MayWarn, "throw": MayThrow}
			var lines []int
			for l := range observed {
				n, err := strconv.Atoi(l)
				if err != nil {
					t.Fatal(err)
				}
				lines = append(lines, n)
			}
			sort.Ints(lines)
			for _, line := range lines {
				got, ok := claimed[line]
				if !ok {
					t.Errorf("line %d: no snippet statement", line)
					continue
				}
				for _, ev := range observed[strconv.Itoa(line)] {
					if got&need[ev] == 0 {
						t.Errorf("line %d: PHP showed %s, HIR claims only {%s}", line, ev, got)
					}
				}
			}
		})
	}
}
