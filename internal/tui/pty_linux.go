//go:build linux

package tui

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

func startPTYCommand(cmdStr string, cols, rows int) (*os.File, *exec.Cmd, error) {
	ptmx, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}

	var unlock int32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&unlock))); errno != 0 {
		_ = ptmx.Close()
		return nil, nil, fmt.Errorf("TIOCSPTLCK failed: %v", errno)
	}

	var ptn uint32
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&ptn))); errno != 0 {
		_ = ptmx.Close()
		return nil, nil, fmt.Errorf("TIOCGPTN failed: %v", errno)
	}
	slaveName := fmt.Sprintf("/dev/pts/%d", ptn)

	pts, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = ptmx.Close()
		return nil, nil, err
	}

	setPTYSize(ptmx, cols, rows)

	cmd := exec.Command("sh", "-c", cmdStr)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	cmd.Stdin = pts
	cmd.Stdout = pts
	cmd.Stderr = pts
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
	}

	if err := cmd.Start(); err != nil {
		_ = pts.Close()
		_ = ptmx.Close()
		return nil, nil, err
	}
	_ = pts.Close()
	return ptmx, cmd, nil
}

func setPTYSize(ptmx *os.File, cols, rows int) {
	if ptmx == nil || cols <= 0 || rows <= 0 {
		return
	}
	ws := struct {
		Row    uint16
		Col    uint16
		Xpixel uint16
		Ypixel uint16
	}{
		Row: uint16(rows),
		Col: uint16(cols),
	}
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, ptmx.Fd(), syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&ws)))
}
