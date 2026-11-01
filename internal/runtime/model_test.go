package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type fixtureFile struct {
	Cases []struct {
		ID     string `json:"id"`
		Events []struct {
			Op       string `json:"op"`
			Request  string `json:"request"`
			Resource string `json:"resource"`
			Handle   string `json:"handle"`
		} `json:"events"`
		Expected map[string]json.RawMessage `json:"expected"`
	} `json:"cases"`
}

func TestLifecycleFixtures(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "lifecycle", "scenarios.json"))
	if err != nil {
		t.Fatal(err)
	}
	var f fixtureFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	if len(f.Cases) != 6 {
		t.Fatalf("unexpected fixture count: %d", len(f.Cases))
	}
	for _, tc := range f.Cases {
		t.Run(tc.ID, func(t *testing.T) {
			requests := map[string]*Request{}
			handles := map[string]*Handle{}
			closed := map[string][]string{}
			returns := map[string]int{}
			failure := map[string]bool{}
			callbackBeforeReady := false
			affected := false
			cleanupFailed := false
			oldUsable := false
			for _, event := range tc.Events {
				id, op := event.Request, event.Op
				r := requests[id]
				var err error
				switch op {
				case "receive":
					if r != nil {
						t.Fatalf("duplicate receive %s", id)
					}
					requests[id] = newTestRequest()
				case "admit":
					err = r.Admit(func(context.Context) error { returns[id]++; return nil })
				case "begin_init":
					err = r.BeginInit()
				case "commit_resource":
					resource := event.Resource
					var h *Handle
					h, err = r.CommitResource(resource, func(context.Context) error {
						closed[id] = append(closed[id], resource)
						if failure[resource] {
							return os.ErrInvalid
						}
						return nil
					})
					if event.Handle != "" {
						handles[event.Handle] = h
					}
				case "begin_resource":
					// A resource has not entered the ledger until CommitResource succeeds.
				case "ready":
					err = r.Ready()
				case "run":
					if r.State() != Ready {
						callbackBeforeReady = true
					}
					err = r.Start()
				case "finish":
					err = r.Finish()
				case "fail_init", "cancel":
					err = r.Abort()
				case "cleanup":
					err = r.Close(testCleanupContext)
				case "cleanup_with_failure":
					failure[event.Resource] = true
					err = r.Close(testCleanupContext)
					cleanupFailed = err != nil
					err = nil
				case "use_handle":
					h := handles[event.Handle]
					oldUsable = h != nil && h.Valid()
					// Handles carry no resource access; a rejected stale handle cannot affect B.
					affected = oldUsable
				default:
					t.Fatalf("unknown fixture operation %q", op)
				}
				if err != nil {
					t.Fatalf("%s: %v", op, err)
				}
			}
			check := func(key string, got any) {
				raw, ok := tc.Expected[key]
				if !ok {
					return
				}
				switch value := got.(type) {
				case bool:
					var want bool
					if json.Unmarshal(raw, &want) != nil || value != want {
						t.Errorf("%s = %v, want %s", key, got, raw)
					}
				case int:
					var want int
					if json.Unmarshal(raw, &want) != nil || value != want {
						t.Errorf("%s = %v, want %s", key, got, raw)
					}
				case []string:
					var want []string
					if json.Unmarshal(raw, &want) != nil || !reflect.DeepEqual(value, want) {
						t.Errorf("%s = %v, want %s", key, got, raw)
					}
				case string:
					var want string
					if json.Unmarshal(raw, &want) != nil || value != want {
						t.Errorf("%s = %v, want %s", key, got, raw)
					}
				}
			}
			if r := requests["A"]; r != nil && r.State() == Closed {
				check("final_state", "Closed")
			} else {
				check("final_state", "other")
			}
			check("closed_once", closed["A"])
			check("admission_returns", returns["A"])
			check("admission_returns_for_A", returns["A"])
			check("callback_before_ready", callbackBeforeReady)
			check("php_shutdown_called", false) // The model has no PHP shutdown operation.
			check("old_handle_usable", oldUsable)
			check("other_request_resource_affected", affected)
			check("cleanup_failure_recorded", cleanupFailed)
			check("resource_reported_clean", !cleanupFailed)
			if raw, ok := tc.Expected["not_in_cleanup_ledger"]; ok {
				var names []string
				if err := json.Unmarshal(raw, &names); err != nil {
					t.Fatal(err)
				}
				for _, name := range names {
					for _, got := range closed["A"] {
						if name == got {
							t.Errorf("unfinished resource %s was cleaned", name)
						}
					}
				}
			}
		})
	}
}

// Explore all operation sequences through length five, including repeated and
// out-of-order transitions. Each sequence starts from an independent request.
func TestFiniteLifecycleTransitions(t *testing.T) {
	const depth = 5
	ops := []string{"admit", "init", "commit", "ready", "start", "finish", "abort", "close"}
	var explore func([]string)
	explore = func(sequence []string) {
		if len(sequence) < depth {
			for _, op := range ops {
				explore(append(append([]string(nil), sequence...), op))
			}
			return
		}
		r := newTestRequest()
		releases, closes := 0, 0
		var handles []*Handle
		everInvalid := map[*Handle]bool{}
		for _, op := range sequence {
			before := r.State()
			switch op {
			case "admit":
				_ = r.Admit(func(context.Context) error { releases++; return nil })
			case "init":
				_ = r.BeginInit()
			case "commit":
				h, err := r.CommitResource("resource", func(context.Context) error { closes++; return nil })
				if err == nil {
					handles = append(handles, h)
				}
			case "ready":
				err := r.Ready()
				if err == nil && before != Initializing {
					t.Fatalf("ready without init: %v", sequence)
				}
			case "start":
				err := r.Start()
				if err == nil && before != Ready {
					t.Fatalf("start before ready: %v", sequence)
				}
			case "finish":
				_ = r.Finish()
			case "abort":
				_ = r.Abort()
			case "close":
				_ = r.Close(testCleanupContext)
			}
			if releases > 1 || closes > 1 {
				t.Fatalf("duplicate cleanup: %v", sequence)
			}
			for _, h := range handles {
				valid := h.Valid()
				if everInvalid[h] && valid {
					t.Fatalf("handle revived: %v", sequence)
				}
				if !valid {
					everInvalid[h] = true
				}
			}
		}
	}
	explore(nil)
}
