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
// contains it. Host directories are opened through os.Root and verified by
// identity. Absolute symlink targets inside granted directories are remapped
// to PHP paths; target parent components cannot escape their selected mount.
// Host directories permit regular files, directories and the host's null,
// zero, full, random and urandom character devices, identified by device
// number; FIFOs, sockets, block devices and other character devices are
// refused.
// Host-created hard links still require a trusted layout.
//
// flag uses the os.O_* values. Any write, create, append or truncate flag
// needs a mount granted for writing. The result is an *os.File for a host
// directory and the file system's own file for an fs.FS mount.
func (p *Policy) OpenFile(name string, flag int, perm fs.FileMode) (fs.File, error) {
	return p.openFile(name, openRequest{flag: flag, perm: perm})
}

// openRequest is one open as it follows redirects between mounts.
type openRequest struct {
	flag      int
	perm      fs.FileMode
	redirects int
}

func (r openRequest) kind() Kind {
	if r.flag&(os.O_WRONLY|os.O_RDWR|os.O_APPEND|os.O_CREATE|os.O_TRUNC) != 0 {
		return FileWrite
	}
	return FileRead
}

// next is the request after one more redirect.
func (r openRequest) next() openRequest {
	r.redirects++
	return r
}

// openable reports what a host directory opens for reading: regular files,
// directories and the listed character devices. FIFOs, sockets, block
// devices and other character devices stay closed: a FIFO or terminal read
// could wait for a peer, and some devices act when opened.
func openable(fi fs.FileInfo) bool {
	m := fi.Mode()
	return m.IsRegular() || m.IsDir() || isCharDevice(m) && listedCharDevice(fi)
}

// filePart retains the root boundary of a symlink target through mount changes.
// Requested parent components have no boundary and may traverse virtual parents.
type filePart struct{ name, boundary string }

func fileParts(name, boundary string) []filePart {
	var parts []filePart
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		parts = append(parts, filePart{part, boundary})
	}
	return parts
}

// rebaseLinkParts moves the remaining components of an earlier link target to
// the mount an absolute link resolved into. Their .. now walks that mount's
// directories, so that mount bounds them; requested components stay free.
func rebaseLinkParts(parts []filePart, mount string) []filePart {
	out := make([]filePart, len(parts))
	for i, part := range parts {
		if part.boundary != "" {
			part.boundary = mount
		}
		out[i] = part
	}
	return out
}

func partsPath(base string, parts []filePart) string {
	names := make([]string, len(parts))
	for i, part := range parts {
		names[i] = part.name
	}
	return mountPath(base, names)
}

func linkParentAllowed(boundary, current string) bool {
	return boundary == "" || current != boundary && strings.HasPrefix(current, strings.TrimSuffix(boundary, "/")+"/")
}

func (p *Policy) openFile(name string, req openRequest) (fs.File, error) {
	if !path.IsAbs(name) || strings.ContainsRune(name, 0) {
		return nil, &PolicyError{Kind: req.kind(), Op: "open", Target: name, Rule: "not-absolute"}
	}
	return p.openFileParts(fileParts(virtualPath(name), ""), req)
}

func (p *Policy) redirectFile(base string, pending []filePart, req openRequest) (fs.File, error) {
	return p.openFileParts(append(fileParts(base, ""), pending...), req.next())
}

func (p *Policy) openFileParts(parts []filePart, req openRequest) (fs.File, error) {
	kind, op := req.kind(), "open"
	// Internal redirects preserve symlink provenance rather than cleaning .. away.
	var normalized []filePart
	for _, part := range parts {
		if part.name == "" || part.name == "." && len(normalized) == 0 {
			continue
		}
		if part.name == ".." && len(normalized) == 0 {
			if part.boundary != "" {
				return nil, &PolicyError{Kind: kind, Op: op, Target: partsPath("/", parts), Rule: "outside-root"}
			}
			continue
		}
		normalized = append(normalized, part)
	}
	parts = normalized
	name := partsPath("/", parts)
	if req.redirects >= 40 {
		return nil, &PolicyError{Kind: kind, Op: op, Target: name, Rule: "path-changed"}
	}
	grant, _, err := p.fileGrant(kind, op, name)
	if err != nil {
		return nil, err
	}
	prefix := 0
	if grant.Path != "/" {
		prefix = len(strings.Split(strings.TrimPrefix(grant.Path, "/"), "/"))
	}
	pending := parts[prefix:]
	if len(pending) == 0 {
		pending = []filePart{{name: "."}}
	}
	if grant.FS != nil {
		return p.openFSFile(grant, pending, name, req)
	}
	root, err := os.OpenRoot(grant.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := p.openRootFile(root, grant, pending, name, req)
	if err != nil {
		if rootEscapeError(err) {
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

// os.Root has no exported escape-error sentinel. Match its exact current
// error text; an unknown future error remains a failure, never an allowance.
func rootEscapeError(err error) bool {
	var pe *fs.PathError
	return errors.As(err, &pe) && pe.Err.Error() == "path escapes from parent"
}
