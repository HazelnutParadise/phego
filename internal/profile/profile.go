// Package profile identifies the exact PHP semantics a compilation or
// execution uses. A profile is the release plus the platform properties PHP
// code can observe, and it is part of the identity of every compiled artifact
// so a program compiled for one profile never runs under another.
package profile

import "fmt"

// Profile is an immutable PHP language profile.
type Profile struct {
	// Release is the exact PHP release, for example "8.5.12".
	Release string
	// IntWidth is the guest integer width in bits: 32 or 64.
	IntWidth int
	// ShortOpenTag mirrors the short_open_tag INI setting, which changes how
	// source is parsed.
	ShortOpenTag bool
}

// Key is the profile identity used by caches and compiled artifacts. Two
// profiles share a key only when every field is equal.
func (p Profile) Key() string {
	return fmt.Sprintf("php-%s/int%d/short_open_tag=%t", p.Release, p.IntWidth, p.ShortOpenTag)
}

func (p Profile) String() string {
	tag := "off"
	if p.ShortOpenTag {
		tag = "on"
	}
	return fmt.Sprintf("PHP %s with %d-bit integers and short_open_tag %s", p.Release, p.IntWidth, tag)
}

// Generation is the language generation of the release, such as "php8".
func (p Profile) Generation() string {
	if r, ok := find(p.Release); ok {
		return r.generation
	}
	return ""
}

type knownRelease struct {
	version    string
	generation string
}

// implemented lists the releases whose syntax and operator semantics this
// build implements. Every other release is rejected rather than approximated
// by a nearby one.
var implemented = map[string]bool{"8.5.12": true}

// Status classifies a requested release.
type Status uint8

const (
	// Unrecorded means the version is not recorded in the PHP release
	// inventory. The inventory is still incomplete, so this does not prove
	// that no such release exists; it means this build cannot confirm it.
	Unrecorded Status = iota
	// NotImplemented means the release exists but this build cannot run it
	// yet. It stays inside the compatibility goal.
	NotImplemented
	// Implemented means this build provides the release's semantics.
	Implemented
)

func (s Status) String() string {
	switch s {
	case NotImplemented:
		return "not implemented"
	case Implemented:
		return "implemented"
	}
	return "unrecorded"
}

func find(version string) (knownRelease, bool) {
	for _, r := range knownReleases {
		if r.version == version {
			return r, true
		}
	}
	return knownRelease{}, false
}

// ReleaseStatus reports what this build can do with a release.
func ReleaseStatus(version string) Status {
	if _, ok := find(version); !ok {
		return Unrecorded
	}
	if implemented[version] {
		return Implemented
	}
	return NotImplemented
}

// Error explains why a requested profile cannot be used.
type Error struct {
	Release  string
	IntWidth int
	Status   Status // Unrecorded or NotImplemented for release errors
	Reason   string
}

func (e *Error) Error() string { return e.Reason }

// Resolve returns the profile for an exact release, integer width, and
// short_open_tag setting. An unrecorded version, a known but unimplemented
// release, and an unsupported width are distinct errors; none falls back to
// another release.
func Resolve(release string, intWidth int, shortOpenTag bool) (Profile, error) {
	switch ReleaseStatus(release) {
	case Unrecorded:
		return Profile{}, &Error{Release: release, IntWidth: intWidth, Status: Unrecorded,
			Reason: fmt.Sprintf("PHP release %q is not recorded in the PHP release inventory, which is still incomplete", release)}
	case NotImplemented:
		return Profile{}, &Error{Release: release, IntWidth: intWidth, Status: NotImplemented,
			Reason: fmt.Sprintf("PHP release %s is a known release that this build does not implement yet", release)}
	}
	if intWidth != 32 && intWidth != 64 {
		return Profile{}, &Error{Release: release, IntWidth: intWidth, Status: Implemented,
			Reason: fmt.Sprintf("integer width %d is not 32 or 64", intWidth)}
	}
	return Profile{Release: release, IntWidth: intWidth, ShortOpenTag: shortOpenTag}, nil
}

// ImplementedReleases returns the releases this build implements.
func ImplementedReleases() []string {
	var out []string
	for _, r := range knownReleases {
		if implemented[r.version] {
			out = append(out, r.version)
		}
	}
	return out
}
