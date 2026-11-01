package policy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestReviewReadOnlyMountShadowsWritableParent(t *testing.T) {
	for _, virtual := range []bool{false, true} {
		t.Run(map[bool]string{false: "directory", true: "fs"}[virtual], func(t *testing.T) {
			root := t.TempDir()
			protected := filepath.Join(root, "protected")
			if err := os.Mkdir(protected, 0700); err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(protected, "file")
			if err := os.WriteFile(name, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			child := FileGrant{Path: "/protected", Dir: protected}
			if virtual {
				child.Dir = ""
				child.FS = fstest.MapFS{"file": {Data: []byte("virtual")}}
			}
			p := mustPolicy(t, Spec{Files: []FileGrant{{Path: "/", Dir: root, Write: true}, child}})
			for _, flag := range []int{os.O_WRONLY, os.O_RDWR, os.O_WRONLY | os.O_TRUNC, os.O_CREATE | os.O_WRONLY, os.O_APPEND | os.O_WRONLY} {
				f, err := p.OpenFile("/protected/file", flag, 0600)
				if f != nil {
					f.Close()
				}
				wantPolicyError(t, err, "not-granted")
			}
			got, err := os.ReadFile(name)
			if err != nil || string(got) != "original" {
				t.Fatalf("protected data=%q err=%v", got, err)
			}
		})
	}
}

func TestReviewCancelledAdmissionDoesNotTakeFreeSlot(t *testing.T) {
	a, _ := NewAdmission(1, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	release, err := a.Acquire(ctx)
	if release != nil {
		release()
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled free-slot acquire: %v", err)
	}
	if a.Active() != 0 {
		t.Fatal("cancelled caller retained slot")
	}
}

func TestReviewCloseWakesQueuedAdmission(t *testing.T) {
	a, _ := NewAdmission(1, 1)
	release, _ := a.Acquire(context.Background())
	defer release()
	done := make(chan error, 1)
	go func() {
		r, e := a.Acquire(context.Background())
		if r != nil {
			r()
		}
		done <- e
	}()
	deadline := time.After(2 * time.Second)
	for {
		a.mu.Lock()
		q := a.queued
		a.mu.Unlock()
		if q == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("caller did not queue")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	a.Close()
	a.Close()
	select {
	case err := <-done:
		if !errors.Is(err, ErrAdmissionClosed) {
			t.Fatalf("close: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Close did not wake queued caller")
	}
	if a.Active() != 1 {
		t.Fatalf("Close released running slot: %d", a.Active())
	}
}

func TestReviewAbsoluteDNSGrants(t *testing.T) {
	for _, host := range []string{"api.example.com.", "*.example.com."} {
		p := mustPolicy(t, Spec{Network: []NetworkGrant{{Scheme: "https", Host: host, Ports: []int{443}}}})
		for _, request := range []string{"api.example.com", "api.example.com."} {
			if _, err := p.networkGrant("https", request, 443); err != nil {
				t.Errorf("grant %q request %q: %v", host, request, err)
			}
		}
	}
}

func TestReviewNetworkBypassInputs(t *testing.T) {
	p := mustPolicy(t, Spec{Network: []NetworkGrant{{Scheme: "https", Host: "*.example.com", Ports: []int{443}}}})
	for _, host := range []string{"example.com", "evilexample.com", "api.example.com.evil", "api.example.com@127.0.0.1", "127.0.0.1", "::ffff:127.0.0.1"} {
		_, err := p.DialContext(context.Background(), "https", "tcp", host, 443)
		wantPolicyError(t, err, "not-granted")
	}
	for _, port := range []int{-1, 0, 80, 65536} {
		_, err := p.DialContext(context.Background(), "https", "tcp", "api.example.com", port)
		wantPolicyError(t, err, "not-granted")
	}
	for _, host := range []string{"127.0.0.1", "::ffff:127.0.0.1", "::1"} {
		q := mustPolicy(t, Spec{Network: []NetworkGrant{{Scheme: "tcp", Host: host, Ports: []int{12345}}}})
		_, err := q.DialContext(context.Background(), "tcp", "tcp", host, 12345)
		wantPolicyError(t, err, "private-address")
	}
}

func TestReviewQueuedCancellationReturnsCapacity(t *testing.T) {
	for i := 0; i < 100; i++ {
		a, _ := NewAdmission(1, 1)
		held, _ := a.Acquire(context.Background())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		r, err := a.Acquire(ctx)
		if r != nil {
			r()
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel: %v", err)
		}
		held()
		held()
		next, err := a.Acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		next()
		if a.Active() != 0 {
			t.Fatal("leaked capacity")
		}
	}
}

func TestReviewRootAndRootFSConfineSymlinks(t *testing.T) {
	base := t.TempDir()
	app := filepath.Join(base, "app")
	if err := os.Mkdir(app, 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"app/ok": "ok", "outside": "outside"} {
		if err := os.WriteFile(filepath.Join(base, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("../outside", filepath.Join(app, "escape")); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(app)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for _, grant := range []FileGrant{{Path: "/app", Dir: app}, {Path: "/app", FS: root.FS()}} {
		p := mustPolicy(t, Spec{Files: []FileGrant{grant}})
		f, err := p.OpenFile("/app/ok", os.O_RDONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		for _, name := range []string{"/app/escape", "/app/../outside", "/application/ok"} {
			f, err := p.OpenFile(name, os.O_RDONLY, 0)
			if f != nil {
				f.Close()
			}
			if err == nil {
				t.Fatalf("escaped confinement with %q", name)
			}
		}
	}
}
