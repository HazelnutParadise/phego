package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// REQ-020/021; T-039-042. This replays the explicit temporary ownership in
// gc_corpus.php; expected counts and event order come only from pinned PHP.
func w13RunGCCorpusCase(t *testing.T, h *heap, name string) w13Case {
	// matrix.php runs cases inside a by-value foreach. Keep that caller's
	// immutable case-name table alive during the action replay as well.
	var cases, item value
	for _, caseName := range w13CaseNames() {
		must(t, h.appendValue(&cases, str(caseName)))
	}
	var result w13Case
	must(t, h.foreachValue(cases, &item, func(key arrayKey) error {
		if item.text == name {
			result = w13GCCorpusBody(t, h, name)
		}
		return nil
	}))
	must(t, h.unset(&item))
	must(t, h.unset(&cases))
	return result
}

func w13GCCorpusBody(t *testing.T, h *heap, name string) w13Case {
	t.Helper()
	caseID, err := strconv.Atoi(strings.TrimPrefix(name, "gc-sequence-"))
	must(t, err)
	events := []any{}
	counts := []any{}
	c := &class{name: "W13GC", destructor: func(_ *heap, this value) error {
		slot, ok := this.object.props.lookup("name")
		if !ok {
			return fmt.Errorf("missing object name")
		}
		events = append(events, slot.text)
		return nil
	}}
	var vars value
	must(t, h.assign(&vars, h.newArrayValue()))
	seed, serial := caseID, 0
	collect := func() {
		n, err := h.collectCycles()
		must(t, err)
		counts = append(counts, int64(n))
	}
	for range 40 {
		seed = (seed*73 + 19) % 65521
		op, a, b := seed%5, int64(seed/5%8), int64(seed/40%8)
		switch op {
		case 0:
			serial++
			obj, err := h.newObject(c)
			must(t, err)
			retain(obj)
			must(t, h.setProperty(obj, "name", str(fmt.Sprintf("s%d-%d", caseID, serial))))
			must(t, h.setElement(&vars, w12Int(a), obj))
			must(t, h.release(obj))
		case 1:
			from, ok, err := h.getElement(vars, w12Int(a))
			must(t, err)
			if !ok {
				continue
			}
			to, ok, err := h.getElement(vars, w12Int(b))
			must(t, err)
			if !ok {
				to = nullValue()
			}
			retain(from)
			retain(to)
			must(t, h.setProperty(from, fmt.Sprintf("p%d", seed%3), to))
			must(t, h.release(from))
			must(t, h.release(to))
		case 2:
			must(t, h.unsetElement(&vars, w12Int(a)))
		case 3:
			collect()
		case 4:
			v, ok, err := h.getElement(vars, w12Int(b))
			must(t, err)
			if ok {
				must(t, h.setElement(&vars, w12Int(a), v))
			}
		}
	}
	collect()
	must(t, h.unset(&vars))
	collect()
	return w13Case{Result: []any{counts, events}, Diagnostics: w13Diagnostics(h)}
}

func TestW13GCCorpus(t *testing.T) {
	for _, width := range []int{32, 64} {
		data, err := os.ReadFile(fmt.Sprintf("testdata/w13-ref-%d.json", width))
		must(t, err)
		var reference struct {
			Cases map[string]w13Case `json:"cases"`
		}
		must(t, json.Unmarshal(data, &reference))
		for i := 1; i <= 64; i++ {
			name := fmt.Sprintf("gc-sequence-%d", i)
			t.Run(fmt.Sprintf("%d/%s", width, name), func(t *testing.T) {
				h, err := referenceHeap(width)
				must(t, err)
				got := w13RunGCCorpusCase(t, h, name)
				gotJSON, err := json.Marshal(got)
				must(t, err)
				wantJSON, err := json.Marshal(reference.Cases[name])
				must(t, err)
				if string(gotJSON) != string(wantJSON) {
					t.Fatalf("Go: %s\nPHP: %s", gotJSON, wantJSON)
				}
			})
		}
	}
}
