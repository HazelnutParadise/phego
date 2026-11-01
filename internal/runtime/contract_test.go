package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestReadyRequiresCommittedInitialization(t *testing.T) {
	steps := []InitStep{{Name: "config"}, {Name: "file", Resource: true}}
	r, err := NewRequest(steps)
	if err != nil {
		t.Fatal(err)
	}
	steps[0].Name = "changed"
	if err = r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err = r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if err = r.Ready(); err == nil {
		t.Fatal("ready with missing initialization")
	}
	if err = r.CompleteInit("config"); err != nil {
		t.Fatal(err)
	}
	if err = r.CompleteInit("file"); err == nil {
		t.Fatal("resource marked complete without commit")
	}
	if err = r.Ready(); err == nil {
		t.Fatal("ready without committed resource")
	}
	if _, err = r.CommitResource("file", func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err = r.Ready(); err != nil {
		t.Fatal(err)
	}
	if err = r.Abort(); err != nil {
		t.Fatal(err)
	}
	if err = r.Close(testCleanupContext); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupDeadlinePreservesPermitUntilCallbackStops(t *testing.T) {
	r, err := NewRequest(nil)
	if err != nil {
		t.Fatal(err)
	}
	released := make(chan struct{})
	entered := make(chan struct{})
	unblock := make(chan struct{})
	if err := r.Admit(func(context.Context) error { close(released); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	h, err := r.CommitResource("file", func(context.Context) error { close(entered); <-unblock; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Abort(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Close(ctx) }()
	<-entered
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	if h.Valid() || r.State() != ResourceCleanup {
		t.Fatal("cleanup state or handle invalidation incorrect")
	}
	select {
	case <-released:
		t.Fatal("permit released while cleanup still running")
	default:
	}
	if err := r.Close(testCleanupContext); !errors.Is(err, ErrCleanupInProgress) {
		t.Fatalf("reentry: %v", err)
	}
	close(unblock)
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("permit not released after cleanup")
	}
}

func TestInitializationStepValidation(t *testing.T) {
	for _, steps := range [][]InitStep{{{Name: ""}}, {{Name: "x"}, {Name: "x", Resource: true}}} {
		if _, err := NewRequest(steps); err == nil {
			t.Fatalf("accepted invalid steps: %v", steps)
		}
	}
	r, err := NewRequest([]InitStep{{Name: "file", Resource: true}, {Name: "config"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitResource("file", nil); err == nil {
		t.Fatal("committed resource without cleanup")
	}
	if err := r.CompleteInit("config"); err != nil {
		t.Fatal(err)
	}
	if err := r.CompleteInit("config"); err == nil {
		t.Fatal("duplicate completion accepted")
	}
	if err := r.Ready(); err == nil {
		t.Fatal("failed resource commit counted as complete")
	}
	if err := r.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(testCleanupContext); err != nil {
		t.Fatal(err)
	}
}

func TestReadyRacingAbortNeverRevivesRequest(t *testing.T) {
	for i := 0; i < 100; i++ {
		r, err := NewRequest([]InitStep{{Name: "config"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Admit(func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if err := r.BeginInit(); err != nil {
			t.Fatal(err)
		}
		if err := r.CompleteInit("config"); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; _ = r.Ready() }()
		go func() { defer wg.Done(); <-start; _ = r.Abort() }()
		close(start)
		wg.Wait()
		if r.State() != Aborting {
			t.Fatal("request did not remain aborted")
		}
		if err := r.Ready(); err == nil {
			t.Fatal("aborted request became ready")
		}
		if err := r.Close(testCleanupContext); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCleanupRequiresDeadline(t *testing.T) {
	r, err := NewRequest(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(context.Background()); err == nil {
		t.Fatal("cleanup accepted an unbounded context")
	}
	if r.State() != Aborting {
		t.Fatal("unbounded cleanup changed state")
	}
	if err := r.Close(testCleanupContext); err != nil {
		t.Fatal(err)
	}
}

func TestExpiredReleaseDoesNotClaimCleanupCompleted(t *testing.T) {
	r, err := NewRequest(nil)
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	unblock := make(chan struct{})
	attempted := make(chan struct{})
	if err := r.Admit(func(ctx context.Context) error { defer close(attempted); return ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitResource("file", func(context.Context) error { close(entered); <-unblock; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Abort(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Close(ctx) }()
	<-entered
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v", err)
	}
	close(unblock)
	<-attempted
	deadline := time.Now().Add(time.Second)
	for {
		err := r.Close(testCleanupContext)
		if errors.Is(err, context.DeadlineExceeded) {
			break
		}
		if !errors.Is(err, ErrCleanupInProgress) || time.Now().After(deadline) {
			t.Fatalf("cleanup failure lost: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	if r.State() != ResourceCleanup {
		t.Fatal("failed permit release was reported closed")
	}
}
