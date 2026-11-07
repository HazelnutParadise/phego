//go:build unix && !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package policy

// fifoProbe has no readiness source on these uncertified systems: the open
// relies on its one-byte read alone, which keeps a byte it reads for the
// first Read.
type fifoProbe struct{}

func newFIFOProbe(int) (*fifoProbe, error) { return &fifoProbe{}, nil }

func (*fifoProbe) state() (data, hangup bool, err error) { return false, false, nil }

func (*fifoProbe) close() {}
