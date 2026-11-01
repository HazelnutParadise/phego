package policy

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"strings"
)

// OpenFile opens name, an absolute PHP path, through the mount that
// contains it. A host directory is opened through os.Root, so ".."
// components and symbolic links cannot leave it even if the file system
// changes between the check and the open; a plain string prefix check
// would not resist that.
//
// flag uses the os.O_* values. Any write, create, append or truncate flag
// needs a mount granted for writing. The result is an *os.File for a host
// directory and the file system's own file for an fs.FS mount.
func (p *Policy) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	kind, op := FileRead, "open"
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		kind = FileWrite
	}
	grant, rel, err := p.fileGrant(kind, op, name)
	if err != nil {
		return nil, err
	}
	if grant.FS != nil {
		return grant.FS.Open(rel)
	}
	root, err := os.OpenRoot(grant.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.OpenFile(rel, flag, perm)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) && strings.Contains(pe.Err.Error(), "escapes") {
			return nil, &PolicyError{Kind: kind, Op: op, Target: name, Rule: "outside-root"}
		}
		return nil, err
	}
	return f, nil
}

// fileGrant finds the most specific mount whose path contains name and
// allows kind, and returns name relative to it in fs.ValidPath form.
func (p *Policy) fileGrant(kind Kind, op, name string) (FileGrant, string, error) {
	if !path.IsAbs(name) || strings.ContainsRune(name, 0) {
		return FileGrant{}, "", &PolicyError{Kind: kind, Op: op, Target: name, Rule: "not-absolute"}
	}
	clean := path.Clean(name)
	var best FileGrant
	found := false
	for _, g := range p.files {
		if kind == FileWrite && !g.Write {
			continue
		}
		if clean != g.Path && !strings.HasPrefix(clean, strings.TrimSuffix(g.Path, "/")+"/") {
			continue
		}
		if !found || len(g.Path) > len(best.Path) {
			best, found = g, true
		}
	}
	if !found {
		rule := "outside-root"
		if kind == FileWrite {
			if _, _, err := p.fileGrant(FileRead, op, name); err == nil {
				rule = "not-granted" // mounted, but read-only
			}
		}
		return FileGrant{}, "", &PolicyError{Kind: kind, Op: op, Target: name, Rule: rule}
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(clean, best.Path), "/")
	if rel == "" {
		rel = "."
	}
	return best, rel, nil
}
