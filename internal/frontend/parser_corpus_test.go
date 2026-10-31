package frontend

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestParseMatchesReferenceCorpus compares acceptance and error lines with
// PHP 8.5.12 token_get_all($src, TOKEN_PARSE) for the corpus described in
// TestLexMatchesReferenceCorpus. The reference is parse-reference.jsonl,
// produced by tests/frontend/parse_dump.php.
// CompileError records ("other") are checks PHP performs while building the
// tree; they must be rejected on the same line as well.
func TestParseMatchesReferenceCorpus(t *testing.T) {
	dir := os.Getenv("PHEGO_LEXER_CORPUS")
	if dir == "" {
		t.Skip("PHEGO_LEXER_CORPUS is not set")
	}
	f, err := os.Open(filepath.Join(dir, "parse-reference.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	files, mismatched := 0, 0
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
		name, _ := rec[0].(string)
		status, _ := rec[1].(string)
		if status != "ok" && (len(rec) < 4 || rec[2] == nil) {
			t.Fatalf("malformed reference record: %.120s", line)
		}
		src, err := os.ReadFile(filepath.Join(dir, "corpus", name))
		if err != nil {
			t.Fatal(err)
		}
		files++
		_, perr := Parse(src, LexOptions{ShortOpenTag: true, IntWidth: 32, MaxBytes: 1 << 24})
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
		key := status + "->" + got
		if got == want && gotLine == wantLine {
			stats[key+" (same line)"]++
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
