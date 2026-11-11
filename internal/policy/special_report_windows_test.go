//go:build windows

package policy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWindowsSpecialFileReport writes what a Root mount does with Windows
// device names and directories, so the special-files workflow can compare
// it with PHP on Windows (tests/policy/special-files/probe-windows.ps1 runs
// the same cases). It runs only when PHEGO_SPECIAL_FILE_REPORT names the
// file to write.
func TestWindowsSpecialFileReport(t *testing.T) {
	out := os.Getenv("PHEGO_SPECIAL_FILE_REPORT")
	if out == "" {
		t.Skip("set PHEGO_SPECIAL_FILE_REPORT to write the report")
	}
	work := t.TempDir()
	for _, d := range []string{"d", "sub"} {
		if err := os.Mkdir(filepath.Join(work, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/app", Dir: work, Write: true}}})
	var report strings.Builder
	open := func(name, path string, flag int) {
		f, err := p.OpenFile(t.Context(), path, flag, 0o600)
		if err != nil {
			fmt.Fprintf(&report, "== %s\nerror %T: %v\n", name, err, err)
			return
		}
		defer f.Close()
		mode := "unknown"
		info, err := f.Stat()
		if err == nil {
			mode = info.Mode().String()
		}
		fmt.Fprintf(&report, "== %s\nopened mode=%s stat-error=%v\n", name, mode, err)
		if flag&(os.O_WRONLY|os.O_RDWR) == 0 {
			b, err := io.ReadAll(io.LimitReader(f, 16))
			fmt.Fprintf(&report, "read %q %v\n", b, err)
		}
	}
	modes := []struct {
		mode string
		flag int
	}{
		{"r", os.O_RDONLY},
		{"w", os.O_WRONLY | os.O_CREATE | os.O_TRUNC},
		{"r+", os.O_RDWR},
		{"a", os.O_WRONLY | os.O_CREATE | os.O_APPEND},
		{"x", os.O_WRONLY | os.O_CREATE | os.O_EXCL},
	}
	for _, name := range []string{"NUL", "nul", "NUL.txt", "sub/NUL", "CON", "COM1", "CONIN$", "CONOUT$", "LPT1", "NUL/x"} {
		open("fopen <work>\\"+strings.ReplaceAll(name, "/", "\\")+" r", "/app/"+name, os.O_RDONLY)
	}
	for _, c := range modes[1:] {
		open("fopen <work>\\NUL "+c.mode, "/app/NUL", c.flag)
	}
	open("fopen <work>\\sub\\CON w", "/app/sub/CON", modes[1].flag)
	for _, c := range modes {
		open("fopen dir "+c.mode, "/app/d", c.flag)
	}
	for _, c := range modes[:2] {
		open("fopen <work> "+c.mode, "/app", c.flag)
	}
	open("fopen <work>\\ads:s r", "/app/ads:s", os.O_RDONLY)
	open("fopen <work>\\ads:s w", "/app/ads:s", modes[1].flag)
	if err := os.WriteFile(out, []byte(report.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Log(report.String())
}
