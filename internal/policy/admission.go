package policy

import (
	"context"
	"fmt"
	"sync"
)

// Admission bounds how many executions run and wait at once. Work beyond
// both bounds is refused before it starts, so neither goroutines nor
// buffered request bodies grow without bound.
type Admission struct {
	slots    chan struct{}
	mu       sync.Mutex
	queued   int
	maxQueue int
	closed   bool
}

// NewAdmission returns an admission gate for maxActive running and
// maxQueued waiting executions. maxActive must be positive; maxQueued may
// be zero to refuse instead of waiting.
func NewAdmission(maxActive, maxQueued int) (*Admission, error) {
	if maxActive < 1 {
		return nil, &SpecError{Field: "MaxActive", Reason: fmt.Sprintf("%d is not positive", maxActive)}
	}
	if maxQueued < 0 {
		return nil, &SpecError{Field: "MaxQueued", Reason: fmt.Sprintf("%d is negative", maxQueued)}
	}
	return &Admission{slots: make(chan struct{}, maxActive), maxQueue: maxQueued}, nil
}

// ErrAdmissionClosed reports admission after Close.
var ErrAdmissionClosed = &PolicyError{Kind: Service, Op: "admit", Target: "engine", Rule: "closed"}

// Acquire waits for a slot until ctx ends. The returned release function
// gives the slot back; calling it more than once returns the slot once.
func (a *Admission) Acquire(ctx context.Context) (release func(), err error) {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil, ErrAdmissionClosed
	}
	select {
	case a.slots <- struct{}{}:
		a.mu.Unlock()
		return a.releaser(), nil
	default:
	}
	if a.queued >= a.maxQueue {
		used := int64(len(a.slots) + a.queued)
		a.mu.Unlock()
		return nil, &ResourceError{Resource: Executions, Limit: int64(cap(a.slots) + a.maxQueue), Used: used, Requested: 1}
	}
	a.queued++
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.queued--
		a.mu.Unlock()
	}()
	select {
	case a.slots <- struct{}{}:
		a.mu.Lock()
		closed := a.closed
		a.mu.Unlock()
		if closed {
			<-a.slots
			return nil, ErrAdmissionClosed
		}
		return a.releaser(), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Admission) releaser() func() {
	var once sync.Once
	return func() { once.Do(func() { <-a.slots }) }
}

// Close refuses new executions. Running ones keep their slots until they
// release them.
func (a *Admission) Close() {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()
}

// Active returns the number of executions holding a slot.
func (a *Admission) Active() int { return len(a.slots) }
