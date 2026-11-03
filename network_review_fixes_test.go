package phego

import "testing"

func TestReviewFixNewEngineRejectsMalformedNetworkHost(t *testing.T) {
	for _, host := range []string{"example.com:443", "1.2.3.4:443", "[::1]", "exa mple.com", "héllo.example", "Kelvin.example", "a..com", "999.1.1.1"} {
		_, err := NewEngine(Config{Policy: Policy{Network: []NetworkGrant{{Scheme: "https", Host: host, Ports: []int{443}}}}})
		wantConfigField(t, err, "Policy.Network[0]")
	}
}
