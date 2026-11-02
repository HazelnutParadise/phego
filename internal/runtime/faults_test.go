package runtime

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
)

// Fail after every committed initialization step and check reverse cleanup.
func TestInitializationFaultAtEveryCommitBoundary(t *testing.T) {
	const total = 4
	for completed := 0; completed <= total; completed++ {
		t.Run(fmt.Sprintf("completed_%d", completed), func(t *testing.T) {
			r := newTestRequest()
			returned := 0
			var closed []string
			if err := r.Admit(func(context.Context) error { returned++; return nil }); err != nil {
				t.Fatal(err)
			}
			if err := r.BeginInit(); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < completed; i++ {
				name := fmt.Sprint(i)
				if _, err := r.CommitResource(name, func(context.Context) error { closed = append(closed, name); return nil }); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.Abort(); err != nil {
				t.Fatal(err)
			}
			if err := r.Close(testCleanupContext); err != nil {
				t.Fatal(err)
			}
			var expected []string
			for i := completed - 1; i >= 0; i-- {
				expected = append(expected, fmt.Sprint(i))
			}
			if !reflect.DeepEqual(closed, expected) || returned != 1 {
				t.Fatalf("closed %v, want %v; permit returns %d", closed, expected, returned)
			}
			if err := r.Ready(); err == nil {
				t.Fatal("failed initialization reached ready")
			}
		})
	}
}

// A permit acquired before a cancellation race stays with exactly one owner.
func TestConcurrentAdmissionAndCancellation(t *testing.T) {
	for i := 0; i < 100; i++ {
		r := newTestRequest()
		var releases atomic.Int32
		var admitted atomic.Bool
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			if r.Admit(func(context.Context) error { releases.Add(1); return nil }) == nil {
				admitted.Store(true)
			}
		}()
		go func() { defer wg.Done(); <-start; _ = r.Abort() }()
		close(start)
		wg.Wait()
		if err := r.Close(testCleanupContext); err != nil {
			t.Fatal(err)
		}
		want := int32(0)
		if admitted.Load() {
			want = 1
		}
		if got := releases.Load(); got != want {
			t.Fatalf("releases %d, want %d", got, want)
		}
	}
}
