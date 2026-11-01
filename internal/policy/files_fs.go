package policy

import (
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// openFSFile interprets the virtual components before entering a trusted FS.
// Crossing its mount root preserves the unprocessed suffix for the next backend.
func (p *Policy) openFSFile(grant FileGrant, rel, name string, flag int, perm fs.FileMode, redirects int) (fs.File, error) {
	kind := FileRead
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		kind = FileWrite
	}
	denied := func(rule string) error { return &PolicyError{Kind: kind, Op: "open", Target: name, Rule: rule} }
	var names []string
	pending := strings.Split(rel, "/")
	links := 0
	for len(pending) > 0 {
		component := pending[0]
		pending = pending[1:]
		switch component {
		case "", ".":
			continue
		case "..":
			if len(names) == 0 {
				if grant.Path == "/" {
					continue
				}
				return p.openFile(mountPath(path.Dir(grant.Path), pending), flag, perm, redirects+1)
			}
			names = names[:len(names)-1]
			continue
		}
		parts := append(append([]string(nil), names...), component)
		lookup := strings.Join(parts, "/")
		current := path.Join(grant.Path, lookup)
		for _, mounted := range p.files {
			if mounted.Path != grant.Path && mounted.Path == current {
				return p.openFile(mountPath(current, pending), flag, perm, redirects+1)
			}
		}
		var info fs.FileInfo
		var err error
		if linkFS, ok := grant.FS.(fs.ReadLinkFS); ok {
			info, err = linkFS.Lstat(lookup)
			if err == nil && info.Mode()&os.ModeSymlink != 0 {
				links++
				if links > 40 {
					return nil, denied("path-changed")
				}
				target, e := linkFS.ReadLink(lookup)
				if e != nil {
					return nil, e
				}
				if path.IsAbs(target) || filepath.VolumeName(target) != "" {
					return nil, denied("outside-root")
				}
				targetParts := strings.Split(target, "/")
				resolved := path.Join(append(append([]string(nil), names...), targetParts...)...)
				if resolved == ".." || strings.HasPrefix(resolved, "../") {
					return nil, denied("outside-root")
				}
				pending = append(targetParts, pending...)
				continue
			}
		} else if len(pending) > 0 {
			info, err = fs.Stat(grant.FS, lookup)
		}
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) || !p.mountAncestor(current) {
				return nil, err
			}
		} else if len(pending) > 0 && info != nil && !info.IsDir() {
			return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
		}
		names = parts
		if len(names) > 256 {
			return nil, denied("protection-check-limit")
		}
	}
	if kind == FileWrite {
		return nil, denied("not-granted")
	}
	result := strings.Join(names, "/")
	if result == "" {
		result = "."
	}
	return grant.FS.Open(result)
}

func (p *Policy) mountAncestor(name string) bool {
	prefix := strings.TrimSuffix(name, "/") + "/"
	for _, mount := range p.files {
		if strings.HasPrefix(mount.Path, prefix) && mount.Path != name {
			return true
		}
	}
	return false
}
