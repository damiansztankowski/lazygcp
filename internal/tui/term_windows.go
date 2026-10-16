//go:build windows

package tui

import (
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

type termiosState struct{}

func notifySignals(sigCh chan<- os.Signal) {
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
}

func enableRawMode(fd int) (*termiosState, error) {
	return &termiosState{}, nil
}

func disableRawMode(fd int, state *termiosState) {}

func (a *App) updateTermSize() {
	if cols, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && cols >= 80 {
		a.Width = cols - 2
	}
	if lines, err := strconv.Atoi(os.Getenv("LINES")); err == nil && lines >= 24 {
		a.Height = lines
	}
}
