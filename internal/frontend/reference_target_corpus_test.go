package frontend

import (
	"encoding/json"
	"os"
	"testing"
)

// The expectations were obtained with TOKEN_PARSE from the pinned PHP image.
func TestReferenceTargetsPHP8512(t *testing.T) {
	data, err := os.ReadFile("testdata/php8512-reference-targets.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		PHPVersion string `json:"php_version"`
		Platform   string `json:"platform"`
		Image      string `json:"image"`
		Cases      []struct {
			Source   string `json:"source"`
			Accepted bool   `json:"accepted"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.PHPVersion != "8.5.12" || fixture.Platform != "linux/386" || fixture.Image != "i386/php@sha256:07e835693e041c2a79cd7a3a7a0fda4b2a8d65c6f93bd89bb8dad5c616f28049" || len(fixture.Cases) != 448 {
		t.Fatal("unexpected PHP reference fixture")
	}
	for _, tc := range fixture.Cases {
		_, err := Parse([]byte(tc.Source), LexOptions{ShortOpenTag: true, IntWidth: 32, MaxBytes: 1 << 22})
		if (err == nil) != tc.Accepted {
			t.Errorf("%q: accepted=%t, PHP accepted=%t; error: %v", tc.Source, err == nil, tc.Accepted, err)
		}
	}
}
