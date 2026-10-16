//go:build darwin || freebsd || netbsd || openbsd

package tui

import (
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"unsafe"
)

type termiosState struct {
	termios syscall.Termios
}

func notifySignals(sigCh chan<- os.Signal) {
	signal.Notify(sigCh, syscall.SIGWINCH, syscall.SIGINT, syscall.SIGTERM)
}

func enableRawMode(fd int) (*termiosState, error) {
	var orig syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&orig)), 0, 0, 0)
	if errno != 0 {
		return nil, errno
	}
	raw := orig
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	raw.Iflag &^= syscall.IXON | syscall.ICRNL | syscall.BRKINT | syscall.INPCK | syscall.ISTRIP
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0

	_, _, errno = syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCSETA), uintptr(unsafe.Pointer(&raw)), 0, 0, 0)
	if errno != 0 {
		return nil, errno
	}
	return &termiosState{termios: orig}, nil
}

func disableRawMode(fd int, state *termiosState) {
	if state == nil {
		return
	}
	_, _, _ = syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCSETA), uintptr(unsafe.Pointer(&state.termios)), 0, 0, 0)
}

type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

func (a *App) updateTermSize() {
	fds := []uintptr{os.Stdout.Fd(), os.Stdin.Fd(), os.Stderr.Fd()}
	for _, fd := range fds {
		var ws winsize
		_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
		if errno == 0 && ws.Col > 0 && ws.Row > 0 {
			w := int(ws.Col) - 1
			if w < 76 {
				w = 76
			}
			a.Width = w
			if ws.Row >= 18 {
				a.Height = int(ws.Row)
			}
			return
		}
	}
	if cols, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && cols >= 76 {
		a.Width = cols - 1
	}
	if lines, err := strconv.Atoi(os.Getenv("LINES")); err == nil && lines >= 18 {
		a.Height = lines
	}
}
