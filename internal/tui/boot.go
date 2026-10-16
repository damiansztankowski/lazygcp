package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/damiansztankowski/lazygcp/internal/gcp"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
var radarFrames = []string{
	"◜ ─── ○ ─── ◝",
	"◞ ─── ◎ ─── ◜",
	"◟ ─── ● ─── ◞",
	"◜ ─── ◎ ─── ◟",
}

// BootWithAnimation renders a full-screen animated cockpit initialization sequence while querying live GCP APIs in parallel.
func BootWithAnimation(overrideProject string, useDemo bool) (*gcp.ProjectState, error) {
	var (
		state   *gcp.ProjectState
		loadErr error
		doneCh  = make(chan struct{})
	)

	startTime := time.Now()

	go func() {
		if useDemo {
			proj, acct := gcp.TryDetectGcloudProject()
			time.Sleep(550 * time.Millisecond)
			state = gcp.BuildMockState(proj, acct)
		} else {
			state, loadErr = gcp.FetchLiveProjectState(overrideProject)
		}
		// Ensure the boot animation completes a smooth visual sweep (at least 750ms)
		elapsed := time.Since(startTime)
		if elapsed < 750*time.Millisecond {
			time.Sleep(750*time.Millisecond - elapsed)
		}
		close(doneCh)
	}()

	// Enter alternate screen buffer and hide cursor during boot animation
	fmt.Print("\x1b[?1049h\x1b[?25l\x1b[2J")

	ticker := time.NewTicker(55 * time.Millisecond)
	defer ticker.Stop()

	tick := 0
	for {
		select {
		case <-doneCh:
			renderBootFrame(tick, true, startTime, overrideProject)
			time.Sleep(120 * time.Millisecond)
			return state, loadErr
		case <-ticker.C:
			renderBootFrame(tick, false, startTime, overrideProject)
			tick++
		}
	}
}

func renderBootFrame(tick int, finished bool, startTime time.Time, overrideProject string) {
	tmp := &App{Width: 110, Height: 30}
	tmp.updateTermSize()
	w, h := tmp.Width, tmp.Height
	if w < 80 {
		w = 100
	}
	if h < 24 {
		h = 30
	}

	elapsedMs := time.Since(startTime).Milliseconds()
	pct := int(elapsedMs * 100 / 900)
	if finished {
		pct = 100
	} else if pct > 96 {
		pct = 96
	}

	spin := spinnerFrames[tick%len(spinnerFrames)]
	radar := radarFrames[(tick/2)%len(radarFrames)]

	projLabel := overrideProject
	if projLabel == "" {
		projLabel = "active gcloud ADC context"
	}

	boxW := 86
	if w-6 < boxW {
		boxW = w - 6
	}
	innerW := boxW - 4

	steps := []struct {
		threshold int
		label     string
		sub       string
	}{
		{10, "OAuth2 ADC Handshake & Token Acquisition", "gcloud auth + TLS 1.3 connection pool"},
		{25, "Cloud Resource Manager & Project Discovery", "projects.list + Cloud Billing API v1"},
		{42, "Cloud Run Services & Cloud Functions", "run.googleapis.com + cloudfunctions v2"},
		{58, "Live Project IAM Policy & Security Graph", "getIamPolicy + iam.v1 serviceAccounts"},
		{72, "GKE Clusters & Compute Engine VMs", "container.v1 + compute.v1 power states"},
		{85, "Cloud SQL, GCS Buckets & Secret Manager", "sqladmin.v1 + storage.v1 + secretmanager"},
		{95, "VPC Firewalls, FinOps Spend & Audit Logs", "firewalls + billing.v1 + cloudaudit.v2"},
	}

	var boxLines []string
	boxLines = append(boxLines, "")
	logoColor := FgCyan + Bold
	if tick%6 >= 3 {
		logoColor = FgBlue + Bold
	}
	boxLines = append(boxLines, centerAnsi(logoColor+"   __                                        "+Reset, innerW))
	boxLines = append(boxLines, centerAnsi(logoColor+"  / /  ____ _____  __  ______ __________     "+Reset, innerW))
	boxLines = append(boxLines, centerAnsi(logoColor+" / /  / __ `/_  / / / / / __ `/ ___/ __ \\    "+Reset, innerW))
	boxLines = append(boxLines, centerAnsi(FgPurple+Bold+"/ /__/ /_/ / / /_/ /_/ / /_/ / /__/ /_/ /    "+Reset, innerW))
	boxLines = append(boxLines, centerAnsi(FgPurple+Bold+"\\____/\\__,_/ /___/\\__, /\\__, /\\___/ .___/     "+Reset, innerW))
	boxLines = append(boxLines, centerAnsi(FgMagenta+Bold+"                 /____//____/    /_/          "+Reset, innerW))
	boxLines = append(boxLines, "")
	boxLines = append(boxLines, centerAnsi(FgWhite+Bold+"⚡ LIVE GOOGLE CLOUD TERMINAL COCKPIT, SSH PTY & FINOPS ENGINE ⚡"+Reset, innerW))
	boxLines = append(boxLines, centerAnsi(FgGray+"Target Context: "+FgYellow+Bold+projLabel+Reset+FgGray+"  │  "+FgCyan+radar+Reset, innerW))
	boxLines = append(boxLines, FgGray+" "+strings.Repeat("─", innerW)+Reset)

	// Progress Bar
	barW := innerW - 18
	if barW < 20 {
		barW = 20
	}
	filled := pct * barW / 100
	if filled > barW {
		filled = barW
	}
	var barBuilder strings.Builder
	for i := 0; i < barW; i++ {
		if i < filled-2 {
			barBuilder.WriteString(FgGreen + Bold + "█")
		} else if i < filled {
			barBuilder.WriteString(FgCyan + Bold + "▓")
		} else if i == filled && !finished {
			barBuilder.WriteString(FgYellow + Bold + "▒")
		} else {
			barBuilder.WriteString(FgGray + "░")
		}
	}
	barBuilder.WriteString(Reset)
	boxLines = append(boxLines, fmt.Sprintf("  [%s] %s%3d%%%s (%dms)",
		barBuilder.String(), FgCyan+Bold, pct, Reset, elapsedMs))
	boxLines = append(boxLines, FgGray+" "+strings.Repeat("─", innerW)+Reset)

	for _, st := range steps {
		var icon, titleStyle, subStyle string
		if finished || pct >= st.threshold {
			icon = FgGreen + Bold + "✓" + Reset
			titleStyle = FgWhite + Bold
			subStyle = FgGreen
		} else if pct+18 >= st.threshold {
			icon = FgCyan + Bold + spin + Reset
			titleStyle = FgYellow + Bold
			subStyle = FgCyan
		} else {
			icon = FgGray + "·" + Reset
			titleStyle = FgGray
			subStyle = FgGray
		}
		line := fmt.Sprintf("   [%s] %-42s %s%s%s",
			icon,
			titleStyle+padPlain(st.label, 42)+Reset,
			subStyle, st.sub, Reset)
		boxLines = append(boxLines, line)
	}

	boxLines = append(boxLines, "")
	if finished {
		boxLines = append(boxLines, centerAnsi(BgSuccess+FgGreen+Bold+" ● TELEMETRY LOCKED — LAUNCHING COCKPIT "+Reset, innerW))
	} else {
		boxLines = append(boxLines, centerAnsi(FgCyan+spin+" Synchronizing live HTTP/2 streams across 9 GCP modules..."+Reset, innerW))
	}

	boxH := len(boxLines) + 3
	renderedBox := drawBox("⚡ lazygcp Initialization Sequence", FgCyan, boxLines, boxW, boxH)

	topPad := (h - boxH) / 2
	if topPad < 0 {
		topPad = 0
	}
	leftPad := (w - boxW) / 2
	if leftPad < 0 {
		leftPad = 0
	}
	padStr := strings.Repeat(" ", leftPad)

	var screen []string
	for i := 0; i < topPad; i++ {
		screen = append(screen, strings.Repeat(" ", w))
	}
	for _, bl := range renderedBox {
		screen = append(screen, padOrTrunc(padStr+bl, w))
	}
	for len(screen) < h {
		screen = append(screen, strings.Repeat(" ", w))
	}
	fmt.Print("\x1b[H" + strings.Join(screen[:h], "\r\n"))
}

func centerAnsi(s string, width int) string {
	vLen := visibleLen(s)
	if vLen >= width {
		return padOrTrunc(s, width)
	}
	left := (width - vLen) / 2
	right := width - vLen - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}
