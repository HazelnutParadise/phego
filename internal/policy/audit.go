package policy

import (
	"net/url"
	"strings"
	"sync"
)

// Event records one checked operation. Targets are redacted before they
// are stored: URL passwords and query values never reach the log.
type Event struct {
	Mode    Mode
	Kind    Kind
	Op      string
	Target  string
	Allowed bool
	Rule    string
}

// Log is a bounded per-execution event log. When it is full, further events
// are counted but not stored, so a script cannot grow it without bound.
type Log struct {
	mu      sync.Mutex
	max     int
	events  []Event
	dropped int
}

// NewLog returns a log that stores at most max events.
func NewLog(max int) *Log {
	if max < 0 {
		max = 0
	}
	return &Log{max: max}
}

// Record stores e with a redacted target.
func (l *Log) Record(e Event) {
	e.Target = Redact(e.Target)
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.events) >= l.max {
		l.dropped++
		return
	}
	l.events = append(l.events, e)
}

// Events returns a copy of the stored events and how many were dropped.
func (l *Log) Events() ([]Event, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]Event(nil), l.events...), l.dropped
}

// Redact removes credentials and query values from a URL-like target and
// leaves other targets unchanged.
func Redact(target string) string {
	if !strings.Contains(target, "://") {
		return target
	}
	u, err := url.Parse(target)
	if err != nil {
		return "[unparsable URL]"
	}
	if u.User != nil {
		u.User = url.User("[redacted]")
	}
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			q[k] = []string{"[redacted]"}
		}
		u.RawQuery = q.Encode()
	}
	u.Fragment = ""
	return u.String()
}
