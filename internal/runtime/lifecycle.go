// Package runtime contains request-local lifecycle primitives.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// State identifies a request's lifecycle phase.
type State uint8

const (
	Received State = iota
	Admitted
	Initializing
	Ready
	Running
	Finishing
	Aborting
	ResourceCleanup
	Closed
)

// ErrCleanupInProgress prevents reentrant or concurrent cleanup from running twice.
var ErrCleanupInProgress = errors.New("request cleanup already in progress")

type resource struct {
	name  string
	close func(context.Context) error
}

// Request owns its permit and committed resources; it must not be copied after use.
type Request struct {
	mu         sync.Mutex
	state      State
	release    func(context.Context) error
	resources  []resource
	names      map[string]struct{}
	cleanupErr error
	required   map[string]bool
	completed  map[string]bool
}

// Handle is a request-local liveness token, not access to a resource.
type Handle struct{ owner *Request }

// InitStep declares a required initialization step. Resource steps are completed
// only by committing their resource; other steps require explicit completion.
type InitStep struct {
	Name     string
	Resource bool
}

func NewRequest(steps []InitStep) (*Request, error) {
	r := &Request{state: Received, names: make(map[string]struct{}), required: make(map[string]bool), completed: make(map[string]bool)}
	for _, step := range steps {
		if step.Name == "" {
			return nil, errors.New("initialization step name is required")
		}
		if _, exists := r.required[step.Name]; exists {
			return nil, fmt.Errorf("duplicate initialization step %q", step.Name)
		}
		r.required[step.Name] = step.Resource
	}
	return r, nil
}

// CompleteInit records a verified, non-resource initialization step.
func (r *Request) CompleteInit(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	resource, exists := r.required[name]
	if r.state != Initializing || !exists || resource || r.completed[name] {
		return fmt.Errorf("cannot complete initialization step %q", name)
	}
	r.completed[name] = true
	return nil
}

func (r *Request) State() State { r.mu.Lock(); defer r.mu.Unlock(); return r.state }

// Valid reports whether the owning request has begun cleanup.
func (h *Handle) Valid() bool {
	if h == nil || h.owner == nil {
		return false
	}
	h.owner.mu.Lock()
	defer h.owner.mu.Unlock()
	return h.owner.state != ResourceCleanup && h.owner.state != Closed
}

func (r *Request) transition(from, to State) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != from {
		return fmt.Errorf("invalid request transition %d -> %d", r.state, to)
	}
	r.state = to
	return nil
}

// Admit transfers ownership of an already acquired permit to this request.
func (r *Request) Admit(release func(context.Context) error) error {
	if release == nil {
		return errors.New("permit release is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != Received {
		return fmt.Errorf("cannot admit request in state %d", r.state)
	}
	r.release = release
	r.state = Admitted
	return nil
}

func (r *Request) BeginInit() error { return r.transition(Admitted, Initializing) }
func (r *Request) Ready() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != Initializing {
		return fmt.Errorf("cannot become ready in state %d", r.state)
	}
	for name := range r.required {
		if !r.completed[name] {
			return fmt.Errorf("initialization step %q incomplete", name)
		}
	}
	r.state = Ready
	return nil
}
func (r *Request) Start() error  { return r.transition(Ready, Running) }
func (r *Request) Finish() error { return r.transition(Running, Finishing) }

// CommitResource transfers ownership only after initialization has succeeded.
func (r *Request) CommitResource(name string, close func(context.Context) error) (*Handle, error) {
	if name == "" || close == nil {
		return nil, errors.New("resource name and cleanup are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state != Initializing {
		return nil, fmt.Errorf("cannot commit resource in state %d", r.state)
	}
	if _, exists := r.names[name]; exists {
		return nil, fmt.Errorf("duplicate resource %q", name)
	}
	if resource, required := r.required[name]; required {
		if !resource {
			return nil, fmt.Errorf("initialization step %q is not a resource", name)
		}
		r.completed[name] = true
	}
	r.names[name] = struct{}{}
	r.resources = append(r.resources, resource{name: name, close: close})
	return &Handle{owner: r}, nil
}

// Abort marks a request as failed or cancelled before cleanup.
func (r *Request) Abort() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch r.state {
	case Received, Admitted, Initializing, Ready, Running, Finishing:
		r.state = Aborting
		return nil
	case Aborting:
		return nil
	default:
		return fmt.Errorf("cannot abort request in state %d", r.state)
	}
}

func invoke(ctx context.Context, name string, fn func(context.Context) error) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("%s cleanup panicked: %v", name, value)
		}
	}()
	if e := fn(ctx); e != nil {
		return fmt.Errorf("%s cleanup: %w", name, e)
	}
	return nil
}

// Close invalidates handles and begins one reverse-order cleanup. A deadline
// bounds the caller's wait, not uncooperative host callbacks. If the deadline
// expires, cleanup remains in progress and the caller must isolate the worker.
func (r *Request) Close(ctx context.Context) error {
	if ctx == nil {
		return errors.New("cleanup context is required")
	}
	if _, ok := ctx.Deadline(); !ok {
		return errors.New("cleanup context requires a deadline")
	}
	r.mu.Lock()
	switch r.state {
	case Closed:
		err := r.cleanupErr
		r.mu.Unlock()
		return err
	case ResourceCleanup:
		err := r.cleanupErr
		r.mu.Unlock()
		if err != nil {
			return err
		}
		return ErrCleanupInProgress
	case Aborting, Finishing:
	default:
		state := r.state
		r.mu.Unlock()
		return fmt.Errorf("cannot clean request in state %d", state)
	}
	r.state = ResourceCleanup
	resources, release := r.resources, r.release
	r.resources, r.release = nil, nil
	r.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		var failures []error
		for i := len(resources) - 1; i >= 0; i-- {
			if err := invoke(ctx, resources[i].name, resources[i].close); err != nil {
				failures = append(failures, err)
			}
		}
		releaseFailed := false
		if release != nil {
			if err := invoke(ctx, "permit", release); err != nil {
				failures = append(failures, err)
				releaseFailed = true
			}
		}
		err := errors.Join(failures...)
		r.mu.Lock()
		r.cleanupErr = err
		if !releaseFailed {
			r.state = Closed
		}
		r.mu.Unlock()
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
