package policy

import "syscall"

// fifoProbe reports, without consuming data or waiting, whether a FIFO
// opened for reading has data waiting and whether a writer opened and closed
// it since this reader opened it. Linux reports the second as EPOLLHUP:
// fifo_open records the writer count, and pipe_poll reports a hangup once it
// has changed and no writer is left. A private level-triggered epoll
// instance answers, separate from the runtime poller.
type fifoProbe struct{ ep int }

func newFIFOProbe(fd int) (*fifoProbe, error) {
	ep, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	if err := syscall.EpollCtl(ep, syscall.EPOLL_CTL_ADD, fd, &syscall.EpollEvent{Events: syscall.EPOLLIN, Fd: int32(fd)}); err != nil {
		syscall.Close(ep)
		return nil, err
	}
	return &fifoProbe{ep}, nil
}

func (p *fifoProbe) state() (data, hangup bool, err error) {
	events := make([]syscall.EpollEvent, 1)
	for {
		n, err := syscall.EpollWait(p.ep, events, 0)
		if err == syscall.EINTR {
			continue
		}
		if n == 0 || err != nil {
			return false, false, err
		}
		return events[0].Events&syscall.EPOLLIN != 0, events[0].Events&syscall.EPOLLHUP != 0, nil
	}
}

func (p *fifoProbe) close() { syscall.Close(p.ep) }
