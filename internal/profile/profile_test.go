package profile

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestReleasesMatchInventory(t *testing.T) {
	raw, err := os.ReadFile("../../docs/compatibility/php-release-inventory.json")
	if err != nil {
		t.Fatal(err)
	}
	var inv struct {
		Releases []struct {
			Version    string `json:"version"`
			Generation string `json:"generation"`
		} `json:"releases"`
	}
	if err := json.Unmarshal(raw, &inv); err != nil {
		t.Fatal(err)
	}
	if len(inv.Releases) != len(knownReleases) {
		t.Fatalf("inventory has %d releases, table has %d; run python3 tools/profile_releases.py", len(inv.Releases), len(knownReleases))
	}
	for i, r := range inv.Releases {
		if knownReleases[i] != (knownRelease{r.Version, r.Generation}) {
			t.Fatalf("release %d is %+v in the table and %+v in the inventory", i, knownReleases[i], r)
		}
	}
	for v := range implemented {
		if _, ok := find(v); !ok {
			t.Errorf("implemented release %q is not in the inventory", v)
		}
	}
}

func TestResolveClassifiesReleases(t *testing.T) {
	for _, tc := range []struct {
		release string
		width   int
		status  Status
	}{
		{"8.4.1", 64, NotImplemented}, // a real release not built yet
		{"1.0", 64, NotImplemented},   // the first public release stays in the goal
		{"8.5.99", 64, Unknown},       // no such release
		{"8.5", 64, Unknown},          // not an exact release
		{"", 64, Unknown},
		{"8.5.12", 16, Implemented}, // release is fine, width is not
	} {
		_, err := Resolve(tc.release, tc.width)
		var pe *Error
		if !errors.As(err, &pe) || pe.Status != tc.status {
			t.Errorf("Resolve(%q, %d) = %v, want status %v", tc.release, tc.width, err, tc.status)
		}
	}
	for _, w := range []int{32, 64} {
		p, err := Resolve("8.5.12", w)
		if err != nil || p.Generation() != "php8" {
			t.Fatalf("Resolve(8.5.12, %d) = %+v, %v", w, p, err)
		}
	}
}

func TestProfileKeysSeparateEveryField(t *testing.T) {
	a := Profile{Release: "8.5.12", IntWidth: 32}
	b := Profile{Release: "8.5.12", IntWidth: 64}
	c := Profile{Release: "8.4.1", IntWidth: 64}
	if a.Key() == b.Key() || b.Key() == c.Key() || a.Key() == c.Key() {
		t.Fatalf("keys collide: %s %s %s", a.Key(), b.Key(), c.Key())
	}
	if got := ImplementedReleases(); len(got) != 1 || got[0] != "8.5.12" {
		t.Fatalf("implemented releases %v", got)
	}
}
