package frontend

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLexMatchesReferenceCorpus compares token kinds, text, and lines with
// PHP 8.5.12 token_get_all output. It runs when PHEGO_LEXER_CORPUS names a
// directory holding corpus/*.php and the reference dump named by
// PHEGO_LEXER_REFERENCE (default reference.jsonl) for the integer width in
// PHEGO_LEXER_INT_WIDTH (default 32), with short_open_tag on; see
// docs/implementation/W-09.md.
func TestLexMatchesReferenceCorpus(t *testing.T) {
	dir := os.Getenv("PHEGO_LEXER_CORPUS")
	if dir == "" {
		t.Skip("PHEGO_LEXER_CORPUS is not set")
	}
	reference, width := os.Getenv("PHEGO_LEXER_REFERENCE"), 32
	if reference == "" {
		reference = "reference.jsonl"
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
	scanner.Buffer(make([]byte, 64<<20), 64<<20)
	files, mismatched := 0, 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '[' {
			// PHP warnings printed by the dump script are not records.
			continue
		}
		var rec []json.RawMessage
		var name string
		var want [][3]any
		if json.Unmarshal(line, &rec) != nil || len(rec) != 2 ||
			json.Unmarshal(rec[0], &name) != nil || json.Unmarshal(rec[1], &want) != nil {
			t.Fatalf("malformed reference record: %.120s", line)
		}
		src, err := os.ReadFile(filepath.Join(dir, "corpus", name))
		if err != nil {
			t.Fatal(err)
		}
		files++
		got, _, err := Lex(src, LexOptions{ShortOpenTag: true, IntWidth: width, MaxBytes: 1 << 24})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if i, msg := firstDifference(got, want); i >= 0 {
			mismatched++
			if mismatched <= 25 {
				t.Errorf("%s token %d: %s", name, i, msg)
			}
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
	t.Logf("%d of %d files match", files-mismatched, files)
	if mismatched > 0 {
		t.Errorf("%d files differ", mismatched)
	}
}

func firstDifference(got []Token, want [][3]any) (int, string) {
	for i := 0; i < len(got) || i < len(want); i++ {
		if i >= len(got) || i >= len(want) {
			return i, "token count differs"
		}
		name, _ := want[i][0].(string)
		text, _ := base64.StdEncoding.DecodeString(want[i][1].(string))
		line, _ := want[i][2].(float64)
		g := got[i]
		if g.Kind.String() != name || g.Text != string(text) || (line != 0 && int(line) != g.Line) {
			return i, "got " + g.Kind.String() + " " + quote(g.Text) + " line " + itoa(g.Line) +
				", want " + name + " " + quote(string(text)) + " line " + itoa(int(line))
		}
	}
	return -1, ""
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	if len(b) > 80 {
		return string(b[:80]) + "…"
	}
	return string(b)
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }
