package tui

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

type vtCell struct {
	ch    rune
	style string
}

// PTYSession represents a live, stateful POSIX PTY process (e.g. `gcloud compute ssh <vm>`) running inside lazygcp.
type PTYSession struct {
	mu         sync.Mutex
	ResourceID string
	Title      string
	Command    string
	Ptmx       *os.File
	Cmd        *exec.Cmd
	Cols       int
	Rows       int
	Grid       [][]vtCell
	Scrollback []string
	CursorR    int
	CursorC    int
	SavedR     int
	SavedC     int
	ScrollTop  int
	ScrollBot  int
	CurStyle   string
	Exited     bool
	ExitMsg    string
	rawBuf     []byte
}

func NewPTYSession(resID, title, cmdStr string, cols, rows int, notifyCh chan<- struct{}) (*PTYSession, error) {
	if cols < 40 {
		cols = 80
	}
	if rows < 6 {
		rows = 16
	}
	ptmx, cmd, err := startPTYCommand(cmdStr, cols, rows)
	if err != nil {
		return nil, err
	}

	s := &PTYSession{
		ResourceID: resID,
		Title:      title,
		Command:    cmdStr,
		Ptmx:       ptmx,
		Cmd:        cmd,
		Cols:       cols,
		Rows:       rows,
		ScrollTop:  0,
		ScrollBot:  rows - 1,
	}
	s.initGrid(cols, rows)

	// Background PTY reader goroutine: streams raw bytes from the live SSH PTY into the VT100 screen grid
	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := ptmx.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.feedBytes(buf[:n])
				s.mu.Unlock()
				select {
				case notifyCh <- struct{}{}:
				default:
				}
			}
			if rerr != nil {
				break
			}
		}
		werr := cmd.Wait()
		s.mu.Lock()
		s.Exited = true
		if werr != nil {
			s.ExitMsg = werr.Error()
		} else {
			s.ExitMsg = "session closed (exit 0)"
		}
		s.mu.Unlock()
		select {
		case notifyCh <- struct{}{}:
		default:
		}
	}()

	return s, nil
}

func (s *PTYSession) initGrid(cols, rows int) {
	grid := make([][]vtCell, rows)
	for r := 0; r < rows; r++ {
		grid[r] = s.makeBlankRow(cols)
	}
	s.Grid = grid
}

func (s *PTYSession) makeBlankRow(cols int) []vtCell {
	row := make([]vtCell, cols)
	for c := 0; c < cols; c++ {
		row[c] = vtCell{ch: ' '}
	}
	return row
}

// Resize dynamically updates the PTY window size (TIOCSWINSZ) and resizes the VT100 grid.
func (s *PTYSession) Resize(cols, rows int) {
	if cols < 30 {
		cols = 30
	}
	if rows < 4 {
		rows = 4
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cols == s.Cols && rows == s.Rows {
		return
	}

	newGrid := make([][]vtCell, rows)
	for r := 0; r < rows; r++ {
		newGrid[r] = s.makeBlankRow(cols)
		if r < len(s.Grid) {
			copyCols := cols
			if len(s.Grid[r]) < copyCols {
				copyCols = len(s.Grid[r])
			}
			copy(newGrid[r][:copyCols], s.Grid[r][:copyCols])
		}
	}
	s.Grid = newGrid
	s.Cols = cols
	s.Rows = rows
	s.ScrollTop = 0
	s.ScrollBot = rows - 1
	if s.CursorR >= rows {
		s.CursorR = rows - 1
	}
	if s.CursorC >= cols {
		s.CursorC = cols - 1
	}
	if !s.Exited && s.Ptmx != nil {
		setPTYSize(s.Ptmx, cols, rows)
	}
}

// WriteRaw sends raw keystroke bytes directly to the live SSH PTY master fd.
func (s *PTYSession) WriteRaw(b []byte) error {
	s.mu.Lock()
	exited := s.Exited
	ptmx := s.Ptmx
	s.mu.Unlock()
	if exited || ptmx == nil {
		return os.ErrClosed
	}
	_, err := ptmx.Write(b)
	return err
}

// Close terminates the PTY session cleanly.
func (s *PTYSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Ptmx != nil {
		_ = s.Ptmx.Close()
		s.Ptmx = nil
	}
	if s.Cmd != nil && s.Cmd.Process != nil && !s.Exited {
		_ = s.Cmd.Process.Kill()
	}
	s.Exited = true
}

func (s *PTYSession) feedBytes(chunk []byte) {
	s.rawBuf = append(s.rawBuf, chunk...)
	data := s.rawBuf
	i := 0
	for i < len(data) {
		b := data[i]
		if b == 0x1b { // ESC
			if i+1 >= len(data) {
				break // wait for more bytes
			}
			next := data[i+1]
			switch next {
			case '[': // CSI sequence: ESC [ ... <final byte 0x40..0x7E>
				j := i + 2
				for j < len(data) && (data[j] < 0x40 || data[j] > 0x7E) {
					j++
				}
				if j >= len(data) {
					goto doneLoop // incomplete CSI, wait for next Read
				}
				params := string(data[i+2 : j])
				final := data[j]
				s.handleCSI(params, final)
				i = j + 1
				continue

			case ']': // OSC sequence: ESC ] ... (BEL 0x07 or ST ESC \)
				j := i + 2
				foundEnd := false
				var oscContent string
				for j < len(data) {
					if data[j] == 0x07 {
						oscContent = string(data[i+2 : j])
						j++
						foundEnd = true
						break
					}
					if data[j] == 0x1b && j+1 < len(data) && data[j+1] == '\\' {
						oscContent = string(data[i+2 : j])
						j += 2
						foundEnd = true
						break
					}
					j++
				}
				if !foundEnd {
					if len(data)-i > 512 {
						i += 2 // discard malformed long OSC
						continue
					}
					goto doneLoop
				}
				if strings.HasPrefix(oscContent, "0;") || strings.HasPrefix(oscContent, "2;") {
					s.Title = oscContent[2:]
				}
				i = j
				continue

			case '(', ')', '*', '+': // Charset designation (e.g. ESC ( B)
				if i+2 >= len(data) {
					goto doneLoop
				}
				i += 3
				continue

			case 'M': // Reverse Index (RI)
				if s.CursorR == s.ScrollTop {
					s.scrollDown(1)
				} else if s.CursorR > 0 {
					s.CursorR--
				}
				i += 2
				continue

			case 'D': // Index (IND)
				s.lineFeed()
				i += 2
				continue

			case 'E': // Next Line (NEL)
				s.CursorC = 0
				s.lineFeed()
				i += 2
				continue

			case '7': // Save Cursor (DECSC)
				s.SavedR = s.CursorR
				s.SavedC = s.CursorC
				i += 2
				continue

			case '8': // Restore Cursor (DECRC)
				s.CursorR = clampInt(s.SavedR, 0, s.Rows-1)
				s.CursorC = clampInt(s.SavedC, 0, s.Cols-1)
				i += 2
				continue

			case 'c': // Full Reset (RIS)
				s.initGrid(s.Cols, s.Rows)
				s.CursorR = 0
				s.CursorC = 0
				s.CurStyle = ""
				i += 2
				continue

			default: // ESC =, ESC >, etc.
				i += 2
				continue
			}
		}

		// Standard Control Characters
		switch b {
		case '\r':
			s.CursorC = 0
			i++
			continue
		case '\n', '\v', '\f':
			s.lineFeed()
			i++
			continue
		case '\b':
			if s.CursorC > 0 {
				s.CursorC--
			}
			i++
			continue
		case '\t':
			nextTab := ((s.CursorC / 8) + 1) * 8
			if nextTab >= s.Cols {
				nextTab = s.Cols - 1
			}
			s.CursorC = nextTab
			i++
			continue
		case 0x07, 0x00, 0x0e, 0x0f: // BEL, NUL, SO, SI
			i++
			continue
		}

		if b < 32 {
			i++
			continue
		}

		// Decode UTF-8 rune
		if !utf8.FullRune(data[i:]) {
			break // wait for remaining UTF-8 bytes
		}
		r, width := utf8.DecodeRune(data[i:])
		i += width
		if r == utf8.RuneError && width == 1 {
			continue
		}
		s.putRune(r)
	}

doneLoop:
	if i >= len(data) {
		s.rawBuf = s.rawBuf[:0]
	} else {
		s.rawBuf = append([]byte{}, data[i:]...)
	}
}

func (s *PTYSession) putRune(r rune) {
	if s.CursorC >= s.Cols {
		s.CursorC = 0
		s.lineFeed()
	}
	if s.CursorR >= 0 && s.CursorR < s.Rows && s.CursorC >= 0 && s.CursorC < s.Cols {
		s.Grid[s.CursorR][s.CursorC] = vtCell{ch: r, style: s.CurStyle}
		s.CursorC++
	}
}

func (s *PTYSession) lineFeed() {
	if s.CursorR == s.ScrollBot {
		s.scrollUp(1)
	} else if s.CursorR < s.Rows-1 {
		s.CursorR++
	}
}

func (s *PTYSession) scrollUp(n int) {
	for k := 0; k < n; k++ {
		if s.ScrollTop == 0 && len(s.Grid) > 0 {
			s.Scrollback = append(s.Scrollback, s.renderRowString(s.Grid[0], -1))
			if len(s.Scrollback) > 1000 {
				s.Scrollback = s.Scrollback[len(s.Scrollback)-1000:]
			}
		}
		for r := s.ScrollTop; r < s.ScrollBot; r++ {
			s.Grid[r] = s.Grid[r+1]
		}
		s.Grid[s.ScrollBot] = s.makeBlankRow(s.Cols)
	}
}

func (s *PTYSession) scrollDown(n int) {
	for k := 0; k < n; k++ {
		for r := s.ScrollBot; r > s.ScrollTop; r-- {
			s.Grid[r] = s.Grid[r-1]
		}
		s.Grid[s.ScrollTop] = s.makeBlankRow(s.Cols)
	}
}

func (s *PTYSession) handleCSI(params string, final byte) {
	// Ignore private mode prefixes like '?' or '>' for numeric parsing when needed
	cleanParams := strings.TrimLeft(params, "?>=")
	parts := strings.Split(cleanParams, ";")
	arg := func(idx, def int) int {
		if idx >= len(parts) || parts[idx] == "" {
			return def
		}
		v, err := strconv.Atoi(parts[idx])
		if err != nil || v <= 0 {
			return def
		}
		return v
	}

	switch final {
	case 'm': // SGR — Select Graphic Rendition
		if cleanParams == "" || cleanParams == "0" {
			s.CurStyle = ""
			return
		}
		for idx := 0; idx < len(parts); idx++ {
			p := parts[idx]
			if p == "" || p == "0" {
				s.CurStyle = ""
			} else if p == "39" || p == "49" || p == "22" || p == "24" || p == "27" {
				s.CurStyle = ""
			} else {
				s.CurStyle = "\x1b[" + cleanParams + "m"
				break
			}
		}

	case 'H', 'f': // Cursor Position (CUP)
		r := arg(0, 1) - 1
		c := arg(1, 1) - 1
		s.CursorR = clampInt(r, 0, s.Rows-1)
		s.CursorC = clampInt(c, 0, s.Cols-1)

	case 'A': // Cursor Up (CUU)
		s.CursorR = clampInt(s.CursorR-arg(0, 1), 0, s.Rows-1)

	case 'B': // Cursor Down (CUD)
		s.CursorR = clampInt(s.CursorR+arg(0, 1), 0, s.Rows-1)

	case 'C': // Cursor Forward (CUF)
		s.CursorC = clampInt(s.CursorC+arg(0, 1), 0, s.Cols-1)

	case 'D': // Cursor Back (CUB)
		s.CursorC = clampInt(s.CursorC-arg(0, 1), 0, s.Cols-1)

	case 'E': // Cursor Next Line (CNL)
		s.CursorR = clampInt(s.CursorR+arg(0, 1), 0, s.Rows-1)
		s.CursorC = 0

	case 'F': // Cursor Preceding Line (CPL)
		s.CursorR = clampInt(s.CursorR-arg(0, 1), 0, s.Rows-1)
		s.CursorC = 0

	case 'G', '`': // Cursor Horizontal Absolute (CHA)
		s.CursorC = clampInt(arg(0, 1)-1, 0, s.Cols-1)

	case 'd': // Vertical Position Absolute (VPA)
		s.CursorR = clampInt(arg(0, 1)-1, 0, s.Rows-1)

	case 'K': // Erase in Line (EL)
		mode := 0
		if cleanParams != "" {
			mode, _ = strconv.Atoi(parts[0])
		}
		row := s.Grid[s.CursorR]
		switch mode {
		case 0: // Cursor to end of line
			for c := clampInt(s.CursorC, 0, s.Cols-1); c < s.Cols; c++ {
				row[c] = vtCell{ch: ' '}
			}
		case 1: // Start of line to cursor
			for c := 0; c <= clampInt(s.CursorC, 0, s.Cols-1); c++ {
				row[c] = vtCell{ch: ' '}
			}
		case 2: // Entire line
			for c := 0; c < s.Cols; c++ {
				row[c] = vtCell{ch: ' '}
			}
		}

	case 'J': // Erase in Display (ED)
		mode := 0
		if cleanParams != "" {
			mode, _ = strconv.Atoi(parts[0])
		}
		switch mode {
		case 0: // Cursor to end of display
			for c := clampInt(s.CursorC, 0, s.Cols-1); c < s.Cols; c++ {
				s.Grid[s.CursorR][c] = vtCell{ch: ' '}
			}
			for r := s.CursorR + 1; r < s.Rows; r++ {
				s.Grid[r] = s.makeBlankRow(s.Cols)
			}
		case 1: // Start of display to cursor
			for r := 0; r < s.CursorR; r++ {
				s.Grid[r] = s.makeBlankRow(s.Cols)
			}
			for c := 0; c <= clampInt(s.CursorC, 0, s.Cols-1); c++ {
				s.Grid[s.CursorR][c] = vtCell{ch: ' '}
			}
		case 2, 3: // Entire display
			s.initGrid(s.Cols, s.Rows)
		}

	case 'P': // Delete Character (DCH)
		n := arg(0, 1)
		row := s.Grid[s.CursorR]
		cur := clampInt(s.CursorC, 0, s.Cols-1)
		for c := cur; c < s.Cols; c++ {
			if c+n < s.Cols {
				row[c] = row[c+n]
			} else {
				row[c] = vtCell{ch: ' '}
			}
		}

	case '@': // Insert Character (ICH)
		n := arg(0, 1)
		row := s.Grid[s.CursorR]
		cur := clampInt(s.CursorC, 0, s.Cols-1)
		for c := s.Cols - 1; c >= cur; c-- {
			if c-cur >= n {
				row[c] = row[c-n]
			} else {
				row[c] = vtCell{ch: ' '}
			}
		}

	case 'X': // Erase Character (ECH)
		n := arg(0, 1)
		row := s.Grid[s.CursorR]
		cur := clampInt(s.CursorC, 0, s.Cols-1)
		for c := cur; c < s.Cols && c < cur+n; c++ {
			row[c] = vtCell{ch: ' '}
		}

	case 'L': // Insert Line (IL)
		n := arg(0, 1)
		for k := 0; k < n; k++ {
			for r := s.ScrollBot; r > s.CursorR; r-- {
				s.Grid[r] = s.Grid[r-1]
			}
			s.Grid[s.CursorR] = s.makeBlankRow(s.Cols)
		}

	case 'M': // Delete Line (DL)
		n := arg(0, 1)
		for k := 0; k < n; k++ {
			for r := s.CursorR; r < s.ScrollBot; r++ {
				s.Grid[r] = s.Grid[r+1]
			}
			s.Grid[s.ScrollBot] = s.makeBlankRow(s.Cols)
		}

	case 'S': // Scroll Up (SU)
		s.scrollUp(arg(0, 1))

	case 'T': // Scroll Down (SD)
		s.scrollDown(arg(0, 1))

	case 'r': // Set Scrolling Region (DECSTBM)
		top := arg(0, 1) - 1
		bot := arg(1, s.Rows) - 1
		top = clampInt(top, 0, s.Rows-1)
		bot = clampInt(bot, top, s.Rows-1)
		s.ScrollTop = top
		s.ScrollBot = bot
		s.CursorR = 0
		s.CursorC = 0

	case 's': // Save Cursor
		s.SavedR = s.CursorR
		s.SavedC = s.CursorC

	case 'u': // Restore Cursor
		s.CursorR = clampInt(s.SavedR, 0, s.Rows-1)
		s.CursorC = clampInt(s.SavedC, 0, s.Cols-1)
	}
}

func (s *PTYSession) renderRowString(row []vtCell, cursorCol int) string {
	// Find last non-blank cell or cursorCol so we don't emit unnecessary trailing spaces
	lastCol := -1
	for c := len(row) - 1; c >= 0; c-- {
		if row[c].ch != ' ' || row[c].style != "" || c == cursorCol {
			lastCol = c
			break
		}
	}
	if lastCol < 0 {
		return ""
	}

	var sb strings.Builder
	curStyle := ""
	for c := 0; c <= lastCol; c++ {
		cell := row[c]
		ch := cell.ch
		if ch < 32 {
			ch = ' '
		}
		if c == cursorCol {
			// Render live block cursor at active cursor position
			sb.WriteString("\x1b[7m")
			sb.WriteRune(ch)
			sb.WriteString(Reset)
			curStyle = ""
			continue
		}
		if cell.style != curStyle {
			sb.WriteString(Reset)
			sb.WriteString(cell.style)
			curStyle = cell.style
		}
		sb.WriteRune(ch)
	}
	if curStyle != "" {
		sb.WriteString(Reset)
	}
	return sb.String()
}

// RenderLines returns the current visible VT100 screen lines (including live cursor when showCursor is true).
func (s *PTYSession) RenderLines(showCursor bool) (lines []string, title string, exited bool, exitMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, s.Rows)
	for r := 0; r < s.Rows; r++ {
		curCol := -1
		if showCursor && !s.Exited && r == s.CursorR {
			curCol = clampInt(s.CursorC, 0, s.Cols-1)
		}
		out[r] = " " + s.renderRowString(s.Grid[r], curCol)
	}
	return out, s.Title, s.Exited, s.ExitMsg
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
