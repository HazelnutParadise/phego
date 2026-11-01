package runtime

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

func TestInitializationFailureCleansOnlyCommittedResources(t *testing.T) {
	r := newTestRequest()
	var released int
	var closed []string
	if err := r.Admit(func(context.Context) error { released++; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	h, err := r.CommitResource("first", func(context.Context) error { closed = append(closed, "first"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(testCleanupContext); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(testCleanupContext); err != nil {
		t.Fatal(err)
	}
	if released != 1 || !reflect.DeepEqual(closed, []string{"first"}) || h.Valid() {
		t.Fatalf("release=%d closed=%v valid=%v", released, closed, h.Valid())
	}
	if r.State() != Closed {
		t.Fatalf("state=%v", r.State())
	}
}

func TestNoExecutionBeforeReadyAndNoReadyAfterAbort(t *testing.T) {
	r := newTestRequest()
	if err := r.Start(); err == nil {
		t.Fatal("started before initialization")
	}
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if err := r.Start(); err == nil {
		t.Fatal("started before ready")
	}
	if err := r.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := r.Ready(); err == nil {
		t.Fatal("aborted request became ready")
	}
	if err := r.Close(testCleanupContext); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupFailureAndPanicStillReleaseOtherResourcesAndPermit(t *testing.T) {
	r := newTestRequest()
	var order []string
	if err := r.Admit(func(context.Context) error { order = append(order, "permit"); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("close failed")
	if _, err := r.CommitResource("first", func(context.Context) error { order = append(order, "first"); return failure }); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitResource("second", func(context.Context) error { order = append(order, "second"); panic("broken") }); err != nil {
		t.Fatal(err)
	}
	if err := r.Abort(); err != nil {
		t.Fatal(err)
	}
	err := r.Close(testCleanupContext)
	if !errors.Is(err, failure) || err == nil {
		t.Fatalf("missing failure: %v", err)
	}
	if !reflect.DeepEqual(order, []string{"second", "first", "permit"}) {
		t.Fatalf("order=%v", order)
	}
	if r.Close(testCleanupContext) == nil || len(order) != 3 {
		t.Fatalf("repeated close: %v", order)
	}
}

func TestCancelBeforeAdmissionDoesNotReleasePermit(t *testing.T) {
	r := newTestRequest()
	if err := r.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(testCleanupContext); err != nil {
		t.Fatal(err)
	}
	if err := r.Admit(func(context.Context) error { t.Error("unacquired permit released"); return nil }); err == nil {
		t.Fatal("admitted closed request")
	}
}

func TestStaleHandlesStayInvalidAndRequestsAreIndependent(t *testing.T) {
	a, b := newTestRequest(), newTestRequest()
	for _, r := range []*Request{a, b} {
		if err := r.Admit(func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if err := r.BeginInit(); err != nil {
			t.Fatal(err)
		}
	}
	old, err := a.CommitResource("resource", func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	current, err := b.CommitResource("resource", func(context.Context) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(testCleanupContext); err != nil {
		t.Fatal(err)
	}
	if old.Valid() || !current.Valid() {
		t.Fatal("handle crossed request lifetime")
	}
	if err := b.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(testCleanupContext); err != nil {
		t.Fatal(err)
	}
}

func TestReentrantAndConcurrentCloseDoNotRepeatCleanup(t *testing.T) {
	r := newTestRequest()
	entered, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	if err := r.Admit(func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.BeginInit(); err != nil {
		t.Fatal(err)
	}
	if _, err := r.CommitResource("one", func(context.Context) error {
		once.Do(func() { close(entered) })
		if !errors.Is(r.Close(testCleanupContext), ErrCleanupInProgress) {
			t.Error("reentrant cleanup not rejected")
		}
		<-unblock
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.Abort(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- r.Close(testCleanupContext) }()
	<-entered
	if !errors.Is(r.Close(testCleanupContext), ErrCleanupInProgress) {
		t.Fatal("concurrent cleanup not rejected")
	}
	close(unblock)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := r.Close(testCleanupContext); err != nil {
		t.Fatal(err)
	}
}
