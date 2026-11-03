package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func w13AssertOracleCase(t *testing.T, width int, name string, got w13Case) {
	t.Helper()
	directory := os.Getenv("PHEGO_W13_REFERENCE")
	if directory == "" {
		directory = "testdata"
	}
	data, err := os.ReadFile(filepath.Join(directory, fmt.Sprintf("w13-ref-%d.json", width)))
	must(t, err)
	var oracle struct {
		PHP     string             `json:"php"`
		IntSize int                `json:"int_size"`
		Cases   map[string]w13Case `json:"cases"`
	}
	must(t, json.Unmarshal(data, &oracle))
	if oracle.PHP != "8.5.12" || oracle.IntSize*8 != width {
		t.Fatal("wrong reference profile")
	}
	want, ok := oracle.Cases[name]
	if !ok {
		t.Fatal("missing native case", name)
	}
	a, err := json.Marshal(got)
	must(t, err)
	b, err := json.Marshal(want)
	must(t, err)
	if string(a) != string(b) {
		t.Fatalf("%s\nGo: %s\nPHP: %s", name, a, b)
	}
}
