package phego

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// W-41: REQ-067; T-133, T-134. Every program under tests/security/php-cve
// replays the trigger of a published PHP vulnerability that the catalog
// (engineering/compatibility/php-cve-catalog.json) lists for it. The
// references were produced by patched PHP 8.5.12 in the pinned images
// (tests/security/php-cve/README.md); Phego must produce the same bytes in
// every error mode, so a trigger that once corrupted PHP's memory ends here
// in PHP's fixed result instead of a panic, a hang or a different value.

// phpCVEProfileEnv, when set, names the one profile a CI job checks.
const phpCVEProfileEnv = "PHEGO_PHP_CVE_PROFILE"

func TestPHPCVERegressions(t *testing.T) {
	dir := filepath.Join("tests", "security", "php-cve")
	files, err := filepath.Glob(filepath.Join(dir, "cases", "*.php"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no regression cases")
	}
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimSuffix(filepath.Base(f), ".php"))
	}
	profiles := []w17Profile{
		{"64", "reference-64.json", 64},
		{"amd64", "reference-64-amd64.json", 64},
		{"32", "reference-32.json", 32},
	}
	if want := os.Getenv(phpCVEProfileEnv); want != "" && !slices.ContainsFunc(profiles, func(p w17Profile) bool { return p.name == want }) {
		t.Fatalf("%s=%q names no profile of this test", phpCVEProfileEnv, want)
	}
	for _, profile := range profiles {
		t.Run(profile.name, func(t *testing.T) {
			if want := os.Getenv(phpCVEProfileEnv); want != "" && want != profile.name {
				t.Skipf("%s selects profile %s", phpCVEProfileEnv, want)
			}
			raw, err := os.ReadFile(filepath.Join(dir, profile.file))
			if err != nil {
				t.Fatal(err)
			}
			var ref w17Reference
			if err := json.Unmarshal(raw, &ref); err != nil {
				t.Fatal(err)
			}
			if ref.IntSize*8 != profile.intSize {
				t.Fatalf("%s has PHP_INT_SIZE %d, want %d bits", profile.file, ref.IntSize, profile.intSize)
			}
			var refNames []string
			for _, c := range ref.Cases {
				refNames = append(refNames, c.Case)
			}
			if !slices.Equal(refNames, names) {
				t.Fatalf("%s covers %v, the cases are %v; regenerate the reference", profile.file, refNames, names)
			}
			e, err := NewEngine(Config{IntSize: profile.intSize})
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range ref.Cases {
				src, err := os.ReadFile(filepath.Join(dir, "cases", c.Case+".php"))
				if err != nil {
					t.Fatal(err)
				}
				if sum := sha256.Sum256(src); hex.EncodeToString(sum[:]) != c.SourceSHA256 {
					t.Fatalf("%s: the case changed after %s was generated; regenerate the reference", c.Case, profile.file)
				}
				for _, mode := range []struct {
					name         string
					want         w17Run
					display, log string
				}{
					{"display", c.Display, "1", "0"},
					{"log", c.Log, "0", "1"},
					{"both", c.Both, "1", "1"},
				} {
					t.Run(c.Case+"/"+mode.name, func(t *testing.T) {
						opts := []RunOption{WithINI("display_errors", mode.display), WithINI("log_errors", mode.log),
							WithINI("html_errors", "0"), WithINI("error_reporting", "-1")}
						opts = append(opts, w17INIOverrides(t, src)...)
						got := w17Execute(t, e, c.Case, string(src), opts...)
						if got.err != nil && got.compileFatal == nil && got.exitCode != 255 {
							t.Fatalf("run failed: %v", got.err)
						}
						w17Compare(t, "stdout", w17Decode(t, mode.want.Stdout), got.stdout)
						w17Compare(t, "stderr", w17Decode(t, mode.want.Stderr), got.stderr)
						if got.exitCode != mode.want.ExitCode {
							t.Errorf("exit code %d, want %d (error %v)", got.exitCode, mode.want.ExitCode, got.err)
						}
					})
				}
			}
		})
	}
}
