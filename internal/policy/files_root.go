package policy

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
)

// File authorization follows the resolved directory handles, not the spelling
// of the path. Host code owns the mount layout; PHP cannot rename directories
// or create links through this API.
func (p *Policy) openRootFile(root *os.Root, grant FileGrant, pending []filePart, name string, flag int, perm fs.FileMode, redirects int) (fs.File, error) {
	kind := FileRead
	if flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		kind = FileWrite
	}
	denied := func(rule string) error { return &PolicyError{Kind: kind, Op: "open", Target: name, Rule: rule} }
	if redirects >= 40 {
		return nil, denied("path-changed")
	}
	type frame struct {
		root *os.Root
		info fs.FileInfo
	}
	info, err := root.Stat(".")
	if err != nil {
		return nil, err
	}
	stack := []frame{{root, info}}
	defer func() {
		for _, frame := range stack[1:] {
			if frame.root != nil {
				frame.root.Close()
			}
		}
	}()
	var names []string
	links := 0
	var leaf string
	var before fs.FileInfo
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
			if len(stack) == 1 {
				if grant.Path == "/" {
					continue
				}
				return p.redirectFile(path.Dir(grant.Path), pending, flag, perm, redirects)
			}
			if stack[len(stack)-1].root != nil {
				stack[len(stack)-1].root.Close()
			}
			stack = stack[:len(stack)-1]
			names = names[:len(names)-1]
			continue
		}
		currentName := path.Join(grant.Path, path.Join(append(append([]string(nil), names...), component)...))
		for _, mounted := range p.files {
			if mounted.Path != grant.Path && mounted.Path == currentName {
				return p.redirectFile(mounted.Path, pending, flag, perm, redirects)
			}
		}
		parent := stack[len(stack)-1].root
		var st fs.FileInfo
		var e error
		if parent == nil {
			e = &fs.PathError{Op: "open", Path: currentName, Err: fs.ErrNotExist}
		} else {
			st, e = parent.Lstat(component)
		}
		if errors.Is(e, fs.ErrNotExist) && p.mountAncestor(currentName) {
			if len(stack) >= 256 {
				return nil, denied("protection-check-limit")
			}
			stack = append(stack, frame{})
			names = append(names, component)
			continue
		}
		if e != nil {
			if parent != nil && len(pending) == 0 && errors.Is(e, fs.ErrNotExist) && flag&os.O_CREATE != 0 {
				leaf = component
				break
			}
			return nil, e
		}
		if len(pending) == 0 && flag&(os.O_CREATE|os.O_EXCL) == os.O_CREATE|os.O_EXCL {
			leaf, before = component, st
			break
		}
		if st.Mode()&os.ModeSymlink != 0 {
			links++
			if links > 40 {
				return nil, denied("path-changed")
			}
			target, e := parent.Readlink(component)
			if e != nil {
				return nil, e
			}
			if filepath.IsAbs(target) {
				mounted, targetParts, ok := p.absoluteLinkMount(target, grant.Path)
				if !ok {
					return nil, denied("outside-root")
				}
				return p.redirectFile(mounted, append(targetParts, pending...), flag, perm, redirects)
			}
			if filepath.VolumeName(target) != "" {
				return nil, denied("outside-root")
			}
			targetParts := fileParts(target, grant.Path)
			pending = append(targetParts, pending...)
			continue
		}
		if st.IsDir() {
			mounted, e := p.directoryMount(root, grant, st)
			if e != nil {
				return nil, e
			}
			if mounted != "" {
				return p.redirectFile(mounted, pending, flag, perm, redirects)
			}
		}
		if len(pending) == 0 {
			leaf = component
			before = st
			break
		}
		if len(stack) >= 256 {
			return nil, denied("protection-check-limit")
		}
		child, e := parent.OpenRoot(component)
		if e != nil {
			return nil, e
		}
		actual, e := child.Stat(".")
		if e != nil {
			child.Close()
			return nil, e
		}
		if !os.SameFile(st, actual) {
			child.Close()
			return nil, denied("path-changed")
		}
		stack = append(stack, frame{child, actual})
		names = append(names, component)
	}
	if leaf == "" {
		if kind == FileRead && stack[len(stack)-1].root != nil {
			return stack[len(stack)-1].root.Open(".")
		}
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	resolved := path.Join(grant.Path, path.Join(append(append([]string(nil), names...), leaf)...))
	selected, _, err := p.fileGrant(kind, "open", resolved)
	if err != nil {
		return nil, err
	}
	if selected.Path != grant.Path {
		return p.openFile(resolved, flag, perm, redirects+1)
	}
	if kind == FileWrite && !grant.Write {
		return nil, denied("not-granted")
	}
	if before != nil && flag&(os.O_CREATE|os.O_EXCL) == os.O_CREATE|os.O_EXCL {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrExist}
	}
	if kind == FileRead {
		if !before.Mode().IsRegular() && !before.IsDir() {
			return nil, denied("not-granted")
		}
		// A regular file may be replaced by a FIFO after Lstat. Open without
		// waiting for a peer, then verify the actual object before returning it.
		f, err := stack[len(stack)-1].root.OpenFile(leaf, flag|nonblockOpenFlag, perm)
		if err != nil {
			return nil, err
		}
		actual, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, err
		}
		if !os.SameFile(before, actual) || !actual.Mode().IsRegular() && !actual.IsDir() {
			f.Close()
			return nil, denied("path-changed")
		}
		if actual.Mode().IsRegular() && !singleLink(actual) {
			if err := p.hiddenReadAlias(root, grant, actual, name); err != nil {
				f.Close()
				return nil, err
			}
		}
		return f, nil
	}
	protections, err := p.fileProtections(root, grant)
	if err != nil {
		var pe *PolicyError
		if errors.As(err, &pe) && pe.Target == "" {
			pe.Target = name
		}
		return nil, err
	}
	defer func() {
		for _, r := range protections {
			r.root.Close()
		}
	}()
	for _, protection := range protections {
		for _, f := range stack {
			if os.SameFile(f.info, protection.info) {
				return nil, denied("not-granted")
			}
		}
	}
	parent := stack[len(stack)-1].root
	if before == nil {
		// Exclusive creation cannot follow a final symlink installed after Lstat.
		return parent.OpenFile(leaf, flag|os.O_EXCL, perm)
	}
	if !before.Mode().IsRegular() {
		return nil, denied("not-granted")
	}
	f, err := parent.OpenFile(leaf, flag&^(os.O_TRUNC|os.O_CREATE|os.O_EXCL)|nonblockOpenFlag, perm)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (fs.File, error) { f.Close(); return nil, e }
	actual, err := f.Stat()
	if err != nil {
		return fail(err)
	}
	if !os.SameFile(before, actual) {
		return fail(denied("path-changed"))
	}
	if !singleLink(actual) {
		for _, protection := range protections {
			protected, e := protection.contains(actual, name)
			if e != nil {
				return fail(e)
			}
			if protected {
				return fail(denied("not-granted"))
			}
		}
	}
	if flag&os.O_TRUNC != 0 {
		if flag&(os.O_WRONLY|os.O_RDWR) == 0 {
			// Preserve O_RDONLY|O_TRUNC: truncate through a separately verified writer,
			// while returning the original read-only descriptor.
			writer, e := parent.OpenFile(leaf, os.O_WRONLY|nonblockOpenFlag, 0)
			if e != nil {
				return fail(e)
			}
			st, e := writer.Stat()
			if e == nil && !os.SameFile(actual, st) {
				e = denied("path-changed")
			}
			if e == nil {
				e = writer.Truncate(0)
			}
			writer.Close()
			if e != nil {
				return fail(e)
			}
		} else if err := f.Truncate(0); err != nil {
			return fail(err)
		}
	}
	return f, nil
}

type fileProtection struct {
	root    *os.Root
	info    fs.FileInfo
	exclude []fs.FileInfo
}

// Both the mounted backend and the parent tree hidden underneath a read-only
// mount are protected. A writable descendant only excludes its own actual tree.
func (p *Policy) fileProtections(root *os.Root, grant FileGrant) (out []fileProtection, err error) {
	defer func() {
		if err != nil {
			for _, r := range out {
				r.root.Close()
			}
		}
	}()
	for _, g := range p.files {
		if g.Write {
			continue
		}
		var roots []*os.Root
		if g.Dir != "" {
			r, e := os.OpenRoot(g.Dir)
			if e != nil {
				return out, e
			}
			roots = append(roots, r)
		}
		prefix := strings.TrimSuffix(grant.Path, "/") + "/"
		if strings.HasPrefix(g.Path, prefix) {
			r, e := root.OpenRoot(strings.TrimPrefix(g.Path, prefix))
			if e != nil && !unavailableShadow(e) {
				for _, r := range roots {
					r.Close()
				}
				return out, e
			}
			if e == nil {
				roots = append(roots, r)
			}
		}
		for i, r := range roots {
			info, e := r.Stat(".")
			if e != nil {
				for _, pending := range roots[i:] {
					pending.Close()
				}
				return out, e
			}
			protection := fileProtection{root: r, info: info}
			out = append(out, protection)
			// A mount can begin inside a protected tree without traversing its
			// root. Only an explicit, deeper virtual writable mount overrides it.
			inside, e := rootContains(r, root, grant.Dir)
			if e != nil || inside && !strings.HasPrefix(grant.Path, strings.TrimSuffix(g.Path, "/")+"/") {
				for _, pending := range roots[i+1:] {
					pending.Close()
				}
				if e != nil {
					return out, e
				}
				return out, &PolicyError{Kind: FileWrite, Op: "open", Rule: "not-granted"}
			}
			for _, w := range p.files {
				if !w.Write || !strings.HasPrefix(w.Path, strings.TrimSuffix(g.Path, "/")+"/") {
					continue
				}
				wr, e := os.OpenRoot(w.Dir)
				if e != nil {
					for _, pending := range roots[i+1:] {
						pending.Close()
					}
					return out, e
				}
				wi, e := wr.Stat(".")
				wr.Close()
				if e != nil {
					for _, pending := range roots[i+1:] {
						pending.Close()
					}
					return out, e
				}
				if !os.SameFile(info, wi) {
					out[len(out)-1].exclude = append(out[len(out)-1].exclude, wi)
				}
			}
		}
	}
	return out, nil
}

// singleLink is only an optimization. Platforms without this metadata take the
// conservative scan path; no writable handle is returned without verification.
func singleLink(info fs.FileInfo) bool {
	v := reflect.ValueOf(info.Sys())
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return false
	}
	n := v.FieldByName("Nlink")
	return n.IsValid() && n.CanUint() && n.Uint() == 1
}

// contains checks pre-existing hard-link aliases without following symlinks.
// Metadata work, directory depth and directory buffers are explicitly bounded.
func (p fileProtection) contains(target fs.FileInfo, name string) (bool, error) {
	const maxEntries = 100000
	visited := 0
	var walk func(*os.Root, int) (bool, error)
	walk = func(root *os.Root, depth int) (bool, error) {
		if depth > 256 {
			return false, &PolicyError{Kind: FileWrite, Op: "open", Target: name, Rule: "protection-check-limit"}
		}
		dir, err := root.Open(".")
		if err != nil {
			return false, err
		}
		defer dir.Close()
		for {
			entries, err := dir.ReadDir(128)
			if err != nil && !errors.Is(err, io.EOF) {
				return false, err
			}
			for _, entry := range entries {
				visited++
				if visited > maxEntries {
					return false, &PolicyError{Kind: FileWrite, Op: "open", Target: name, Rule: "protection-check-limit"}
				}
				info, e := root.Lstat(entry.Name())
				if e != nil {
					return false, e
				}
				if os.SameFile(info, target) {
					return true, nil
				}
				if !info.IsDir() {
					continue
				}
				skip := false
				for _, exclude := range p.exclude {
					if os.SameFile(info, exclude) {
						skip = true
						break
					}
				}
				if skip {
					continue
				}
				child, e := root.OpenRoot(entry.Name())
				if e != nil {
					return false, e
				}
				actual, e := child.Stat(".")
				if e != nil {
					child.Close()
					return false, e
				}
				if !os.SameFile(info, actual) {
					child.Close()
					return false, &PolicyError{Kind: FileWrite, Op: "open", Target: name, Rule: "path-changed"}
				}
				found, e := walk(child, depth+1)
				child.Close()
				if found || e != nil {
					return found, e
				}
			}
			if err != nil {
				return false, nil
			}
		}
	}
	return walk(p.root, 0)
}

// rootContains checks actual directory identities up the host-controlled mount
// path. String prefix/Rel checks are insufficient on case-insensitive volumes.
// PHP cannot modify this mount topology.
func rootContains(parent, child *os.Root, childName string) (bool, error) {
	want, err := parent.Stat(".")
	if err != nil {
		return false, err
	}
	start, err := child.Stat(".")
	if err != nil {
		return false, err
	}
	current, err := filepath.EvalSymlinks(childName)
	if err != nil {
		return false, err
	}
	for depth := 0; depth < 256; depth++ {
		probe, err := os.OpenRoot(current)
		if err != nil {
			return false, err
		}
		info, err := probe.Stat(".")
		probe.Close()
		if err != nil {
			return false, err
		}
		if depth == 0 && !os.SameFile(start, info) {
			return false, &PolicyError{Kind: FileWrite, Op: "open", Rule: "path-changed"}
		}
		if os.SameFile(want, info) {
			return true, nil
		}
		next := filepath.Dir(current)
		if next == current {
			return false, nil
		}
		current = next
	}
	return false, &PolicyError{Kind: FileWrite, Op: "open", Rule: "protection-check-limit"}
}

// directoryMount recognizes nested mounts by physical directory identity, also
// when the host filesystem accepts a different spelling of the directory name.
func (p *Policy) directoryMount(root *os.Root, grant FileGrant, info fs.FileInfo) (string, error) {
	prefix := strings.TrimSuffix(grant.Path, "/") + "/"
	best := ""
	for _, mounted := range p.files {
		if mounted.Path == grant.Path || !strings.HasPrefix(mounted.Path, prefix) || len(mounted.Path) <= len(best) {
			continue
		}
		shadow, err := root.Stat(strings.TrimPrefix(mounted.Path, prefix))
		if unavailableShadow(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if os.SameFile(info, shadow) {
			best = mounted.Path
		}
	}
	return best, nil
}

// A hard link has no symlink target to rematch. Refuse an alias of data hidden
// by a nested mount, while leaving ordinary readable hard links usable.
func (p *Policy) hiddenReadAlias(root *os.Root, grant FileGrant, target fs.FileInfo, name string) error {
	prefix := strings.TrimSuffix(grant.Path, "/") + "/"
	for _, mounted := range p.files {
		if mounted.Path == grant.Path || !strings.HasPrefix(mounted.Path, prefix) {
			continue
		}
		shadow, err := root.OpenRoot(strings.TrimPrefix(mounted.Path, prefix))
		if unavailableShadow(err) {
			continue
		}
		if err != nil {
			return err
		}
		if mounted.Dir != "" {
			backend, e := os.OpenRoot(mounted.Dir)
			if e == nil {
				a, ae := backend.Stat(".")
				b, be := shadow.Stat(".")
				backend.Close()
				if ae == nil && be == nil && os.SameFile(a, b) {
					shadow.Close()
					continue
				}
			}
		}
		found, err := (fileProtection{root: shadow}).contains(target, name)
		shadow.Close()
		if err != nil {
			var pe *PolicyError
			if errors.As(err, &pe) {
				pe.Kind = FileRead
			}
			return err
		}
		if found {
			return &PolicyError{Kind: FileRead, Op: "open", Target: name, Rule: "not-granted"}
		}
	}
	return nil
}

// A shadow with no resolvable in-root directory cannot match an unrelated
// directory identity. Permission and I/O failures still fail closed.
func unavailableShadow(err error) bool {
	return errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) || rootEscapeError(err)
}
