package policy

import "fmt"

// SpecError reports an invalid capability or limit specification.
type SpecError struct {
	Field  string
	Reason string
}

func (e *SpecError) Error() string { return "policy: invalid " + e.Field + ": " + e.Reason }

// PolicyError reports an operation the policy refused. Rule identifies why:
// "not-granted" for a missing capability, "outside-root" for a path that
// leaves every granted tree, "private-address" for a non-public network
// address, or the name of a matched heuristic rule in enforce mode.
type PolicyError struct {
	Kind   Kind
	Op     string
	Target string
	Rule   string
}

func (e *PolicyError) Error() string {
	return fmt.Sprintf("policy: %s %s %q refused (%s)", e.Kind, e.Op, e.Target, e.Rule)
}

// ResourceError reports a resource budget that would be exceeded. The
// budget is logical: it counts what the engine charges, not process RSS.
type ResourceError struct {
	Resource  Resource
	Limit     int64
	Used      int64
	Requested int64
}

func (e *ResourceError) Error() string {
	return fmt.Sprintf("policy: %s limit %d exceeded (used %d, requested %d)", e.Resource, e.Limit, e.Used, e.Requested)
}
