package frontend

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParserReferenceCallerLocations(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "corpus"), 0755); err != nil {
		t.Fatal(err)
	}
	name := "0000000000000001.php"
	if err := os.WriteFile(filepath.Join(root, "corpus", name), []byte("<?php\n<<<X\n$a\n X;"), 0600); err != nil {
		t.Fatal(err)
	}
	base := `["0000000000000001.php","error",6,"Invalid body indentation level (expecting an indentation level of at least 1)"`
	for _, tc := range []struct{ record, failure string }{
		{`["0000000000000001.php","ok"]`, "files differ"},
		{base + `,{"kind":"caller","call_lines":[6,18],"observed":[["/parse.php",6],["/parse.php",18]]}]`, ""},
		{base + `]`, "regenerate"},
		{base + `,{"kind":"bogus","call_lines":[6,18],"observed":[["/parse.php",6],["/parse.php",18]]}]`, "malformed"},
		{base + `,{"kind":"caller","call_lines":[6,18],"observed":[["/parse.php",6]]}]`, "malformed"},
		{base + `,{"kind":"caller","call_lines":[6,6],"observed":[["/parse.php",6],["/parse.php",6]]}]`, "malformed"},
		{base + `,{"kind":"caller","call_lines":[6,18],"observed":[["/parse.php",6],["/other.php",18]]}]`, "malformed"},
		{base + `,{"kind":"caller","call_lines":[6,18],"observed":[["/parse.php",6],["/parse.php",17]]}]`, "malformed"},
		{base + `,{"kind":"source","call_lines":[6,18],"observed":[["/parse.php",6],["/parse.php",6]]}]`, "files differ"},
	} {
		if err := os.WriteFile(filepath.Join(root, "parse-reference.jsonl"), []byte(tc.record+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		binary, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "-test.run=^TestParseMatchesReferenceCorpus$", "-test.v")
		cmd.Env = append(os.Environ(), "PHEGO_LEXER_CORPUS="+root, "PHEGO_PARSER_REFERENCE=", "PHEGO_LEXER_INT_WIDTH=64")
		out, err := cmd.CombinedOutput()
		if tc.failure == "" && err != nil {
			t.Fatalf("valid caller observations rejected: %s", out)
		}
		if tc.failure != "" && (err == nil || !strings.Contains(string(out), tc.failure)) {
			t.Errorf("expected %q failure for %s; got %s", tc.failure, tc.record, out)
		}
	}
}
