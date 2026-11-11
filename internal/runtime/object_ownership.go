package runtime

import "fmt"

// claimValues validates every incoming graph and destination before committing
// ownership of previously unclaimed arrays/references. It never calls PHP.
// Runtime-owned nodes may use a constant-time check because all writes preserve
// their request ownership. Low-level unclaimed builders require a graph walk.
func (h *heap) claimValues(values ...value) error {
	pending, err := h.pendingValues(values...)
	if err != nil {
		return err
	}
	h.commitValueOwnership(pending)
	return nil
}

func (h *heap) commitValueOwnership(pending []gcNode) {
	for _, n := range pending {
		switch n := n.(type) {
		case *phpArray:
			n.owner = h
		case *reference:
			n.owner = h
		}
	}
}

func (h *heap) pendingValues(values ...value) ([]gcNode, error) {
	var pending []gcNode
	var visited map[gcNode]struct{}
	var visit func(value) error
	visit = func(v value) error {
		switch v.kind {
		case valueArray:
			a := v.array
			if a == nil {
				return fmt.Errorf("nil array value")
			}
			if a.owner == h {
				return nil
			}
			if a.owner != nil {
				return fmt.Errorf("array belongs to another or closed request")
			}
			if _, ok := visited[a]; ok {
				return nil
			}
			if visited == nil {
				visited = make(map[gcNode]struct{})
			}
			visited[a] = struct{}{}
			pending = append(pending, a)
		case valueReference:
			r := v.ref
			if r == nil {
				return fmt.Errorf("nil reference value")
			}
			if r.owner == h {
				return nil
			}
			if r.owner != nil {
				return fmt.Errorf("reference belongs to another or closed request")
			}
			if _, ok := visited[r]; ok {
				return nil
			}
			if visited == nil {
				visited = make(map[gcNode]struct{})
			}
			visited[r] = struct{}{}
			pending = append(pending, r)
		case valueObject:
			if err := h.validateObject(v.object); err != nil {
				return err
			}
		case valueResource:
			if v.object == nil {
				return fmt.Errorf("nil resource value")
			}
			if _, err := h.resourceRecord(v); err != nil {
				return err
			}
		}
		return nil
	}

	for _, v := range values {
		if err := visit(v); err != nil {
			return nil, err
		}
	}
	for i := 0; i < len(pending); i++ {
		switch n := pending[i].(type) {
		case *phpArray:
			for _, b := range n.buckets {
				if b != nil {
					if err := visit(b.val); err != nil {
						return nil, err
					}
				}
			}
		case *reference:
			if err := visit(n.target); err != nil {
				return nil, err
			}
		}
	}
	return pending, nil
}

func (h *heap) validateObject(o *object) error {
	if o == nil || o.owner != h || o.freed || h.liveObject(o.handle) != o {
		return fmt.Errorf("object belongs to another or closed request")
	}
	return nil
}
