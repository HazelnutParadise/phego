//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package policy

import "syscall"

// fifoProbe reports, without consuming data or waiting, whether a FIFO
// opened for reading has data waiting, through a private kqueue whose read
// filter carries the byte count. It reports a hangup only where the kernel
// sets EV_EOF. macOS gives a reader no trace of a writer that opened and
// closed a FIFO without writing (Go issue 24164), so the open keeps waiting
// for a writer or data there, as W-20.md records.
type fifoProbe struct {
	kq int
	fd int
}

func newFIFOProbe(fd int) (*fifoProbe, error) {
	kq, err := syscall.Kqueue()
	if err != nil {
		return nil, err
	}
	syscall.CloseOnExec(kq)
	return &fifoProbe{kq, fd}, nil
}

func (p *fifoProbe) state() (data, hangup bool, err error) {
	change := make([]syscall.Kevent_t, 1)
	syscall.SetKevent(&change[0], p.fd, syscall.EVFILT_READ, syscall.EV_ADD|syscall.EV_ONESHOT)
	events := make([]syscall.Kevent_t, 1)
	for {
		n, err := syscall.Kevent(p.kq, change, events, &syscall.Timespec{})
		if err == syscall.EINTR {
			continue
		}
		if n == 0 || err != nil {
			return false, false, err
		}
		return events[0].Data > 0, events[0].Flags&syscall.EV_EOF != 0, nil
	}
}

func (p *fifoProbe) close() { syscall.Close(p.kq) }
