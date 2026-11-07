package policy

import "syscall"

// hangupProbe asks whether a writer opened and closed a FIFO since this
// reader opened it, which Linux reports as EPOLLHUP: fifo_open records the
// writer count, and pipe_poll reports a hangup once it has changed and no
// writer is left. A private level-triggered epoll instance answers without
// consuming data or waiting, and is separate from the runtime poller.
type hangupProbe struct{ ep int }

func newHangupProbe(fd int) (*hangupProbe, error) {
	ep, err := syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	if err := syscall.EpollCtl(ep, syscall.EPOLL_CTL_ADD, fd, &syscall.EpollEvent{Events: syscall.EPOLLIN, Fd: int32(fd)}); err != nil {
		syscall.Close(ep)
		return nil, err
	}
	return &hangupProbe{ep}, nil
}

func (h *hangupProbe) hungUp() (bool, error) {
	events := make([]syscall.EpollEvent, 1)
	for {
		n, err := syscall.EpollWait(h.ep, events, 0)
		if err == syscall.EINTR {
			continue
		}
		return n > 0 && events[0].Events&syscall.EPOLLHUP != 0, err
	}
}

func (h *hangupProbe) close() { syscall.Close(h.ep) }
