package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// REQ-012/018/020; T-023/024/035/036/039/040. The isolated driver
// preserves exact profile checks while exposing each pre-fix regression.
func TestW13R6PHP8512Regression(t *testing.T) {
	dir := os.Getenv("PHEGO_W13_REFERENCE")
	if dir == "" {
		dir = "testdata"
	}
	for _, width := range []int{32, 64} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("w13-ref-%d.json", width)))
			must(t, err)
			var ref struct {
				PHP     string             `json:"php"`
				IntSize int                `json:"int_size"`
				Cases   map[string]w13Case `json:"cases"`
			}
			must(t, json.Unmarshal(data, &ref))
			if ref.PHP != "8.5.12" || ref.IntSize*8 != width {
				t.Fatalf("unexpected reference profile: %s/%d", ref.PHP, ref.IntSize)
			}
			names := append(w13R6LazyNames(), w13R6ReferenceNames()...)
			names = append(names, w13R6ScopeNames()...)
			names = append(names, w13R6ProxyGuardNames()...)
			names = append(names, w13R6GuardLifetimeName)
			for _, name := range names {
				t.Run(name, func(t *testing.T) {
					want, ok := ref.Cases[name]
					if !ok {
						t.Fatalf("missing reference case %q", name)
					}
					h, err := referenceHeap(width)
					must(t, err)
					got := w13RunCase(t, h, nil, name)
					gotJSON, err := json.Marshal(got)
					must(t, err)
					wantJSON, err := json.Marshal(want)
					must(t, err)
					if string(gotJSON) != string(wantJSON) {
						t.Fatalf("Go: %s\nPHP: %s", gotJSON, wantJSON)
					}
				})
			}
		})
	}
}
