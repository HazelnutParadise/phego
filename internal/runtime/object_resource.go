package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// REQ-019/020; T-037/038/039/040. Resource values reuse the existing
// identity pointer field, avoiding an extra pointer in every PHP value.
// The registry retains metadata, never logical ownership of the resource.
type phpResource struct {
	identity   *object
	generation uint64
	kind       string
	ctx        context.Context
	close      func(context.Context) error
	closed     bool
}

func (h *heap) newResource(ctx context.Context, kind string, close func(context.Context) error) (value, error) {
	if err := h.runtimeCheck(); err != nil {
		return value{}, err
	}
	if ctx == nil || kind == "" || close == nil {
		return value{}, errors.New("resource requires a context, kind and close callback")
	}
	if err := ctx.Err(); err != nil {
		return value{}, err
	}
	if h.resources == nil {
		h.resources = make(map[uint64]*phpResource)
	}
	h.nextResource++
	identity := &object{owner: h, handle: h.nextResource}
	h.resources[identity.handle] = &phpResource{identity: identity, generation: identity.handle, kind: kind, ctx: ctx, close: close}
	return value{kind: valueResource, object: identity}, nil
}

func (h *heap) resourceRecord(v value) (*phpResource, error) {
	v = v.deref()
	if v.kind != valueResource || v.object == nil || v.object.owner != h {
		return nil, fmt.Errorf("resource does not belong to this request")
	}
	r := h.resources[v.object.handle]
	if r == nil || r.identity != v.object || r.generation != v.object.handle {
		return nil, fmt.Errorf("resource generation is no longer valid")
	}
	return r, nil
}

func (h *heap) checkResource(v value, kind string) error {
	if h.closed {
		return ErrHeapClosed
	}
	r, err := h.resourceRecord(v)
	if err != nil {
		return err
	}
	if r.closed || r.kind != kind {
		return fmt.Errorf("expected an open %s resource", kind)
	}
	return r.ctx.Err()
}

func closeResourceRecord(r *phpResource, ctx context.Context) (err error) {
	if r.closed {
		return nil
	}
	// Commit closed before invoking host code, including reentrant close.
	r.closed = true
	r.identity.freed = true
	close := r.close
	r.close = nil
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("resource close panicked: %v", p)
		}
	}()
	return close(ctx)
}

func (h *heap) closeResource(v value, kind string, ctx context.Context) error {
	if ctx == nil {
		return errors.New("resource cleanup context is required")
	}
	if err := h.checkResource(v, kind); err != nil {
		return err
	}
	r, _ := h.resourceRecord(v)
	return closeResourceRecord(r, ctx)
}

func (h *heap) releaseResource(v value) error {
	r, err := h.resourceRecord(v)
	if err != nil {
		return err
	}
	v.object.refcount--
	if v.object.refcount > 0 {
		return nil
	}
	ctx := r.ctx
	if h.execution != nil && h.execution.cleanupContext != nil {
		ctx = h.execution.cleanupContext
	}
	err = closeResourceRecord(r, ctx)
	delete(h.resources, v.object.handle)
	return err
}

func (h *heap) closeResources(ctx context.Context) error {
	ids := make([]uint64, 0, len(h.resources))
	for id := range h.resources {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var errs []error
	for _, id := range ids {
		r := h.resources[id]
		if r == nil {
			continue
		} // an earlier host cleanup may have released it
		if err := closeResourceRecord(r, ctx); err != nil {
			errs = append(errs, err)
		}
	}
	clear(h.resources)
	return errors.Join(errs...)
}
