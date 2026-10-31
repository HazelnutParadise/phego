package frontend

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCorpusComparisonRejectsDuplicateAndMalformedRecords(t *testing.T) {
	for _, tc := range []struct{ name, test, filename, first, second, bad string }{
		{"parse", "TestParseMatchesReferenceCorpus", "parse-reference.jsonl", `["0000000000000001.php","ok"]`, `["0000000000000002.php","ok"]`, `["0000000000000002.php","bogus",1,"x"]`},
		{"lexer", "TestLexMatchesReferenceCorpus", "reference.jsonl", `["0000000000000001.php",[["T_OPEN_TAG","PD9waHAg",1]]]`, `["0000000000000002.php",[["T_OPEN_TAG","PD9waHAg",1]]]`, `["0000000000000002.php",[["T_OPEN_TAG","!!!!",1]]]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "corpus")
			if err := os.Mkdir(dir, 0755); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"0000000000000001.php", "0000000000000002.php"} {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("<?php "), 0600); err != nil {
					t.Fatal(err)
				}
			}
			run := func(contents string) (string, error) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(root, tc.filename), []byte(contents), 0600); err != nil {
					t.Fatal(err)
				}
				binary, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command(binary, "-test.run=^"+tc.test+"$", "-test.v")
				cmd.Env = append(os.Environ(), "PHEGO_LEXER_CORPUS="+root, "PHEGO_LEXER_REFERENCE=", "PHEGO_LEXER_INT_WIDTH=32")
				output, err := cmd.CombinedOutput()
				return string(output), err
			}
			if out, err := run(tc.first + "\n" + tc.second + "\n"); err != nil {
				t.Fatalf("valid input failed: %s", out)
			}
			if out, err := run(tc.first + "\n" + tc.first + "\n"); err == nil || !strings.Contains(out, "duplicate") {
				t.Fatalf("duplicate reference accepted or unclear: %s", out)
			}
			if out, err := run(tc.first + "\n" + tc.bad + "\n"); err == nil || !strings.Contains(out, "malformed") {
				t.Fatalf("malformed reference accepted or unclear: %s", out)
			}
		})
	}
}
