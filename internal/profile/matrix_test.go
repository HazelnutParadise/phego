package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type regressionMatrix struct {
	SchemaVersion int    `json:"schema_version"`
	Description   string `json:"description"`
	Profiles      []struct {
		Release  string `json:"release"`
		IntWidth int    `json:"int_width"`
		Suites   []struct {
			ID       string   `json:"id"`
			Tests    []string `json:"tests"`
			Requires string   `json:"requires"`
		} `json:"suites"`
		SecurityExceptions []struct {
			ID       string `json:"id"`
			Evidence string `json:"evidence"`
		} `json:"security_exceptions"`
		KnownGaps []string `json:"known_gaps"`
	} `json:"profiles"`
}

// checkMatrix returns every way raw fails to cover the implemented
// releases with existing tests.
func checkMatrix(raw []byte, defined map[string]bool) []string {
	var m regressionMatrix
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil || m.SchemaVersion != 1 {
		return []string{fmt.Sprintf("invalid matrix: %v", err)}
	}
	var problems []string
	seen := map[string]bool{}
	for _, p := range m.Profiles {
		key := fmt.Sprintf("%s/int%d", p.Release, p.IntWidth)
		if seen[key] {
			problems = append(problems, key+" is listed twice")
		}
		seen[key] = true
		if ReleaseStatus(p.Release) != Implemented {
			problems = append(problems, key+" is in the matrix but not implemented")
		}
		if p.IntWidth != 32 && p.IntWidth != 64 {
			problems = append(problems, key+" has an invalid integer width")
		}
		if len(p.Suites) == 0 {
			problems = append(problems, key+" has no regression suites")
		}
		for _, s := range p.Suites {
			if s.ID == "" || len(s.Tests) == 0 {
				problems = append(problems, fmt.Sprintf("%s: suite %q names no tests", key, s.ID))
			}
			for _, name := range s.Tests {
				if !defined[name] {
					problems = append(problems, fmt.Sprintf("%s suite %s: test %s does not exist", key, s.ID, name))
				}
			}
		}
		for _, e := range p.SecurityExceptions {
			if e.ID == "" || e.Evidence == "" {
				problems = append(problems, key+": security exception without id or evidence")
			}
		}
	}
	for release := range implemented {
		for _, width := range []int{32, 64} {
			if key := fmt.Sprintf("%s/int%d", release, width); !seen[key] {
				problems = append(problems, "implemented "+key+" has no regression matrix")
			}
		}
	}
	return problems
}

func definedTests(t *testing.T, root string) map[string]bool {
	t.Helper()
	defined := map[string]bool{}
	testFunc := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && (d.Name() == ".git" || d.Name() == "testdata"):
			return filepath.SkipDir
		case !strings.HasSuffix(path, "_test.go"):
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range testFunc.FindAllSubmatch(src, -1) {
			defined[string(m[1])] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return defined
}

// TestEveryImplementedProfileHasRegressionSuites keeps
// docs/compatibility/profile-matrix.json complete: every implemented
// release has an entry for both integer widths, every entry lists suites,
// and every suite names Go tests that exist. Adding a release without its
// regression matrix fails here (REQ-066, T-131, T-132).
func TestEveryImplementedProfileHasRegressionSuites(t *testing.T) {
	root := filepath.Join("..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "docs", "compatibility", "profile-matrix.json"))
	if err != nil {
		t.Fatal(err)
	}
	defined := definedTests(t, root)
	for _, p := range checkMatrix(raw, defined) {
		t.Error(p)
	}
	// The check itself must catch a missing width and a renamed test.
	broken := `{"schema_version": 1, "profiles": [{"release": "8.5.12", "int_width": 64,
		"suites": [{"id": "x", "tests": ["TestDoesNotExist"]}], "security_exceptions": [{"id": "CVE-1"}]}]}`
	got := strings.Join(checkMatrix([]byte(broken), defined), "\n")
	for _, want := range []string{"test TestDoesNotExist does not exist", "8.5.12/int32 has no regression matrix", "without id or evidence"} {
		if !strings.Contains(got, want) {
			t.Errorf("broken matrix: missing %q in:\n%s", want, got)
		}
	}
}
