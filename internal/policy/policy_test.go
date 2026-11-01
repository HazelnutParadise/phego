package policy

import (
	"context"
	"errors"
	"math"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func mustPolicy(t *testing.T, spec Spec) *Policy {
	t.Helper()
	p, err := New(spec)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func wantPolicyError(t *testing.T, err error, rule string) {
	t.Helper()
	var pe *PolicyError
	if !errors.As(err, &pe) || pe.Rule != rule {
		t.Fatalf("got %v, want PolicyError with rule %q", err, rule)
	}
}

func TestZeroSpecGrantsNothing(t *testing.T) {
	p := mustPolicy(t, Spec{})
	for _, k := range []Kind{Database, Process, Shell, DynamicCode, Callback, Service} {
		wantPolicyError(t, p.Allow(k, "use", "x"), "not-granted")
	}
	_, err := p.OpenFile("/etc/passwd", os.O_RDONLY, 0)
	wantPolicyError(t, err, "outside-root")
	_, err = p.DialContext(context.Background(), "https", "tcp", "example.com", 443)
	wantPolicyError(t, err, "not-granted")
}

func TestSpecValidation(t *testing.T) {
	bad := []Spec{
		{Mode: 9},
		{Files: []FileGrant{{Root: "relative/dir"}}},
		{Files: []FileGrant{{Root: ""}}},
		{Network: []NetworkGrant{{Scheme: "https://", Host: "a.com", Ports: []int{443}}}},
		{Network: []NetworkGrant{{Scheme: "https", Host: "a.com/x", Ports: []int{443}}}},
		{Network: []NetworkGrant{{Scheme: "https", Host: "a.*.com", Ports: []int{443}}}},
		{Network: []NetworkGrant{{Scheme: "https", Host: "a.com"}}},
		{Network: []NetworkGrant{{Scheme: "https", Host: "a.com", Ports: []int{0}}}},
		{Network: []NetworkGrant{{Scheme: "https", Host: "a.com", Ports: []int{65536}}}},
		{Databases: []string{""}},
		{Processes: []string{"bin/sh"}},
		{Processes: []string{"/usr/bin/../bin/env"}},
	}
	for i, spec := range bad {
		var se *SpecError
		if _, err := New(spec); !errors.As(err, &se) {
			t.Errorf("spec %d: got %v, want SpecError", i, err)
		}
	}
}

func TestCapabilitiesAreCheckedInEveryMode(t *testing.T) {
	for _, mode := range []Mode{Compat, Observe, Enforce} {
		p := mustPolicy(t, Spec{Mode: mode, Databases: []string{"main"}, Callbacks: []string{"Lookup"}, Processes: []string{"/usr/bin/convert"}})
		if err := p.Allow(Database, "connect", "main"); err != nil {
			t.Errorf("%s: granted database refused: %v", mode, err)
		}
		wantPolicyError(t, p.Allow(Database, "connect", "other"), "not-granted")
		if err := p.Allow(Callback, "call", "LOOKUP"); err != nil {
			t.Errorf("%s: callback names compare case-insensitively: %v", mode, err)
		}
		if err := p.Allow(Process, "exec", "/usr/bin/convert"); err != nil {
			t.Errorf("%s: granted process refused: %v", mode, err)
		}
		// An argument vector grant does not grant a shell command line.
		wantPolicyError(t, p.Allow(Shell, "exec", "/usr/bin/convert a.png b.jpg"), "not-granted")
		wantPolicyError(t, p.Allow(DynamicCode, "eval", ""), "not-granted")
	}
}

func TestRulesBlockOnlyInEnforceMode(t *testing.T) {
	for mode, blocked := range map[Mode]bool{Compat: false, Observe: false, Enforce: true} {
		p := mustPolicy(t, Spec{Mode: mode})
		err := p.RuleMatched(Database, "query", "main", "sql-structure-from-input")
		if (err != nil) != blocked {
			t.Errorf("%s: RuleMatched = %v, blocked want %v", mode, err, blocked)
		}
		if blocked {
			wantPolicyError(t, err, "sql-structure-from-input")
		}
	}
}

func TestFilesStayInsideGrantedTrees(t *testing.T) {
	base := t.TempDir()
	app := filepath.Join(base, "app")
	data := filepath.Join(app, "data")
	secret := filepath.Join(base, "secret")
	for _, d := range []string{app, data, secret} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(app, "index.php"), []byte("<?php"), 0o644)
	os.WriteFile(filepath.Join(secret, "key"), []byte("k"), 0o600)
	if err := os.Symlink(filepath.Join(secret, "key"), filepath.Join(app, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../../secret", filepath.Join(data, "up")); err != nil {
		t.Fatal(err)
	}
	p := mustPolicy(t, Spec{Files: []FileGrant{{Root: app}, {Root: data, Write: true}}})

	f, err := p.OpenFile(filepath.Join(app, "index.php"), os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("read inside the tree: %v", err)
	}
	f.Close()
	for _, name := range []string{
		filepath.Join(app, "..", "secret", "key"),
		filepath.Join(secret, "key"),
		"relative.php",
	} {
		if _, err := p.OpenFile(name, os.O_RDONLY, 0); err == nil {
			t.Errorf("%s: read outside the tree succeeded", name)
		}
	}
	// Symbolic links that point outside are refused by os.Root.
	for _, name := range []string{filepath.Join(app, "link"), filepath.Join(data, "up", "key")} {
		if _, err := p.OpenFile(name, os.O_RDONLY, 0); err == nil {
			t.Errorf("%s: symbolic link escape succeeded", name)
		}
	}
	// Writing needs a write grant: app is read-only, data is writable.
	_, err = p.OpenFile(filepath.Join(app, "new.txt"), os.O_CREATE|os.O_WRONLY, 0o644)
	wantPolicyError(t, err, "outside-root")
	w, err := p.OpenFile(filepath.Join(data, "cache.txt"), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("write in the writable tree: %v", err)
	}
	w.Close()
	if _, err := p.OpenFile(filepath.Join(app, "index.php"), os.O_RDWR, 0); err == nil {
		t.Error("read-write open of a read-only tree succeeded")
	}
}

func TestNetworkGrantsMatchSchemeHostAndPort(t *testing.T) {
	p := mustPolicy(t, Spec{Network: []NetworkGrant{
		{Scheme: "https", Host: "api.example.com", Ports: []int{443}},
		{Scheme: "https", Host: "*.cdn.example.com", Ports: []int{443, 8443}},
	}})
	for _, c := range []struct {
		scheme, host string
		port         int
		ok           bool
	}{
		{"https", "api.example.com", 443, true},
		{"HTTPS", "API.example.com.", 443, true},
		{"http", "api.example.com", 443, false},
		{"https", "api.example.com", 80, false},
		{"https", "evil.com", 443, false},
		{"https", "img.cdn.example.com", 8443, true},
		{"https", "cdn.example.com", 443, false},
		{"https", "xcdn.example.com", 443, false},
	} {
		_, err := p.networkGrant(c.scheme, c.host, c.port)
		if (err == nil) != c.ok {
			t.Errorf("%s://%s:%d: allowed=%v, want %v", c.scheme, c.host, c.port, err == nil, c.ok)
		}
	}
}

func TestDialChecksTheAddressActuallyUsed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	// "localhost" matches the grant by name, but resolves to a loopback
	// address, which the grant does not allow.
	p := mustPolicy(t, Spec{Network: []NetworkGrant{{Scheme: "tcp", Host: "localhost", Ports: []int{port}}}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = p.DialContext(ctx, "tcp", "tcp", "localhost", port)
	wantPolicyError(t, err, "private-address")

	p = mustPolicy(t, Spec{Network: []NetworkGrant{{Scheme: "tcp", Host: "127.0.0.1", Ports: []int{port}, AllowPrivate: true}}})
	conn, err := p.DialContext(ctx, "tcp", "tcp", "127.0.0.1", port)
	if err != nil {
		t.Fatalf("private address with AllowPrivate: %v", err)
	}
	conn.Close()
	_, err = p.DialContext(ctx, "tcp", "unix", "127.0.0.1", port)
	wantPolicyError(t, err, "unsupported-network")
}

func TestPublicAddresses(t *testing.T) {
	for addr, public := range map[string]bool{
		"8.8.8.8": true, "2606:4700::1111": true,
		"127.0.0.1": false, "10.1.2.3": false, "172.16.0.1": false, "192.168.1.1": false,
		"169.254.169.254": false, "100.64.0.1": false, "0.0.0.0": false, "224.0.0.1": false,
		"::1": false, "fe80::1": false, "fc00::1": false, "::ffff:127.0.0.1": false, "::": false,
		"192.0.2.1": false, "198.18.0.1": false, "255.255.255.255": false,
	} {
		if got := Public(netip.MustParseAddr(addr)); got != public {
			t.Errorf("Public(%s) = %v, want %v", addr, got, public)
		}
	}
}

func TestMeterRefusesWithoutCharging(t *testing.T) {
	m, err := NewMeter(Limits{Output: 10, OpenFiles: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Charge(Output, 8); err != nil {
		t.Fatal(err)
	}
	var re *ResourceError
	if err := m.Charge(Output, 3); !errors.As(err, &re) || re.Used != 8 || re.Limit != 10 || re.Requested != 3 {
		t.Fatalf("over budget: %v", err)
	}
	if err := m.Charge(Output, 2); err != nil {
		t.Fatalf("the refused charge must not have counted: %v", err)
	}
	if err := m.Charge(Steps, math.MaxInt64); err != nil {
		t.Fatal(err)
	}
	if err := m.Charge(Steps, 1); !errors.As(err, &re) {
		t.Fatalf("overflow must be refused, got %v", err)
	}
	// Gauges go back down; accumulating resources cannot be released.
	m.Charge(OpenFiles, 2)
	if err := m.Charge(OpenFiles, 1); !errors.As(err, &re) {
		t.Fatalf("third open file: %v", err)
	}
	if err := m.Release(OpenFiles, 1); err != nil {
		t.Fatal(err)
	}
	if err := m.Charge(OpenFiles, 1); err != nil {
		t.Fatalf("after release: %v", err)
	}
	var se *SpecError
	if err := m.Release(OpenFiles, 5); !errors.As(err, &se) {
		t.Errorf("over-release: %v", err)
	}
	if err := m.Release(Output, 1); !errors.As(err, &se) {
		t.Errorf("releasing output: %v", err)
	}
	if err := m.Charge(Output, -1); !errors.As(err, &se) {
		t.Errorf("negative charge: %v", err)
	}
	u := m.Usage()
	if u.Used[Output] != 10 || u.Peak[OpenFiles] != 2 || u.Used[OpenFiles] != 2 {
		t.Errorf("usage %+v", u)
	}
	for _, bad := range []Limits{{Output: -1}, {0: 1}, {resourceCount: 1}} {
		if _, err := NewMeter(bad); !errors.As(err, &se) {
			t.Errorf("NewMeter(%v): %v", bad, err)
		}
	}
}

func TestCheckpointStopsOnCancellationAndBudget(t *testing.T) {
	m, _ := NewMeter(Limits{Steps: 3})
	ctx, cancel := context.WithCancel(context.Background())
	for i := 0; i < 3; i++ {
		if err := m.Checkpoint(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var re *ResourceError
	if err := m.Checkpoint(ctx); !errors.As(err, &re) || re.Resource != Steps {
		t.Fatalf("step budget: %v", err)
	}
	cancel()
	if err := m.Checkpoint(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestMeterIsSafeForConcurrentCallbacks(t *testing.T) {
	m, _ := NewMeter(Limits{Output: 1000})
	var wg sync.WaitGroup
	var refused sync.Map
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				if err := m.Charge(Output, 1); err != nil {
					refused.Store(strconv.Itoa(i)+"/"+strconv.Itoa(j), true)
				}
			}
		}(i)
	}
	wg.Wait()
	if got := m.Usage().Used[Output]; got != 1000 {
		t.Errorf("used %d, want exactly the limit", got)
	}
}

func TestAdmissionBoundsActiveAndQueued(t *testing.T) {
	a, err := NewAdmission(2, 1)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	r1, _ := a.Acquire(ctx)
	r2, _ := a.Acquire(ctx)
	waiting := make(chan error, 1)
	go func() {
		release, err := a.Acquire(ctx)
		if err == nil {
			release()
		}
		waiting <- err
	}()
	// Wait until the third caller is queued, then the fourth is refused.
	deadline := time.Now().Add(5 * time.Second)
	for {
		a.mu.Lock()
		q := a.queued
		a.mu.Unlock()
		if q == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	var re *ResourceError
	if _, err := a.Acquire(ctx); !errors.As(err, &re) || re.Resource != Executions {
		t.Fatalf("over capacity: %v", err)
	}
	r1()
	r1() // releasing twice returns the slot once
	if err := <-waiting; err != nil {
		t.Fatalf("queued caller: %v", err)
	}
	if a.Active() != 1 {
		t.Errorf("active %d, want 1 after the double release", a.Active())
	}
	r2()
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	r3, _ := a.Acquire(ctx)
	r4, _ := a.Acquire(ctx)
	if _, err := a.Acquire(cctx); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled wait: %v", err)
	}
	a.Close()
	if _, err := a.Acquire(ctx); !errors.Is(err, ErrAdmissionClosed) {
		t.Errorf("after Close: %v", err)
	}
	r3()
	r4()
	var se *SpecError
	if _, err := NewAdmission(0, 0); !errors.As(err, &se) {
		t.Error("zero active slots accepted")
	}
	if _, err := NewAdmission(1, -1); !errors.As(err, &se) {
		t.Error("negative queue accepted")
	}
}

func TestLogIsBoundedAndRedacted(t *testing.T) {
	l := NewLog(2)
	l.Record(Event{Kind: Network, Op: "https", Target: "https://user:pw@api.example.com/x?token=abc&q=1#frag", Allowed: true})
	l.Record(Event{Kind: Database, Op: "connect", Target: "main"})
	l.Record(Event{Kind: Database, Op: "connect", Target: "main"})
	events, dropped := l.Events()
	if len(events) != 2 || dropped != 1 {
		t.Fatalf("events %d dropped %d", len(events), dropped)
	}
	got := events[0].Target
	if got != "https://%5Bredacted%5D@api.example.com/x?q=%5Bredacted%5D&token=%5Bredacted%5D" {
		t.Errorf("redacted target %q", got)
	}
	if Redact("%zz://x") != "[unparsable URL]" {
		t.Error("unparsable URL kept")
	}
}
