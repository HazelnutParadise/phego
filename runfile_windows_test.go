//go:build windows

package phego

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// php <dir> and php <dir>\NUL (PHP 8.5.12 on windows-2025) print "Could
// not open input file". RunFile fails the same way, with the error the
// policy reports, not a policy refusal or an empty script.
func TestRunFileWindowsDirectoryAndDeviceName(t *testing.T) {
	app := t.TempDir()
	if err := os.Mkdir(filepath.Join(app, "d"), 0o700); err != nil {
		t.Fatal(err)
	}
	e := newPolicyEngine(t, Config{Policy: Policy{Files: []FileGrant{{Path: "/app", Root: app}}}})
	for _, c := range []struct {
		path string
		want error
	}{{"/app/d", fs.ErrPermission}, {"/app", fs.ErrPermission}, {"/app/NUL", fs.ErrNotExist}, {"/app/d/CON", fs.ErrNotExist}} {
		res, err := e.RunFile(t.Context(), c.path)
		var pe *PolicyError
		if res == nil || res.Status != StatusFailed || errors.As(err, &pe) || !errors.Is(err, c.want) {
			t.Errorf("RunFile(%s) = %+v, %v; want StatusFailed with %v", c.path, res, err, c.want)
		}
	}
}
