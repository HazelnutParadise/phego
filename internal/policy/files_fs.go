package policy

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// openFSFile interprets the virtual components before entering a trusted FS.
// Crossing its mount root preserves the unprocessed suffix for the next backend.
func (p *Policy) openFSFile(grant FileGrant, pending []filePart, name string, flag int, perm fs.FileMode, redirects int) (fs.File, error) {
	kind := FileRead
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		kind = FileWrite
	}
	denied := func(rule string) error { return &PolicyError{Kind: kind, Op: "open", Target: name, Rule: rule} }
	var names []string
	links := 0
	entryBudget := 100000
	syntheticDepth := 0
	_, symlinkAware := grant.FS.(fs.ReadLinkFS)
	for len(pending) > 0 {
		part := pending[0]
		component := part.name
		pending = pending[1:]
		switch component {
		case "", ".":
			continue
		case "..":
			if !linkParentAllowed(part.boundary, mountPath(grant.Path, names)) {
				return nil, denied("outside-root")
			}
			if len(names) == 0 {
				if grant.Path == "/" {
					continue
				}
				return p.redirectFile(path.Dir(grant.Path), pending, flag, perm, redirects)
			}
			names = names[:len(names)-1]
			if len(names) < syntheticDepth {
				syntheticDepth = 0
			}
			continue
		}
		parts := append(append([]string(nil), names...), component)
		lookup := strings.Join(parts, "/")
		current := path.Join(grant.Path, lookup)
		for _, mounted := range p.files {
			if mounted.Path != grant.Path && mounted.Path == current {
				return p.redirectFile(current, pending, flag, perm, redirects)
			}
		}
		// An FS may accept aliases such as a different case spelling. When
		// a nested mount exists here, only exact enumerated names may reach
		// the parent backend. Virtual mount ancestors need no host entry.
		parent := path.Join(grant.Path, strings.Join(names, "/"))
		if syntheticDepth > 0 {
			if !p.mountAncestor(current) {
				return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
			}
			names = parts
			if len(names) > 256 {
				return nil, denied("protection-check-limit")
			}
			continue
		}
		if p.mountAncestor(parent) || !symlinkAware && p.mountAncestor(grant.Path) {
			directory := strings.Join(names, "/")
			if directory == "" {
				directory = "."
			}
			if err := exactFSEntry(grant.FS, directory, component, &entryBudget); err != nil {
				if p.mountAncestor(current) {
					// Traverse only virtual descendants. An unverified backing
					// name must never become a source for sibling reads.
					syntheticDepth = len(parts)
					names = parts
					if len(names) > 256 {
						return nil, denied("protection-check-limit")
					}
					continue
				}
				return nil, err
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
				targetParts := fileParts(target, grant.Path)
				pending = append(targetParts, pending...)
				continue
			}
		} else if len(pending) > 0 {
			info, err = fs.Stat(grant.FS, lookup)
		}
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) || !p.mountAncestor(current) {
				if kind == FileWrite {
					return nil, denied("not-granted")
				}
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
	if syntheticDepth > 0 {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	result := strings.Join(names, "/")
	if result == "" {
		result = "."
	}
	f, err := grant.FS.Open(result)
	if err != nil {
		return nil, err
	}
	if p.mountAncestor(grant.Path) {
		info, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		// Enforce the host-directory Root contract before exposing OS files.
		// This recognizes OS metadata, not arbitrary trusted FS internals.
		if os.SameFile(info, info) {
			f.Close()
			return nil, denied("not-granted")
		}
	}
	return f, nil
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

// exactFSEntry uses bounded batches rather than an unbounded ReadDirFS call.
// FS providers must make directory names and exact Open names consistent.
func exactFSEntry(source fs.FS, directory, name string, budget *int) error {
	f, err := source.Open(directory)
	if err != nil {
		return err
	}
	defer f.Close()
	dir, ok := f.(fs.ReadDirFile)
	if !ok {
		return &fs.PathError{Op: "open", Path: directory, Err: fs.ErrPermission}
	}
	for *budget > 0 {
		count := 128
		if count > *budget {
			count = *budget
		}
		entries, err := dir.ReadDir(count)
		*budget -= len(entries)
		if len(entries) > count {
			return &fs.PathError{Op: "open", Path: directory, Err: fs.ErrPermission}
		}
		for _, entry := range entries {
			if entry.Name() == name {
				if entry.Type()&os.ModeSymlink != 0 {
					if _, ok := source.(fs.ReadLinkFS); !ok {
						return &fs.PathError{Op: "open", Path: path.Join(directory, name), Err: fs.ErrPermission}
					}
				}
				return nil
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return &fs.PathError{Op: "open", Path: path.Join(directory, name), Err: fs.ErrNotExist}
			}
			return err
		}
		if len(entries) == 0 {
			break
		}
	}
	return &fs.PathError{Op: "open", Path: directory, Err: fs.ErrPermission}
}
