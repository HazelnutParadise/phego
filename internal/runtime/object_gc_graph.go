package runtime

import "slices"

// REQ-021; T-041/042. WeakMap entries own their values once, but make a
// value reachable only when both map and key are reachable. The work queue
// visits each enabled entry at most twice, avoiding repeated fixed-point scans.
type gcWeakEntry struct{ mapNode, key, value gcNode }

func discoveryEdges(n gcNode, visit func(gcNode)) {
	edges(n, visit)
	weakEntries(n, func(m *weakMap, key *object, v value) {
		visit(m.identity)
		visit(key)
		if node := compositeNode(v); node != nil {
			visit(node)
		}
	})
}

func compositeNode(v value) gcNode {
	switch v.kind {
	case valueArray:
		return v.array
	case valueObject:
		return v.object
	case valueReference:
		return v.ref
	}
	return nil
}

func weakEntries(n gcNode, visit func(*weakMap, *object, value)) {
	o, ok := n.(*object)
	if !ok || o.owner == nil {
		return
	}
	if m := o.owner.weakMapObjects[o]; m != nil && !m.closed {
		for _, key := range m.keys {
			if v, ok := m.entries[key]; ok {
				visit(m, key, v)
			}
		}
	}
	for _, m := range o.weakMaps {
		if m.identity != nil && !m.closed {
			if v, ok := m.entries[o]; ok {
				visit(m, o, v)
			}
		}
	}
}

func garbageChecked(roots []gcNode, check func() error) ([]gcNode, error) {
	order := []gcNode{}
	seen := map[gcNode]bool{}
	internal := map[gcNode]int{}
	weak := []gcWeakEntry{}
	watched := map[gcNode][]int{}
	entries := map[*weakMap]map[*object]bool{}
	stack := slices.Clone(roots)
	slices.Reverse(stack)
	steps := 0
	poll := func() error {
		steps++
		if steps%256 == 0 && check != nil {
			return check()
		}
		return nil
	}
	for len(stack) > 0 {
		if err := poll(); err != nil {
			return nil, err
		}
		i := len(stack) - 1
		n := stack[i]
		stack = stack[:i]
		if n == nil || seen[n] {
			continue
		}
		seen[n] = true
		order = append(order, n)
		start := len(stack)
		edges(n, func(child gcNode) { internal[child]++; stack = append(stack, child) })
		weakEntries(n, func(m *weakMap, key *object, v value) {
			if entries[m] == nil {
				entries[m] = make(map[*object]bool)
			}
			if entries[m][key] {
				return
			}
			entries[m][key] = true
			valueNode := compositeNode(v)
			if valueNode != nil {
				internal[valueNode]++
			}
			entry := gcWeakEntry{m.identity, key, valueNode}
			index := len(weak)
			weak = append(weak, entry)
			watched[entry.mapNode] = append(watched[entry.mapNode], index)
			watched[key] = append(watched[key], index)
			stack = append(stack, entry.mapNode, key)
			if valueNode != nil {
				stack = append(stack, valueNode)
			}
		})
		slices.Reverse(stack[start:])
	}
	alive := map[gcNode]bool{}
	stack = stack[:0]
	for _, n := range order {
		if refcountOf(n) > internal[n] {
			stack = append(stack, n)
		}
	}
	for len(stack) > 0 {
		if err := poll(); err != nil {
			return nil, err
		}
		i := len(stack) - 1
		n := stack[i]
		stack = stack[:i]
		if n == nil || alive[n] {
			continue
		}
		alive[n] = true
		edges(n, func(child gcNode) { stack = append(stack, child) })
		for _, index := range watched[n] {
			entry := weak[index]
			if entry.value != nil && alive[entry.mapNode] && alive[entry.key] {
				stack = append(stack, entry.value)
			}
		}
	}
	white := make([]gcNode, 0, len(order))
	for _, n := range order {
		if !alive[n] {
			white = append(white, n)
		}
	}
	if check != nil {
		if err := check(); err != nil {
			return nil, err
		}
	}
	return white, nil
}
