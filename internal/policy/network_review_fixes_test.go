package policy

import (
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"testing"
)

func TestReviewFixPublicAddressClassification(t *testing.T) {
	tests := []struct {
		address string
		public  bool
	}{
		{"64:ff9b::a00:1", false},
		{"64:ff9b::7f00:1", false},
		{"64:ff9b::a9fe:a9fe", false},
		{"2002:a00:1::1", false},
		{"2002:7f00:1::1", false},
		{"2002::1", false},
		{"::ffff:0:a00:1", false},
		{"::ffff:0:808:808", false},
		{"3fff::1", false},
		{"5f00::1", false},
		{"192.88.99.1", false},
		{"2001::1", false},
		{"64:ff9b:1::1", false},
		{"64:ff9b::808:808", true},
		{"2002:808:808::1", true},
		{"2606:4700::1111", true},
		{"8.8.8.8", true},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			address, err := netip.ParseAddr(tt.address)
			if err != nil {
				t.Fatalf("ParseAddr(%q): %v", tt.address, err)
			}
			if got := Public(address); got != tt.public {
				t.Errorf("Public(%s) = %v, want %v", address, got, tt.public)
			}
		})
	}
}

func TestReviewFixNetworkHostValidationAndNormalization(t *testing.T) {
	longName := strings.Join([]string{
		strings.Repeat("a", 63),
		strings.Repeat("b", 63),
		strings.Repeat("c", 63),
		strings.Repeat("d", 61),
	}, ".")
	if len(longName) != 253 {
		t.Fatalf("test control name length = %d, want 253", len(longName))
	}

	hosts := []struct {
		input string
		want  string
	}{
		{"example.com", "example.com"},
		{"EXAMPLE.com", "example.com"},
		{"api.example.com.", "api.example.com"},
		{"*.example.com", "*.example.com"},
		{"*.com", "*.com"},
		{"under_score.example", "under_score.example"},
		{"1.2.3.4", "1.2.3.4"},
		{"::1", "::1"},
		{"fe80::1%En0", "fe80::1%En0"},
		{"::ffff:1.2.3.4", "::ffff:1.2.3.4"},
		{strings.Repeat("a", 63) + ".example", strings.Repeat("a", 63) + ".example"},
		{longName, longName},
	}

	for _, tt := range hosts {
		t.Run(tt.input, func(t *testing.T) {
			p, err := New(Spec{Network: []NetworkGrant{{
				Scheme: "https",
				Host:   tt.input,
				Ports:  []int{443},
			}}})
			if err != nil {
				t.Fatalf("New rejected valid host %q: %v", tt.input, err)
			}
			if got := p.network[0].Host; got != tt.want {
				t.Errorf("stored Host = %q, want normalized %q", got, tt.want)
			}
		})
	}
}

func TestReviewFixInvalidNetworkHostsReturnSpecError(t *testing.T) {
	tooLongName := strings.Join([]string{
		strings.Repeat("a", 63),
		strings.Repeat("b", 63),
		strings.Repeat("c", 63),
		strings.Repeat("d", 62),
	}, ".")
	if len(tooLongName) != 254 {
		t.Fatalf("test invalid name length = %d, want 254", len(tooLongName))
	}

	hosts := []struct {
		input string
		want  string
	}{
		{"example.com:443", "example.com:443"},
		{"1.2.3.4:443", "1.2.3.4:443"},
		{"[::1]", "[::1]"},
		{"exa mple.com", "exa mple.com"},
		{"héllo.example", "héllo.example"},
		{"Kelvin.example", "Kelvin.example"},
		{"a..com", "a..com"},
		{".a.com", ".a.com"},
		{"-a.com", "-a.com"},
		{"a-.com", "a-.com"},
		{"*.", "*"},
		{"*", "*"},
		{"*.*.a.com", "*.*.a.com"},
		{"a.*.com", "a.*.com"},
		{"999.1.1.1", "999.1.1.1"},
		{strings.Repeat("a", 64) + ".example", strings.Repeat("a", 64) + ".example"},
		{tooLongName, tooLongName},
	}

	for _, tt := range hosts {
		t.Run(tt.input, func(t *testing.T) {
			_, err := New(Spec{Network: []NetworkGrant{{
				Scheme: "https",
				Host:   tt.input,
				Ports:  []int{443},
			}}})
			var specErr *SpecError
			if !errors.As(err, &specErr) {
				t.Fatalf("New error = %v, want *SpecError", err)
			}
			if specErr.Field != "Network[0]" {
				t.Errorf("SpecError.Field = %q, want Network[0]", specErr.Field)
			}
			wantReason := fmt.Sprintf("host %q is not a host name, address or *.domain pattern", tt.want)
			if specErr.Reason != wantReason {
				t.Errorf("SpecError.Reason = %q, want %q", specErr.Reason, wantReason)
			}
		})
	}
}

func TestReviewFixNormalizeHostDoesNotFoldUnicode(t *testing.T) {
	if got := normalizeHost("Kelvin.example"); got != "Kelvin.example" {
		t.Fatalf("Unicode normalized to %q", got)
	}
}

func TestReviewFixIPv6TrailingZoneDotRemainsDistinct(t *testing.T) {
	p, err := New(Spec{Network: []NetworkGrant{{Scheme: "tcp", Host: "fe80::1%En0.", Ports: []int{80}, AllowPrivate: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.networkGrant("tcp", "fe80::1%En0.", 80); err != nil {
		t.Errorf("same zone refused: %v", err)
	}
	if _, err := p.networkGrant("tcp", "fe80::1%En0", 80); err == nil {
		t.Error("different zone was granted")
	}
}
