package frontend

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseMatchesReferenceCorpus compares acceptance, error text and locations with
// PHP 8.5.12 token_get_all($src, TOKEN_PARSE) for the corpus described in
// TestLexMatchesReferenceCorpus. PHEGO_PARSER_REFERENCE selects the reference
// dump (default parse-reference.jsonl), produced by tests/frontend/parse_dump.php.
// PHEGO_LEXER_INT_WIDTH selects the same integer width as the lexer test.
// CompileError records ("other") are checks PHP performs while building the
// tree; they must be rejected on the same line as well.
func TestParseMatchesReferenceCorpus(t *testing.T) {
	dir := os.Getenv("PHEGO_LEXER_CORPUS")
	if dir == "" {
		t.Skip("PHEGO_LEXER_CORPUS is not set")
	}
	reference, width := os.Getenv("PHEGO_PARSER_REFERENCE"), 32
	if reference == "" {
		reference = "parse-reference.jsonl"
	}
	if os.Getenv("PHEGO_LEXER_INT_WIDTH") == "64" {
		width = 64
	}
	f, err := os.Open(filepath.Join(dir, reference))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	files, mismatched := 0, 0
	seen := make(map[string]bool)
	stats := map[string]int{}
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '[' {
			continue
		}
		var rec []any
		if json.Unmarshal(line, &rec) != nil || len(rec) < 2 {
			t.Fatalf("malformed reference record: %.120s", line)
		}
		name, okName := rec[0].(string)
		status, okStatus := rec[1].(string)
		if !okName || !validCorpusFileName(name) || !okStatus || (status != "ok" && status != "error" && status != "other") {
			t.Fatal("malformed reference record: invalid name or status")
		}
		if status == "ok" && len(rec) != 2 {
			t.Fatal("malformed reference record: unexpected successful record fields")
		}
		wantCaller := false
		if status != "ok" {
			if len(rec) != 4 && len(rec) != 5 {
				t.Fatal("malformed reference record: missing error fields")
			}
			line, okLine := rec[2].(float64)
			_, okMessage := rec[3].(string)
			if !okLine || line < 1 || line > 1<<31 || line != float64(int64(line)) || !okMessage {
				t.Fatal("malformed reference record: invalid error line or message")
			}
			if len(rec) == 5 {
				var err error
				wantCaller, err = referenceCallerLocation(rec[4], line)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		if seen[name] {
			t.Fatalf("duplicate reference record for %s", name)
		}
		seen[name] = true
		src, err := os.ReadFile(filepath.Join(dir, "corpus", name))
		if err != nil {
			t.Fatal(err)
		}
		files++
		_, perr := Parse(src, LexOptions{ShortOpenTag: os.Getenv("PHEGO_LEXER_SHORT_OPEN_TAG") != "0", IntWidth: width, MaxBytes: 1 << 24})
		var se *SyntaxError
		got := "ok"
		gotLine := 0
		if errors.As(perr, &se) {
			got, gotLine = "error", se.Line
		} else if perr != nil {
			t.Fatalf("%s: %v", name, perr)
		}
		want, wantLine := "ok", 0
		if status != "ok" {
			want, wantLine = "error", int(rec[2].(float64))
		}
		// "other" records are CompileErrors PHP raises while parsing.
		if se != nil && se.Compile != (status == "other") {
			got = "misclassified"
		}
		if se != nil && se.CallerLocation && len(rec) == 4 {
			t.Fatalf("%s: caller location requires paired observations; regenerate parse reference with tests/frontend/parse_dump.php", name)
		}
		locationMatches := gotLine == wantLine
		if wantCaller || (se != nil && se.CallerLocation) {
			locationMatches = se != nil && se.CallerLocation == wantCaller && len(rec) >= 4 && se.Message == rec[3]
		}
		key := status + "->" + got
		messageMatches := status == "ok"
		if se != nil && status != "ok" {
			// The PHP JSON dumper replaces invalid UTF-8 bytes. Keep the Go
			// diagnostic raw and normalize only at this reference boundary.
			message := strings.Map(func(r rune) rune { return r }, se.Message)
			wantMessage := rec[3].(string)
			if status == "other" {
				wantMessage = strings.TrimPrefix(wantMessage, "CompileError: ")
			}
			messageMatches = message == wantMessage
		}
		if got == want && locationMatches && messageMatches {
			if wantCaller {
				stats[key+" (caller location)"]++
			} else {
				stats[key+" (same line)"]++
			}
			continue
		}
		stats[key]++
		mismatched++
		if mismatched <= 40 {
			msg := ""
			if se != nil {
				msg = se.Message
			}
			t.Errorf("%s: reference %v, got %s line %d %s", name, rec[1:], got, gotLine, msg)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	corpus, err := filepath.Glob(filepath.Join(dir, "corpus", "*.php"))
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 || files != len(corpus) {
		t.Fatalf("compared %d reference records for %d corpus files", files, len(corpus))
	}
	t.Logf("%d of %d files agree; %v", files-mismatched, files, stats)
	if mismatched > 0 {
		t.Errorf("%d files differ", mismatched)
	}
}

// A caller-relative exception is established by two independently observed
// call sites. Keep raw file/line observations rather than rewriting PHP's line
// to the parser's source anchor or accepting a caller claim without evidence.
func referenceCallerLocation(raw any, firstLine float64) (bool, error) {
	fail := func() (bool, error) {
		return false, fmt.Errorf("malformed reference record: invalid location observations")
	}
	m, ok := raw.(map[string]any)
	if !ok || len(m) != 3 {
		return fail()
	}
	kind, ok := m["kind"].(string)
	if !ok || (kind != "caller" && kind != "source") {
		return fail()
	}
	calls, ok := m["call_lines"].([]any)
	observed, okObserved := m["observed"].([]any)
	if !ok || !okObserved || len(calls) != 2 || len(observed) != 2 {
		return fail()
	}
	var call, line [2]float64
	var file [2]string
	validLine := func(v float64) bool { return v >= 1 && v <= 1<<31 && v == float64(int64(v)) }
	for i := range 2 {
		call[i], ok = calls[i].(float64)
		if !ok || !validLine(call[i]) {
			return fail()
		}
		location, ok := observed[i].([]any)
		if !ok || len(location) != 2 {
			return fail()
		}
		file[i], ok = location[0].(string)
		if !ok {
			return fail()
		}
		line[i], ok = location[1].(float64)
		if !ok || !validLine(line[i]) {
			return fail()
		}
	}
	if call[0] == call[1] || file[0] != file[1] || line[0] != firstLine {
		return fail()
	}
	if kind == "caller" {
		if line != call {
			return fail()
		}
		return true, nil
	}
	if line[0] != line[1] {
		return fail()
	}
	return false, nil
}
