package policy

import (
	"path/filepath"
	"strings"
)

// absoluteLinkMount translates an absolute host target into a granted PHP
// path. Only the trusted mount root is canonicalized: cleaning the target
// would discard symlink and parent-component semantics before authorization.
// The longest physical prefix wins; ties prefer the current mount, then the
// first configured mount. Existing physical protections still apply.
func (p *Policy) absoluteLinkMount(target, current string) (string, []filePart, bool) {
	type physicalMount struct {
		grant FileGrant
		root  []string
	}
	var mounts []physicalMount
	raw := hostPathComponents(target)
	components := raw
	aliasDepth := -1
	for _, grant := range p.files {
		if grant.Dir == "" {
			continue
		}
		root, err := filepath.EvalSymlinks(grant.Dir)
		if err != nil {
			continue
		}
		canonical := hostPathComponents(root)
		mounts = append(mounts, physicalMount{grant, canonical})
		// A configured root may itself use a host alias, such as /var on
		// macOS. Canonicalize only that verified prefix, never its suffix.
		prefix := hostPathComponents(grant.Dir)
		if len(prefix) > aliasDepth && hasPathComponents(raw, prefix) {
			components = append(append([]string(nil), canonical...), raw[len(prefix):]...)
			aliasDepth = len(prefix)
		}
	}
	best := -1
	var selected FileGrant
	for _, mount := range mounts {
		depth := len(mount.root)
		if depth < best || !hasPathComponents(components, mount.root) {
			continue
		}
		if depth == best && (selected.Path == current || mount.grant.Path != current) {
			continue
		}
		best, selected = depth, mount.grant
	}
	if best < 0 {
		return "", nil, false
	}
	parts := make([]filePart, len(components)-best)
	for i, component := range components[best:] {
		parts[i] = filePart{component, selected.Path}
	}
	if strings.HasSuffix(filepath.ToSlash(target), "/") {
		parts = append(parts, filePart{".", selected.Path})
	}
	return selected.Path, parts, true
}

func hasPathComponents(name, prefix []string) bool {
	if len(name) < len(prefix) {
		return false
	}
	for i, part := range prefix {
		if name[i] != part {
			return false
		}
	}
	return true
}

// Remove repeated separators only, preserving dots, parents and their order.
func hostPathComponents(name string) []string {
	var parts []string
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}
