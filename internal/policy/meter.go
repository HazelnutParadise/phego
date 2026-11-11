package policy

import (
	"context"
	"fmt"
	"math"
	"sync"
)

// Resource is a metered resource.
type Resource uint8

const (
	Steps        Resource = iota + 1 // VM instructions and loop back edges
	Memory                           // logical bytes of PHP values
	Output                           // bytes written to the output stream
	Diagnostics                      // notices, warnings and errors recorded
	Frames                           // call depth
	OpenFiles                        // files open at once
	Connections                      // network and database connections open at once
	CompileBytes                     // source bytes compiled at run time (include, eval)
	Executions                       // executions running or waiting on one engine
	resourceCount
)

var resourceNames = [...]string{
	Steps: "steps", Memory: "memory", Output: "output", Diagnostics: "diagnostics", Frames: "frames",
	OpenFiles: "open-files", Connections: "connections", CompileBytes: "compile-bytes", Executions: "executions",
}

func (r Resource) String() string {
	if r > 0 && r < resourceCount {
		return resourceNames[r]
	}
	return fmt.Sprintf("Resource(%d)", uint8(r))
}

// gauge resources go up and down; the others only accumulate.
func (r Resource) gauge() bool { return r == Frames || r == OpenFiles || r == Connections }

// Limits are per-execution budgets. Zero means no budget for that resource.
type Limits map[Resource]int64

// Meter counts one execution's resource use against its limits. It is
// owned by one execution and safe for use by its host callbacks.
type Meter struct {
	mu     sync.Mutex
	limits [resourceCount]int64
	used   [resourceCount]int64
	peak   [resourceCount]int64
}

// NewMeter validates limits and returns a meter with nothing used.
func NewMeter(limits Limits) (*Meter, error) {
	m := &Meter{}
	for r, n := range limits {
		if r == 0 || r >= resourceCount {
			return nil, &SpecError{Field: "Limits", Reason: fmt.Sprintf("unknown resource %d", r)}
		}
		if n < 0 {
			return nil, &SpecError{Field: "Limits", Reason: fmt.Sprintf("%s limit %d is negative", r, n)}
		}
		m.limits[r] = n
	}
	return m, nil
}

// Charge adds n units of r. It fails without charging when the budget
// would be exceeded or the count would overflow, so a refused charge leaves
// the meter unchanged.
func (m *Meter) Charge(r Resource, n int64) error {
	if r == 0 || r >= resourceCount || n < 0 {
		return &SpecError{Field: "Charge", Reason: fmt.Sprintf("invalid charge of %d %s", n, r)}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	used := m.used[r]
	if n > math.MaxInt64-used {
		return &ResourceError{Resource: r, Limit: m.limits[r], Used: used, Requested: n}
	}
	if limit := m.limits[r]; limit > 0 && used+n > limit {
		return &ResourceError{Resource: r, Limit: limit, Used: used, Requested: n}
	}
	m.used[r] = used + n
	if m.used[r] > m.peak[r] {
		m.peak[r] = m.used[r]
	}
	return nil
}

// Release returns n units of a gauge resource such as an open file. It is
// an error to release more than is held or to release an accumulating
// resource.
func (m *Meter) Release(r Resource, n int64) error {
	if r == 0 || r >= resourceCount || !r.gauge() || n < 0 {
		return &SpecError{Field: "Release", Reason: fmt.Sprintf("%s cannot be released", r)}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if n > m.used[r] {
		return &SpecError{Field: "Release", Reason: fmt.Sprintf("releasing %d %s while %d are held", n, r, m.used[r])}
	}
	m.used[r] -= n
	return nil
}

// Checkpoint charges one step and reports cancellation. Loops, calls and
// long operations call it so a cancelled or over-budget execution stops at
// a controlled point.
func (m *Meter) Checkpoint(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return m.Charge(Steps, 1)
}

// Usage is a snapshot of a meter.
type Usage struct {
	Used, Peak Limits
}

// Usage returns the current and peak use of every resource that was used.
func (m *Meter) Usage() Usage {
	m.mu.Lock()
	defer m.mu.Unlock()
	u := Usage{Used: Limits{}, Peak: Limits{}}
	for r := Resource(1); r < resourceCount; r++ {
		if m.peak[r] > 0 {
			u.Used[r], u.Peak[r] = m.used[r], m.peak[r]
		}
	}
	return u
}
