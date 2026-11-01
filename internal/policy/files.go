package policy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// OpenFile opens name for the requested access through the granted tree
// that contains it. name must be absolute. The open goes through os.Root, so
// ".." components and symbolic links cannot leave the tree even if the file
// system changes between the check and the open; a plain string prefix
// check would not resist that.
//
// flag uses the os.O_* values. Any write, create, append or truncate flag
// needs a tree granted for writing.
func (p *Policy) OpenFile(name string, flag int, perm fs.FileMode) (*os.File, error) {
	kind, op := FileRead, "open"
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		kind = FileWrite
	}
	grant, rel, err := p.fileGrant(kind, op, name)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(grant.Root)
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

// fileGrant finds the most specific grant whose tree contains name and
// allows kind.
func (p *Policy) fileGrant(kind Kind, op, name string) (FileGrant, string, error) {
	if !filepath.IsAbs(name) || strings.ContainsRune(name, 0) {
		return FileGrant{}, "", &PolicyError{Kind: kind, Op: op, Target: name, Rule: "not-absolute"}
	}
	clean := filepath.Clean(name)
	var best FileGrant
	found := false
	for _, g := range p.files {
		if kind == FileWrite && !g.Write {
			continue
		}
		rel, err := filepath.Rel(g.Root, clean)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		if !found || len(g.Root) > len(best.Root) {
			best, found = g, true
		}
	}
	if !found {
		return FileGrant{}, "", &PolicyError{Kind: kind, Op: op, Target: name, Rule: "outside-root"}
	}
	rel, _ := filepath.Rel(best.Root, clean)
	return best, rel, nil
}
