package runtime

import "slices"

// REQ-018/020/021; T-035/036/039-042. Immutable layout belongs to the
// class; slots belong to one object. Declared storage never grows. Dynamic
// storage grows in fixed blocks so escaped locations never move.
type propertyLayout struct {
	names []string
	index map[string]int
}

func newPropertyLayout(names []string) *propertyLayout {
	p := &propertyLayout{names: slices.Clone(names), index: make(map[string]int, len(names))}
	for i, name := range names {
		p.index[name] = i
	}
	return p
}

type propertyTable struct {
	layout   *propertyLayout
	declared []value
	index    map[string]*value
	head     *propertyBlock
	tail     *propertyBlock
}

// Each dynamic block and its fixed-capacity entries will share one safe Go
// allocation. Existing locations remain stable when a later block is linked.
type propertyEntry struct {
	name  string
	value value
}

type propertyBlock struct {
	next    *propertyBlock
	used    int
	entries []propertyEntry
}

func newPropertyBlock(capacity int) *propertyBlock {
	switch capacity {
	case 1:
		storage := &struct {
			block   propertyBlock
			entries [1]propertyEntry
		}{}
		storage.block.entries = storage.entries[:]
		return &storage.block
	case 2:
		storage := &struct {
			block   propertyBlock
			entries [2]propertyEntry
		}{}
		storage.block.entries = storage.entries[:]
		return &storage.block
	case 4:
		storage := &struct {
			block   propertyBlock
			entries [4]propertyEntry
		}{}
		storage.block.entries = storage.entries[:]
		return &storage.block
	case 8:
		storage := &struct {
			block   propertyBlock
			entries [8]propertyEntry
		}{}
		storage.block.entries = storage.entries[:]
		return &storage.block
	case 16:
		storage := &struct {
			block   propertyBlock
			entries [16]propertyEntry
		}{}
		storage.block.entries = storage.entries[:]
		return &storage.block
	default:
		panic("invalid property block capacity")
	}
}

func newPropertyTable(layout *propertyLayout) *propertyTable {
	p := &propertyTable{}
	p.initializeLayout(layout)
	return p
}

// Initialize fresh storage before any property locations escape.
func (p *propertyTable) initializeLayout(layout *propertyLayout) {
	p.layout = layout
	if layout != nil {
		p.declared = make([]value, len(layout.names))
	}
}

func (p *propertyTable) lookup(name string) (*value, bool) {
	if p.layout != nil {
		if i, ok := p.layout.index[name]; ok {
			return &p.declared[i], true
		}
	}
	v, ok := p.index[name]
	if !ok {
		return nil, false
	}
	return v, true
}

func (p *propertyTable) slot(name string) *value {
	if v, ok := p.lookup(name); ok {
		return v
	}
	if p.index == nil {
		p.index = make(map[string]*value)
	}
	if p.tail == nil || p.tail.used == len(p.tail.entries) {
		capacity := 1
		if p.tail != nil {
			capacity = len(p.tail.entries) * 2
			if capacity > 16 {
				capacity = 16
			}
		}
		block := newPropertyBlock(capacity)
		if p.tail == nil {
			p.head = block
		} else {
			p.tail.next = block
		}
		p.tail = block
	}
	entry := &p.tail.entries[p.tail.used]
	entry.name = name
	entry.value = nullValue()
	p.tail.used++
	p.index[name] = &entry.value
	return &entry.value
}

// remove detaches ownership before a destructor can reenter the table.
func (p *propertyTable) remove(name string) (value, bool) {
	v, ok := p.lookup(name)
	if !ok || v.kind == valueUndefined {
		return value{}, false
	}
	old := *v
	*v = value{}
	delete(p.index, name)
	return old, true
}

func (p *propertyTable) each(visit func(string, *value)) {
	if p.layout != nil {
		for i, name := range p.layout.names {
			if v := &p.declared[i]; v.kind != valueUndefined {
				visit(name, v)
			}
		}
	}
	// Re-read used and next after each callback so reentrant appends are visited.
	for block := p.head; block != nil; block = block.next {
		for i := 0; i < block.used; i++ {
			entry := &block.entries[i]
			if current, ok := p.index[entry.name]; !ok || current != &entry.value {
				continue
			}
			if entry.value.kind != valueUndefined {
				visit(entry.name, &entry.value)
			}
		}
	}
}

// Detach the location before invoking a destructor. Reentrant writes into
// existing declared slots are subsequently released as well.
func (h *heap) freeProperties(o *object, dead map[gcNode]bool) error {
	return h.freePropertyTable(o.props, dead, true)
}

func (h *heap) freePropertyTable(table *propertyTable, dead map[gcNode]bool, detach bool) error {
	var first error
	for {
		found := false
		table.each(func(_ string, slot *value) {
			found = true
			if detach {
				h.detachPropertySource(slot)
			}
			old := *slot
			*slot = value{}
			first = h.mergeObjectErrors(first, h.releaseOutside(old, dead))
		})
		if !found {
			return first
		}
	}
}
