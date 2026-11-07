//go:build unix && !linux

package policy

// hangupProbe cannot see a writer that opened and closed a FIFO without
// writing: on macOS and the BSDs the reader's read returns 0 and kqueue
// reports nothing, as before any writer arrived (Go issue 24164). The open
// keeps waiting for a writer or data, as W-20.md records.
type hangupProbe struct{}

func newHangupProbe(int) (*hangupProbe, error) { return &hangupProbe{}, nil }

func (*hangupProbe) hungUp() (bool, error) { return false, nil }

func (*hangupProbe) close() {}
