package policy

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
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
	return p.openFile(name, flag, perm, 0)
}

func (p *Policy) openFile(name string, flag int, perm fs.FileMode, redirects int) (fs.File, error) {
	kind, op := FileRead, "open"
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		kind = FileWrite
	}
	if redirects >= 40 {
		return nil, &PolicyError{Kind: kind, Op: op, Target: name, Rule: "path-changed"}
	}
	grant, rel, err := p.fileGrant(kind, op, name)
	if err != nil {
		return nil, err
	}

	if kind == FileWrite && !grant.Write {
		parent := false
		for _, component := range strings.Split(rel, "/") {
			if component == ".." {
				parent = true
			}
		}
		if !parent {
			return nil, &PolicyError{Kind: kind, Op: op, Target: name, Rule: "not-granted"}
		}
	}
	if grant.FS != nil {
		return p.openFSFile(grant, rel, name, flag, perm, redirects)
	}

	root, err := os.OpenRoot(grant.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := p.openRootFile(root, grant, rel, name, flag, perm, redirects)
	if err != nil {
		var pe *fs.PathError
		if errors.As(err, &pe) && strings.Contains(pe.Err.Error(), "escapes") {
			return nil, &PolicyError{Kind: kind, Op: op, Target: name, Rule: "outside-root"}
		}
		return nil, err
	}
	return f, nil
}

// fileGrant selects the longest spelled mount prefix. Parent components remain
// unresolved until the rooted opener has followed any preceding symlinks.
func (p *Policy) fileGrant(kind Kind, op, name string) (FileGrant, string, error) {
	if !path.IsAbs(name) || strings.ContainsRune(name, 0) {
		return FileGrant{}, "", &PolicyError{Kind: kind, Op: op, Target: name, Rule: "not-absolute"}
	}
	clean := virtualPath(name)
	var best FileGrant
	found := false
	for _, g := range p.files {
		if clean != g.Path && !strings.HasPrefix(clean, strings.TrimSuffix(g.Path, "/")+"/") {
			continue
		}
		if !found || len(g.Path) > len(best.Path) {
			best, found = g, true
		}
	}
	if !found {
		return FileGrant{}, "", &PolicyError{Kind: kind, Op: op, Target: name, Rule: "outside-root"}
	}

	rel := strings.TrimPrefix(strings.TrimPrefix(clean, best.Path), "/")
	if rel == "" {
		rel = "."
	}
	return best, rel, nil
}

// virtualPath removes redundant separators and leading dot components, but never
// collapses a parent component past a name that could be a symbolic link.
func virtualPath(name string) string {
	var components []string
	for _, component := range strings.Split(filepath.ToSlash(name), "/") {
		if component == "" || component == "." && len(components) == 0 {
			continue
		}
		if component == ".." && len(components) == 0 {
			continue
		}
		components = append(components, component)
	}
	if len(components) > 0 && strings.HasSuffix(filepath.ToSlash(name), "/") {
		components = append(components, ".")
	}
	return "/" + strings.Join(components, "/")
}

func mountPath(mount string, remaining []string) string {
	if len(remaining) == 0 {
		return mount
	}
	return strings.TrimSuffix(mount, "/") + "/" + strings.Join(remaining, "/")
}
