//go:build windows

package tui

import (
	"fmt"
	"os"
	"os/exec"
)

func startPTYCommand(cmdStr string, cols, rows int) (*os.File, *exec.Cmd, error) {
	return nil, nil, fmt.Errorf("PTY not supported on windows")
}

func setPTYSize(ptmx *os.File, cols, rows int) {}
