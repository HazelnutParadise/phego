package runtime

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// REQ-018; T-035. A heap made for this host must initialize lazy comparison
// operands in the order PHP 8.5.12 uses on the same system. The host-profile
// workflow runs tests/semantics/w13-objects/lazy-operand-order.php with that
// PHP and names its output in PHEGO_HOST_LAZY_ORDER.
func TestHostProfileLazyOperandOrder(t *testing.T) {
	file := os.Getenv("PHEGO_HOST_LAZY_ORDER")
	if file == "" {
		t.Skip("set PHEGO_HOST_LAZY_ORDER to the output of lazy-operand-order.php on this system")
	}
	observed, err := os.ReadFile(file)
	must(t, err)
	var want any
	must(t, json.Unmarshal(bytes.TrimSpace(observed), &want))
	h, err := newHeap(64)
	must(t, err)
	got, err := json.Marshal(w13ReviewCase(t, h, "compare-lazy-operand-owners").Result)
	must(t, err)
	wantJSON, err := json.Marshal(want)
	must(t, err)
	if !bytes.Equal(got, wantJSON) {
		t.Fatalf("host profile %s/%s: Phego %s, PHP 8.5.12 on this system %s", h.os, h.arch, got, wantJSON)
	}
}
