package runtime

import "strings"

// REQ-018/020; T-035/036/039/040. Descriptors are immutable DAGs: builders
// reference already completed descriptors, never the descriptor being built.
func classReachable(c, want *class, names []string) bool {
	for c != nil {
		if c == want {
			return true
		}
		for _, name := range names {
			if phpNameEqual(c.name, strings.TrimPrefix(name, "\\")) {
				return true
			}
		}
		if c.schema == nil {
			return false
		}
		if len(c.schema.interfaces) == 0 {
			c = c.schema.parent
		} else if c.schema.parent == nil && len(c.schema.interfaces) == 1 {
			c = c.schema.interfaces[0]
		} else {
			return classGraphReachable(c, want, names)
		}
	}
	return false
}

func classGraphReachable(c, want *class, names []string) bool {
	var inline [32]classGraphFrame
	frames := inline[:1]
	frames[0].node = c
	var failures classGraphFailures
	for len(frames) != 0 {
		frame := &frames[len(frames)-1]
		schema := frame.node.schema
		var next *class
		if frame.nextEdge < len(schema.interfaces) {
			next = schema.interfaces[frame.nextEdge]
			frame.nextEdge++
		} else if frame.nextEdge == len(schema.interfaces) {
			next = schema.parent
			frame.nextEdge++
		} else {
			// Cache a miss only after every interface and the parent failed.
			// A successful first path never populates this cache.
			failures.add(frame.node)
			frames = frames[:len(frames)-1]
			continue
		}
		if next == nil || failures.contains(next) {
			continue
		}
		if next == want {
			return true
		}
		for _, name := range names {
			if phpNameEqual(next.name, strings.TrimPrefix(name, "\\")) {
				return true
			}
		}
		if next.schema == nil {
			failures.add(next)
			continue
		}
		frames = append(frames, classGraphFrame{node: next})
	}
	return false
}

type classGraphFrame struct {
	node     *class
	nextEdge int
}

// Keep small failures on the stack; larger DAGs use Go's pointer-keyed map.
// This state belongs to one lookup, never to a shared class or request cache.
type classGraphFailures struct {
	inline [8]*class
	count  int
	large  map[*class]struct{}
}

func (s *classGraphFailures) contains(node *class) bool {
	if s.large != nil {
		_, found := s.large[node]
		return found
	}
	for _, failed := range s.inline[:s.count] {
		if failed == node {
			return true
		}
	}
	return false
}

func (s *classGraphFailures) add(node *class) {
	if s.large != nil {
		s.large[node] = struct{}{}
		return
	}
	if s.count < len(s.inline) {
		s.inline[s.count] = node
		s.count++
		return
	}
	s.large = make(map[*class]struct{}, 16)
	for _, failed := range s.inline {
		s.large[failed] = struct{}{}
	}
	s.large[node] = struct{}{}
}
