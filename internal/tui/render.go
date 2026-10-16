package tui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/damiansztankowski/lazygcp/internal/gcp"
)

const (
	Reset     = "\x1b[0m"
	Bold      = "\x1b[1m"
	Dim       = "\x1b[2m"
	FgCyan    = "\x1b[38;5;51m"
	FgBlue    = "\x1b[38;5;39m"
	FgGreen   = "\x1b[38;5;48m"
	FgYellow  = "\x1b[38;5;220m"
	FgOrange  = "\x1b[38;5;208m"
	FgRed     = "\x1b[38;5;196m"
	FgMagenta = "\x1b[38;5;201m"
	FgPurple  = "\x1b[38;5;141m"
	FgWhite   = "\x1b[38;5;255m"
	FgGray    = "\x1b[38;5;245m"

	BgHeader  = "\x1b[48;5;235m"
	BgActive  = "\x1b[48;5;237m"
	BgSelect  = "\x1b[48;5;24m"
	BgAlert   = "\x1b[48;5;52m"
	BgSuccess = "\x1b[48;5;22m"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func runeWidth(r rune) int {
	if r == 0xFE0F || r == 0xFE0E || r < 32 {
		return 0
	}
	if (r >= 0x1F300 && r <= 0x1FAFF) ||
		(r >= 0x2600 && r <= 0x27BF) ||
		(r >= 0x2300 && r <= 0x23FF) ||
		(r >= 0x2B00 && r <= 0x2BFF) ||
		(r >= 0x3000 && r <= 0x9FFF) {
		return 2
	}
	return 1
}

func sanitizeForTerminal(s string) string {
	replacer := strings.NewReplacer(
		"\uFE0F", "",
		"🛡️", "◆",
		"☸️", "⎈",
		"🗄️", "▤",
		"🏳️", "▫",
		"🟧", "▲",
		"⬛", "■",
		"🟨", "◆",
		"🟦", "●",
		"🗺️", "⊞",
		"⌨️", "⌘",
		"♻️", "↻",
		"☁️", "☁",
		"⚠️", "!",
	)
	return replacer.Replace(s)
}

func visibleLen(s string) int {
	clean := ansiRegex.ReplaceAllString(s, "")
	w := 0
	for _, r := range clean {
		w += runeWidth(r)
	}
	return w
}

func padOrTrunc(s string, width int) string {
	s = sanitizeForTerminal(s)
	if width <= 0 {
		return ""
	}
	vLen := visibleLen(s)
	if vLen == width {
		return s
	}
	if vLen < width {
		return s + strings.Repeat(" ", width-vLen)
	}

	target := width - 1
	if target < 0 {
		target = 0
	}
	var out strings.Builder
	curW := 0
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			out.WriteRune(r)
			continue
		}
		if inEsc {
			out.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		rw := runeWidth(r)
		if curW+rw > target {
			break
		}
		out.WriteRune(r)
		curW += rw
	}
	out.WriteString(Reset)
	if curW < width {
		out.WriteString("…")
		curW++
	}
	if curW < width {
		out.WriteString(strings.Repeat(" ", width-curW))
	}
	return out.String()
}

func truncPlain(s string, maxW int) string {
	s = sanitizeForTerminal(s)
	clean := ansiRegex.ReplaceAllString(s, "")
	var out strings.Builder
	w := 0
	for _, r := range clean {
		rw := runeWidth(r)
		if w+rw > maxW {
			out.WriteString("…")
			break
		}
		out.WriteRune(r)
		w += rw
	}
	return out.String()
}

func padPlain(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = sanitizeForTerminal(s)
	clean := ansiRegex.ReplaceAllString(s, "")
	var out strings.Builder
	w := 0
	for _, r := range clean {
		rw := runeWidth(r)
		if w+rw > width {
			if w < width {
				out.WriteString("…")
				w++
			}
			break
		}
		out.WriteRune(r)
		w += rw
	}
	if w < width {
		out.WriteString(strings.Repeat(" ", width-w))
	}
	return out.String()
}

func wrapAnsiLine(s string, maxW int) []string {
	s = sanitizeForTerminal(s)
	if maxW < 25 || visibleLen(s) <= maxW {
		return []string{s}
	}

	// Measure leading plain-space indentation so wrapped lines align neatly underneath
	clean := ansiRegex.ReplaceAllString(s, "")
	leadSpaces := 0
	for _, r := range clean {
		if r == ' ' {
			leadSpaces++
		} else {
			break
		}
	}
	if leadSpaces > 10 {
		leadSpaces = 10
	}
	contIndent := strings.Repeat(" ", leadSpaces+2)
	contIndentW := leadSpaces + 2

	type styledRune struct {
		r    rune
		rw   int
		ansi string
	}

	var tokens []styledRune
	curAnsi := ""
	var escBuf strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			escBuf.Reset()
			escBuf.WriteRune(r)
			continue
		}
		if inEsc {
			escBuf.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
				seq := escBuf.String()
				if seq == Reset {
					curAnsi = ""
				} else {
					curAnsi += seq
				}
			}
			continue
		}
		rw := runeWidth(r)
		if rw == 0 {
			continue
		}
		tokens = append(tokens, styledRune{r: r, rw: rw, ansi: curAnsi})
	}

	var out []string
	idx := 0
	lineNum := 0
	for idx < len(tokens) {
		limitW := maxW
		prefix := ""
		if lineNum > 0 {
			limitW = maxW - contIndentW
			if limitW < 15 {
				limitW = 15
			}
			prefix = contIndent
		}

		w := 0
		endIdx := idx
		lastBreakIdx := -1
		for endIdx < len(tokens) {
			if w+tokens[endIdx].rw > limitW {
				break
			}
			w += tokens[endIdx].rw
			ch := tokens[endIdx].r
			if ch == ' ' || ch == '/' || ch == '|' || ch == ',' {
				lastBreakIdx = endIdx + 1
			}
			endIdx++
		}

		if endIdx < len(tokens) && lastBreakIdx > idx && (endIdx-lastBreakIdx) <= 28 {
			endIdx = lastBreakIdx
		}
		if endIdx == idx {
			endIdx = idx + 1
		}

		var lineBuilder strings.Builder
		lineBuilder.WriteString(prefix)
		prevAnsi := ""
		for k := idx; k < endIdx; k++ {
			if tokens[k].ansi != prevAnsi {
				lineBuilder.WriteString(Reset)
				lineBuilder.WriteString(tokens[k].ansi)
				prevAnsi = tokens[k].ansi
			}
			lineBuilder.WriteRune(tokens[k].r)
		}
		if prevAnsi != "" {
			lineBuilder.WriteString(Reset)
		}
		out = append(out, lineBuilder.String())

		// Trim leading spaces at the start of the next wrapped line
		idx = endIdx
		for idx < len(tokens) && tokens[idx].r == ' ' {
			idx++
		}
		lineNum++
	}

	return out
}

func wrapAnsiLines(lines []string, maxW int) []string {
	var out []string
	for _, l := range lines {
		out = append(out, wrapAnsiLine(l, maxW)...)
	}
	return out
}

func drawBox(title string, borderColor string, lines []string, width, height int) []string {
	title = sanitizeForTerminal(title)
	if width < 6 {
		width = 6
	}
	if height < 3 {
		height = 3
	}
	innerW := width - 2
	var out []string

	titleStr := ""
	if title != "" {
		maxTitleW := innerW - 4
		if maxTitleW > 4 {
			title = truncPlain(title, maxTitleW)
		}
		titleStr = " " + Bold + title + Reset + borderColor + " "
	}
	tLen := visibleLen(titleStr)
	rem := innerW - tLen - 1
	if rem < 0 {
		rem = 0
	}
	top := borderColor + "╭─" + titleStr + strings.Repeat("─", rem) + "╮" + Reset
	out = append(out, top)

	bodyRows := height - 2
	for i := 0; i < bodyRows; i++ {
		var content string
		if i < len(lines) {
			content = padOrTrunc(lines[i], innerW)
		} else {
			content = strings.Repeat(" ", innerW)
		}
		out = append(out, borderColor+"│"+Reset+content+borderColor+"│"+Reset)
	}

	bottom := borderColor + "╰" + strings.Repeat("─", innerW) + "╯" + Reset
	out = append(out, bottom)
	return out
}

func joinHorizontal(left, right []string) []string {
	maxH := len(left)
	if len(right) > maxH {
		maxH = len(right)
	}
	var out []string
	for i := 0; i < maxH; i++ {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out = append(out, l+r)
	}
	return out
}

func riskBadge(r gcp.RiskLevel) string {
	switch r {
	case gcp.RiskCritical:
		return BgAlert + FgRed + Bold + " CRIT " + Reset
	case gcp.RiskHigh:
		return FgOrange + Bold + " HIGH " + Reset
	case gcp.RiskMedium:
		return FgYellow + " MED  " + Reset
	case gcp.RiskLow:
		return FgCyan + " LOW  " + Reset
	default:
		return BgSuccess + FgGreen + Bold + " OK   " + Reset
	}
}

func (a *App) countModuleItems(cat gcp.ResourceCategory) int {
	if cat == gcp.CatTimeMachine {
		return len(a.State.AuditTimeline)
	}
	n := 0
	for _, r := range a.State.Resources {
		if cat == gcp.CatFinOps {
			if r.MonthlyCost > 0 || r.WasteAmount > 0 || r.Category == gcp.CatFinOps {
				n++
			}
		} else if r.Category == cat {
			n++
		}
	}
	return n
}

type subTabDef struct {
	id    InspectorSubTab
	label string
}

// moduleSubTabs returns ONLY the relevant inspector tabs for the active module (no VM YAML, no cross-module clutter).
func (a *App) moduleSubTabs() []subTabDef {
	switch a.ActiveCat {
	case gcp.CatCloudRun:
		return []subTabDef{
			{TabOverview, "[v] Overview"},
			{TabServiceYAML, "[y] YAML"},
			{TabInlineTerminal, "[s] Terminal"},
			{TabTerraform, "[t] Terraform"},
			{TabLogsAI, "[a] Cloud Logs"},
		}
	case gcp.CatGKECompute:
		if isVMResourcePoweredOff(a.CurrentResource()) {
			return []subTabDef{
				{TabOverview, "[v] VM Overview (Powered Off)"},
				{TabTerraform, "[t] 1:1 Terraform"},
				{TabLogsAI, "[a] Cloud Logs"},
			}
		}
		return []subTabDef{
			{TabOverview, "[v] VM / GKE Overview"},
			{TabInlineTerminal, "[s] Live SSH Terminal"},
			{TabTerraform, "[t] 1:1 Terraform"},
			{TabLogsAI, "[a] Cloud Logs"},
		}
	case gcp.CatStorageSQL:
		return []subTabDef{
			{TabOverview, "[v] DB / Bucket Overview"},
			{TabInlineTerminal, "[s] Inline SQL / Bucket Console"},
			{TabTerraform, "[t] 1:1 Terraform"},
		}
	case gcp.CatIAMZero:
		return []subTabDef{
			{TabOverview, "[v] Role & Who Overview"},
			{TabIAMZero, "[z] Permissions & Blast-Radius"},
			{TabTerraform, "[t] 1:1 Terraform"},
		}
	case gcp.CatFinOps:
		return []subTabDef{
			{TabOverview, "[v] Cloud Billing & FinOps Breakdown"},
			{TabTerraform, "[t] 1:1 Terraform"},
		}
	default:
		return []subTabDef{
			{TabOverview, "[v] Resource Overview"},
			{TabTerraform, "[t] 1:1 Terraform"},
			{TabLogsAI, "[a] Cloud Logs"},
		}
	}
}

// RenderFrame renders the full-screen multi-pane lazygcp cockpit adapted to the exact terminal size.
func (a *App) RenderFrame() string {
	a.updateTermSize()
	w := a.Width
	if w < 60 {
		w = 80
	}
	h := a.Height
	if h < 18 {
		h = 24
	}

	var screen []string

	// 1. TOP COCKPIT TELEMETRY BAR (2 rows)
	tunnelCount := len(a.State.ActiveTunnels)
	gradeColor := FgRed
	if a.State.SecurityScore >= 80 {
		gradeColor = FgGreen
	} else if a.State.SecurityScore >= 65 {
		gradeColor = FgYellow
	}

	excessPerms := 0
	for _, r := range a.State.Resources {
		if r.IAMInfo != nil && r.IAMInfo.GrantedPermsCount > r.IAMInfo.UsedPermsCount {
			excessPerms += (r.IAMInfo.GrantedPermsCount - r.IAMInfo.UsedPermsCount)
		}
	}

	projCount := len(a.State.ProjectList)
	if projCount == 0 {
		projCount = 1
	}

	topLine1 := fmt.Sprintf(" %s⚡ lazygcp%s │ Project: %s%s%s (%d avail — Press %s[P]%s to choose) │ Region: %s │ Posture: %s%s (%d/100)%s │ Acct: %s",
		FgCyan+Bold, Reset,
		FgYellow+Bold, a.State.ProjectID, Reset, projCount, FgGreen+Bold, Reset,
		a.State.Region,
		gradeColor+Bold, a.State.Grade, a.State.SecurityScore, Reset,
		a.State.ActiveAccount)

	topLine2 := fmt.Sprintf(" Est. Burn: %s$%.0f/mo%s │ Reclaimable Waste: %s-$%.0f/mo%s │ Live Resources: %s%d%s │ IAM Excess Perms: %s%d%s │ Tunnels: %s%d%s │ %dx%d",
		FgWhite+Bold, a.State.TotalMonthly, Reset,
		FgGreen+Bold, a.State.Reclaimable, Reset,
		FgCyan+Bold, len(a.State.Resources), Reset,
		FgRed+Bold, excessPerms, Reset,
		FgCyan+Bold, tunnelCount, Reset,
		w, h)

	screen = append(screen, BgHeader+padOrTrunc(topLine1, w)+Reset)
	screen = append(screen, BgHeader+padOrTrunc(topLine2, w)+Reset)

	bodyH := h - 4
	if bodyH < 10 {
		bodyH = 10
	}

	leftW := w * 22 / 100
	if leftW < 28 {
		leftW = 28
	}
	if leftW > 44 {
		leftW = 44
	}
	if w < 100 {
		leftW = 24
	}
	rightW := w - leftW

	// 2. LEFT SIDEBAR (9 Core GCP Modules with Live Counts + Project Risk Posture + Quick Actions)
	var sideLines []string
	innerSideW := leftW - 4
	hdrLeft := " KEY MODULE"
	hdrRight := "ITEMS "
	if innerSideW > len(hdrLeft)+len(hdrRight) {
		sideLines = append(sideLines, FgGray+hdrLeft+strings.Repeat(" ", innerSideW-len(hdrLeft)-len(hdrRight))+hdrRight+Reset)
	} else {
		sideLines = append(sideLines, FgGray+" KEY MODULE"+Reset)
	}
	sideLines = append(sideLines, FgGray+" "+strings.Repeat("─", maxInt(4, innerSideW))+Reset)

	for _, cat := range gcp.Categories {
		prefix := "  "
		style := FgWhite
		if cat.ID == a.ActiveCat {
			prefix = FgCyan + Bold + "> " + Reset
			style = BgSelect + FgWhite + Bold
		}
		count := a.countModuleItems(cat.ID)
		countStr := fmt.Sprintf("(%d)", count)
		maxTitle := innerSideW - 7 - len(countStr)
		if maxTitle < 8 {
			maxTitle = 8
		}
		paddedTitle := padPlain(cat.Title, maxTitle)
		countColor := FgGray
		if count > 0 {
			countColor = FgCyan
		}
		line := fmt.Sprintf("%s%s[%s]%s %s %s%s%s",
			prefix, FgCyan, cat.Key, Reset,
			style+paddedTitle+Reset,
			countColor, countStr, Reset)
		sideLines = append(sideLines, line)
	}

	if bodyH > 16 {
		critCount, highCount, medCount, okCount := 0, 0, 0, 0
		for _, r := range a.State.Resources {
			switch r.Risk {
			case gcp.RiskCritical:
				critCount++
			case gcp.RiskHigh:
				highCount++
			case gcp.RiskMedium:
				medCount++
			default:
				okCount++
			}
		}
		sideLines = append(sideLines, "")
		sideLines = append(sideLines, FgPurple+Bold+" ◆ PROJECT RISK POSTURE"+Reset)
		sideLines = append(sideLines, FgGray+" "+strings.Repeat("─", maxInt(4, innerSideW))+Reset)
		sideLines = append(sideLines, fmt.Sprintf(" %sCRITICAL%s : %s%d%s resources", FgRed+Bold, Reset, FgRed+Bold, critCount, Reset))
		sideLines = append(sideLines, fmt.Sprintf(" %sHIGH    %s : %s%d%s resources", FgOrange+Bold, Reset, FgOrange+Bold, highCount, Reset))
		sideLines = append(sideLines, fmt.Sprintf(" %sMEDIUM  %s : %s%d%s resources", FgYellow, Reset, FgYellow+Bold, medCount, Reset))
		sideLines = append(sideLines, fmt.Sprintf(" %sHEALTHY %s : %s%d%s resources", FgGreen, Reset, FgGreen+Bold, okCount, Reset))
		if len(a.State.ActiveTunnels) > 0 {
			sideLines = append(sideLines, "")
			sideLines = append(sideLines, FgGreen+Bold+" 🔌 ACTIVE TUNNELS"+Reset)
			for name, tun := range a.State.ActiveTunnels {
				sideLines = append(sideLines, fmt.Sprintf(" • %s: %s", name, FgCyan+tun+Reset))
			}
		}
	}

	if bodyH-len(sideLines) > 10 {
		sideLines = append(sideLines, "")
		sideLines = append(sideLines, FgCyan+Bold+" ⌘ ENGINEER ACTIONS"+Reset)
		sideLines = append(sideLines, FgGray+" "+strings.Repeat("─", maxInt(4, innerSideW))+Reset)
		sideLines = append(sideLines, fmt.Sprintf(" %s[s/e]%s Inline SSH / Console", FgGreen+Bold, Reset))
		sideLines = append(sideLines, fmt.Sprintf(" %s[t]%s   1:1 Live Terraform HCL", FgCyan+Bold, Reset))
		sideLines = append(sideLines, fmt.Sprintf(" %s[P]%s   List & Choose Project", FgYellow+Bold, Reset))
		sideLines = append(sideLines, fmt.Sprintf(" %s[y]%s   Cloud Run Service YAML", FgCyan+Bold, Reset))
		sideLines = append(sideLines, fmt.Sprintf(" %s[b]%s   Immediate Block FW Rule", FgRed+Bold, Reset))
		sideLines = append(sideLines, fmt.Sprintf(" %s[f]%s   Apply Security/Cost Fix", FgGreen+Bold, Reset))
		sideLines = append(sideLines, fmt.Sprintf(" %s[r]%s   Rollback / Revert Event", FgOrange+Bold, Reset))
		sideLines = append(sideLines, fmt.Sprintf(" %s[w]%s   Export Terraform (.tf)", FgGreen+Bold, Reset))
		sideLines = append(sideLines, fmt.Sprintf(" %s[?]%s   3-Page Reference Guide", FgYellow+Bold, Reset))
	}

	leftBox := drawBox("GCP Modules [1-9]", FgCyan, sideLines, leftW, bodyH)

	// 3. RIGHT PANE: Content-Aware Adaptive Split
	numTableItems := len(a.FilteredResources())
	if a.ActiveCat == gcp.CatTimeMachine {
		numTableItems = len(a.State.AuditTimeline)
	}

	idealTopH := numTableItems + 6
	if a.ActiveCat == gcp.CatFinOps {
		idealTopH = numTableItems + 8
	}
	minTopH := 8
	maxTopH := bodyH * 46 / 100
	if a.InlineTermActive || a.InspectorTab == TabInlineTerminal {
		// Give extra room to the bottom Inline Terminal when active!
		maxTopH = bodyH * 34 / 100
	}
	if maxTopH < minTopH {
		maxTopH = minTopH
	}
	topRightH := idealTopH
	if topRightH < minTopH {
		topRightH = minTopH
	}
	if topRightH > maxTopH {
		topRightH = maxTopH
	}
	botRightH := bodyH - topRightH
	if botRightH < 6 {
		botRightH = 6
		topRightH = bodyH - botRightH
	}

	var rightCombined []string
	if a.ConfirmModal {
		rightCombined = a.renderConfirmModal(rightW, bodyH)
	} else if a.ShowSSHModal {
		rightCombined = a.renderSSHModal(rightW, bodyH)
	} else if a.ShowProjectModal {
		rightCombined = a.renderProjectSelectorModal(rightW, bodyH)
	} else if a.ShowHelpModal {
		rightCombined = a.renderHelpModal(rightW, bodyH)
	} else if a.TermFullscreen && a.InspectorTab == TabInlineTerminal {
		rightCombined = a.renderBottomInspector(rightW, bodyH)
	} else {
		topRightBox := a.renderTopRightTable(rightW, topRightH)
		botRightBox := a.renderBottomInspector(rightW, botRightH)
		rightCombined = append(topRightBox, botRightBox...)
	}

	middle := joinHorizontal(leftBox, rightCombined)
	screen = append(screen, middle...)

	// 4. BOTTOM STATUS & KEYBINDING BAR (2 rows)
	var cmdBar string
	if a.InlineTermActive {
		resName := a.State.ProjectID
		if res := a.CurrentResource(); res != nil {
			resName = res.Name
		}
		cmdBar = fmt.Sprintf(" %s🟢 LIVE SSH / SHELL PTY FOCUS (%s):%s Typing directly into remote PTY (Tab/Arrows/Ctrl+C/cd/vim work!) │ %s[Ctrl+F]%s Full-Height │ %s[Esc / Ctrl+]]%s Detach",
			FgGreen+Bold, resName, Reset, FgYellow+Bold, Reset, FgOrange+Bold, Reset)
	} else if a.FilterMode {
		cmdBar = fmt.Sprintf(" %sFILTER (type to search, Enter/Esc to finish):%s %s█", FgYellow+Bold, Reset, a.FilterQuery)
	} else if a.CommandMode {
		cmdBar = fmt.Sprintf(" %sCOMMAND (:proj, :run, :iam, :gke, :sql, :sec, :net, :ai, :cost, :diff, :ssh, :iap, :tf, :q):%s :%s█", FgCyan+Bold, Reset, a.CommandQuery)
	} else {
		cmdBar = " " + FgCyan + Bold + "STATUS:" + Reset + " " + a.StatusBanner
	}

	keysBar := fmt.Sprintf(" %s[j/k]%s Nav %s[1-9]%s Mod %s[s/S]%s SSH/IAP-Tunnel %s[Ctrl+F]%s FullscreenPTY %s[t]%s 1:1-Terraform %s[P]%s ChooseProject %s[b]%s BlockFW %s[f]%s Fix %s[w]%s Export.tf %s[?]%s Help",
		FgYellow, Reset, FgYellow, Reset, FgGreen+Bold, Reset, FgCyan+Bold, Reset, FgCyan+Bold, Reset, FgYellow+Bold, Reset, FgRed+Bold, Reset, FgGreen+Bold, Reset, FgGreen, Reset, FgYellow+Bold, Reset)

	screen = append(screen, BgActive+padOrTrunc(cmdBar, w)+Reset)
	screen = append(screen, BgHeader+padOrTrunc(keysBar, w)+Reset)

	for i := range screen {
		screen[i] = screen[i] + "\x1b[K"
	}

	return strings.Join(screen, "\r\n")
}

// renderProjectSelectorModal renders an interactive modal to list, filter, and choose any accessible GCP project.
func (a *App) renderProjectSelectorModal(width, height int) []string {
	projs := a.FilteredProjectList()
	innerW := width - 6
	if innerW < 30 {
		innerW = 30
	}

	lines := []string{
		"",
		FgCyan + Bold + "  ☁️  SELECT GOOGLE CLOUD PROJECT (LIVE PROJECT SWITCHER)" + Reset,
		fmt.Sprintf("  %sUse ↑/↓ (or j/k) to navigate, type to filter, and press [Enter] to load project. Press [Esc] to cancel.%s", FgGray, Reset),
		FgGray + "  " + strings.Repeat("─", innerW) + Reset,
		fmt.Sprintf("  %sSearch Filter:%s %s█  %s(%d / %d projects shown)%s",
			FgYellow+Bold, Reset, a.ProjectFilter, FgGray, len(projs), len(a.State.ProjectList), Reset),
		FgGray + "  " + strings.Repeat("─", innerW) + Reset,
	}

	maxListRows := height - 11
	if maxListRows < 3 {
		maxListRows = 3
	}

	startIdx := 0
	if a.ProjectModalIdx >= maxListRows {
		startIdx = a.ProjectModalIdx - maxListRows + 1
	}

	if len(projs) == 0 {
		lines = append(lines, "  "+FgRed+"No matching GCP projects found. Press Backspace or Ctrl+U to clear filter."+Reset)
	} else {
		for i := startIdx; i < len(projs) && i-startIdx < maxListRows; i++ {
			p := projs[i]
			cursor := "   "
			rowStyle := FgWhite
			if i == a.ProjectModalIdx {
				cursor = FgCyan + Bold + " > " + Reset
				rowStyle = BgSelect + FgWhite + Bold
			}
			activeTag := ""
			if p == a.State.ProjectID {
				activeTag = "  " + BgSuccess + FgGreen + Bold + " ● CURRENT ACTIVE PROJECT " + Reset
			}
			line := fmt.Sprintf("%s%s [%2d]  %-44s %s%s",
				cursor, rowStyle, i+1, p, Reset, activeTag)
			lines = append(lines, line)
		}
	}

	lines = append(lines, "")
	lines = append(lines, FgGray+"  "+strings.Repeat("─", innerW)+Reset)
	lines = append(lines, fmt.Sprintf("  Press %s[Enter]%s to switch to selected project │ %s[Ctrl+U]%s Clear filter │ %s[Esc]%s Close",
		BgSuccess+FgGreen+Bold, Reset, FgYellow+Bold, Reset, BgAlert+FgRed+Bold, Reset))

	return drawBox("Choose GCP Project [P]", FgYellow, lines, width, height)
}

func (a *App) renderConfirmModal(width, height int) []string {
	lines := []string{
		"",
		FgYellow + Bold + "  ⚡ LIVE GOOGLE CLOUD COMMAND EXECUTION CONFIRMATION" + Reset,
		FgGray + "  " + strings.Repeat("─", maxInt(10, width-6)) + Reset,
		"",
		fmt.Sprintf("  %sAction :%s %s", FgCyan+Bold, Reset, a.ConfirmTitle),
		fmt.Sprintf("  %sProject:%s %s (%s)", FgCyan+Bold, Reset, a.State.ProjectID, a.State.ActiveAccount),
		"",
		FgWhite + Bold + "  Exact Command to Execute:" + Reset,
		"    $ " + FgGreen + Bold + a.ConfirmCommand + Reset,
		"",
		FgGray + "  " + strings.Repeat("─", maxInt(10, width-6)) + Reset,
		fmt.Sprintf("  Press %s[y / Enter]%s to execute against live GCP, or %s[n / Esc]%s to cancel.",
			BgSuccess+FgGreen+Bold, Reset, BgAlert+FgRed+Bold, Reset),
	}
	return drawBox("Safety Confirmation [y/N]", FgYellow, lines, width, height)
}

// renderSSHModal renders the SSH Connection Mode Selector (Direct SSH vs IAP Tunnel + Best-Practice IAP Auto-Configuration Prompt).
func (a *App) renderSSHModal(width, height int) []string {
	res := a.CurrentResource()
	innerW := width - 6
	if innerW < 30 {
		innerW = 30
	}
	if res == nil {
		return drawBox("SSH Connection Mode Selector", FgCyan, []string{" No VM selected."}, width, height)
	}

	proj := a.State.ProjectID
	vpc := a.vmNetworkName(res)
	iapReady := a.isIAPConfiguredForVM(res)
	netExposure := ""
	if res.Metrics != nil {
		netExposure = res.Metrics["4. Network Exposure"]
	}

	iapBadge := BgAlert + FgYellow + Bold + " ⚠️  IAP FIREWALL RULE NOT DETECTED ON VPC '" + vpc + "' " + Reset
	if iapReady {
		iapBadge = BgSuccess + FgGreen + Bold + " ✅ IAP TCP FORWARDING READY (35.235.240.0/20 -> tcp:22) " + Reset
	}

	lines := []string{
		"",
		fmt.Sprintf("  %s🔐 SELECT SSH CONNECTION MODE FOR COMPUTE VM '%s' (%s)%s", FgCyan+Bold, res.Name, res.Region, Reset),
		fmt.Sprintf("  %sProject:%s %s  │  %sVPC Network:%s %s  │  %sNetwork:%s %s",
			FgGray, Reset, proj, FgGray, Reset, vpc, FgGray, Reset, netExposure),
		fmt.Sprintf("  %sIAP Posture:%s %s", FgGray, Reset, iapBadge),
		FgGray + "  " + strings.Repeat("─", innerW) + Reset,
	}

	if a.SSHModalPromptIAP {
		fwRuleName := "allow-ssh-ingress-from-iap"
		if vpc != "" && vpc != "default" {
			fwRuleName = fmt.Sprintf("allow-ssh-ingress-from-iap-%s", vpc)
		}
		acct := a.State.ActiveAccount
		if acct == "" {
			acct = "<your-gcp-account>"
		}
		lines = append(lines,
			"",
			FgYellow+Bold+"  ⚠️  IDENTITY-AWARE PROXY (IAP) IS NOT CONFIGURED FOR THIS VPC YET!"+Reset,
			"  Would you like lazygcp to automatically configure IAP according to Google Cloud Best Practices and then connect?",
			"",
			FgCyan+Bold+"  Google Cloud Best-Practice IAP Configuration Steps to Execute:"+Reset,
			fmt.Sprintf("    1. %sEnable IAP API%s         : $ gcloud services enable iap.googleapis.com --project=%s", FgWhite+Bold, Reset, proj),
			fmt.Sprintf("    2. %sLeast-Privilege FW Rule%s: $ gcloud compute firewall-rules create %s --network=%s \\", FgWhite+Bold, Reset, fwRuleName, vpc),
			"                                  --direction=INGRESS --action=ALLOW --rules=tcp:22 --source-ranges=35.235.240.0/20",
			fmt.Sprintf("    3. %sGrant IAP Tunnel IAM%s   : $ gcloud projects add-iam-policy-binding %s \\", FgWhite+Bold, Reset, proj),
			fmt.Sprintf("                                  --member=user:%s --role=roles/iap.tunnelResourceAccessor", acct),
			fmt.Sprintf("    4. %sConnect via IAP Tunnel%s : $ gcloud compute ssh %s --zone=%s --project=%s --tunnel-through-iap", FgWhite+Bold, Reset, res.Name, res.Region, proj),
			"",
			FgGray+"  "+strings.Repeat("─", innerW)+Reset,
			fmt.Sprintf("  Press %s[y / Enter]%s Configure IAP Best Practices & Connect │ %s[d]%s Use Direct SSH Instead │ %s[n / Esc]%s Back",
				BgSuccess+FgGreen+Bold, Reset, FgCyan+Bold, Reset, BgAlert+FgRed+Bold, Reset),
		)
		return drawBox("Configure IAP Best Practices? [y/N]", FgYellow, wrapAnsiLines(lines, innerW), width, height)
	}

	options := []struct {
		key   string
		title string
		desc  string
		cmd   string
	}{
		{
			key:   "1",
			title: "Direct SSH (Standard External / Internal IP)",
			desc:  "Connects directly to the VM's IP address on port 22 without an IAP proxy.",
			cmd:   fmt.Sprintf("gcloud compute ssh %s --zone=%s --project=%s", res.Name, res.Region, proj),
		},
		{
			key:   "2",
			title: "IAP Tunnel SSH (Zero-Trust Identity-Aware Proxy)",
			desc:  "Tunnels SSH over HTTPS via Google Identity-Aware Proxy (--tunnel-through-iap). Prompts to configure IAP if missing.",
			cmd:   fmt.Sprintf("gcloud compute ssh %s --zone=%s --project=%s --tunnel-through-iap", res.Name, res.Region, proj),
		},
		{
			key:   "3",
			title: "Configure IAP Best Practices & Connect via IAP Tunnel",
			desc:  "Enables iap.googleapis.com, creates firewall rule (35.235.240.0/20 -> tcp:22), binds roles/iap.tunnelResourceAccessor, and connects.",
			cmd:   fmt.Sprintf("gcloud services enable iap.googleapis.com && gcloud compute firewall-rules create allow-ssh-ingress-from-iap ... && gcloud compute ssh %s --tunnel-through-iap", res.Name),
		},
	}

	for i, opt := range options {
		cursor := "   "
		rowStyle := FgWhite + Bold
		if i == a.SSHModalIdx {
			cursor = FgCyan + Bold + " > " + Reset
			rowStyle = BgSelect + FgWhite + Bold
		}
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%s%s [%s] %s %s", cursor, rowStyle, opt.key, opt.title, Reset))
		lines = append(lines, fmt.Sprintf("       %s%s%s", FgGray, opt.desc, Reset))
		lines = append(lines, fmt.Sprintf("       $ %s%s%s", FgGreen, opt.cmd, Reset))
	}

	lines = append(lines, "")
	lines = append(lines, FgGray+"  "+strings.Repeat("─", innerW)+Reset)
	lines = append(lines, fmt.Sprintf("  Use %s↑/↓%s or press %s[1]%s Direct SSH │ %s[2]%s IAP Tunnel │ %s[3]%s Configure IAP Best Practices │ %s[Enter]%s Launch │ %s[Esc]%s Cancel",
		FgYellow+Bold, Reset, FgCyan+Bold, Reset, FgGreen+Bold, Reset, FgYellow+Bold, Reset, BgSuccess+FgGreen+Bold, Reset, BgAlert+FgRed+Bold, Reset))

	return drawBox("Select VM SSH Connection Mode [s / S]", FgCyan, wrapAnsiLines(lines, innerW), width, height)
}

func (a *App) renderTopRightTable(width, height int) []string {
	catMeta := gcp.Categories[a.ActiveCat]
	title := fmt.Sprintf("[%s :%s] %s — %s", catMeta.Key, catMeta.Alias, catMeta.Title, catMeta.Subtitle)
	if a.FilterQuery != "" {
		title += fmt.Sprintf(" (Filter: %q)", a.FilterQuery)
	}

	innerW := width - 4
	if innerW < 40 {
		innerW = 40
	}
	var rows []string

	// Audit Time-Machine Table (Module 9)
	if a.ActiveCat == gcp.CatTimeMachine {
		colTime := 10
		colRisk := 6
		rem := innerW - colTime - colRisk - 8
		colActor := rem * 30 / 100
		colAction := rem * 34 / 100
		colRes := rem - colActor - colAction

		hdr := fmt.Sprintf("  %s  %s  %s  %s  %s",
			padPlain("TIME (UTC)", colTime),
			padPlain("WHO (PRINCIPAL)", colActor),
			padPlain("WHAT CHANGED (API METHOD)", colAction),
			padPlain("TARGET GCP RESOURCE", colRes),
			"RISK")
		rows = append(rows, FgGray+Bold+hdr+Reset)
		rows = append(rows, FgGray+" "+strings.Repeat("─", innerW)+Reset)

		if len(a.State.AuditTimeline) == 0 {
			rows = append(rows, "  "+FgGray+"No recent Cloud Audit Log entries returned for this project."+Reset)
		}
		visibleDataRows := height - 4
		if visibleDataRows < 1 {
			visibleDataRows = 1
		}
		for i, ev := range a.State.AuditTimeline {
			if len(rows)-2 >= visibleDataRows {
				break
			}
			sel := "  "
			rowStyle := ""
			if i == a.SelectedIdx {
				sel = FgCyan + Bold + "> " + Reset
				rowStyle = BgSelect + FgWhite + Bold
			}
			line := fmt.Sprintf("%s%s%s  %s  %s  %s%s  %s",
				sel, rowStyle,
				padPlain(ev.TimeAgo, colTime),
				padPlain(ev.Actor, colActor),
				padPlain(ev.Action, colAction),
				padPlain(ev.Resource, colRes),
				Reset,
				riskBadge(ev.Risk))
			rows = append(rows, line)
		}
		return drawBox(title, FgPurple, rows, width, height)
	}

	// Module 8 (FinOps & Cloud Billing) Explanatory Banner
	headerRowsCount := 2
	if a.ActiveCat == gcp.CatFinOps {
		bAcct := a.State.BillingAccountID
		if a.State.BillingAccountName != "" {
			bAcct = a.State.BillingAccountName + " (" + a.State.BillingAccountID + ")"
		}
		if bAcct == "" {
			bAcct = "Active GCP Billing"
		}
		daily := a.State.TotalMonthly / 30.0
		annual := a.State.TotalMonthly * 12.0
		finBanner := fmt.Sprintf("  %s💳 CLOUD BILLING:%s %s │ Daily: %s$%.2f/d%s │ Monthly: %s$%.0f/mo%s │ Annual Forecast: %s$%.0f/yr%s │ Reclaimable: %s-$%.0f/mo%s",
			FgYellow+Bold, Reset, bAcct,
			FgCyan+Bold, daily, Reset,
			FgWhite+Bold, a.State.TotalMonthly, Reset,
			FgPurple+Bold, annual, Reset,
			FgGreen+Bold, a.State.Reclaimable, Reset)
		rows = append(rows, finBanner)
		rows = append(rows, FgGray+" "+strings.Repeat("─", innerW)+Reset)
		headerRowsCount = 4
	}

	// Standard Resource Table
	colRisk := 6
	rem := innerW - colRisk - 10
	colName := rem * 32 / 100
	colKind := rem * 20 / 100
	colReg := rem * 13 / 100
	colStat := rem * 20 / 100
	colCost := rem - colName - colKind - colReg - colStat

	nameHdr := "RESOURCE NAME"
	kindHdr := "KIND / TYPE"
	regHdr := "REGION/SCOPE"
	statHdr := "STATUS / POSTURE"
	costHdr := "MONTHLY COST (SAVINGS)"
	if a.ActiveCat == gcp.CatIAMZero {
		nameHdr = "PRINCIPAL (WHO)"
		kindHdr = "GRANTED ROLE (WHAT)"
		regHdr = "SCOPE"
		statHdr = "BLAST RADIUS / IMPACT"
	} else if a.ActiveCat == gcp.CatNetworkVPC {
		nameHdr = "FIREWALL RULE NAME"
		kindHdr = "PRIORITY / TYPE"
		regHdr = "VPC NETWORK"
		statHdr = "DIRECTION & EXPOSED PORTS"
	} else if a.ActiveCat == gcp.CatGKECompute {
		statHdr = "POWER STATE / STATUS"
	}

	hdr := fmt.Sprintf("  %s  %s  %s  %s  %s  %s",
		padPlain(nameHdr, colName),
		padPlain(kindHdr, colKind),
		padPlain(regHdr, colReg),
		padPlain(statHdr, colStat),
		padPlain(costHdr, colCost),
		"RISK")
	rows = append(rows, FgGray+Bold+hdr+Reset)
	rows = append(rows, FgGray+" "+strings.Repeat("─", innerW)+Reset)

	list := a.FilteredResources()
	if len(list) == 0 {
		rows = append(rows, fmt.Sprintf("  %sNo resources found in module '%s' for project '%s'. Press [P] to choose another project or [1-9] for other modules.%s",
			FgGray, catMeta.Title, a.State.ProjectID, Reset))
		return drawBox(title, FgBlue, rows, width, height)
	}

	visibleDataRows := height - headerRowsCount - 2
	if visibleDataRows < 1 {
		visibleDataRows = 1
	}
	if a.SelectedIdx < a.TableOffset {
		a.TableOffset = a.SelectedIdx
	}
	if a.SelectedIdx >= a.TableOffset+visibleDataRows {
		a.TableOffset = a.SelectedIdx - visibleDataRows + 1
	}
	if a.TableOffset < 0 {
		a.TableOffset = 0
	}

	modCost := 0.0
	modWaste := 0.0
	highRiskCount := 0
	for _, item := range list {
		modCost += item.MonthlyCost
		modWaste += item.WasteAmount
		if item.Risk == gcp.RiskCritical || item.Risk == gcp.RiskHigh {
			highRiskCount++
		}
	}

	for i := a.TableOffset; i < len(list) && len(rows)-headerRowsCount < visibleDataRows; i++ {
		r := list[i]
		sel := "  "
		rowStyle := ""
		if i == a.SelectedIdx {
			sel = FgCyan + Bold + "> " + Reset
			rowStyle = BgSelect + FgWhite + Bold
		}
		costStr := fmt.Sprintf("$%.1f/mo", r.MonthlyCost)
		if strings.HasPrefix(r.ID, "finops-billing-account-") {
			costStr = fmt.Sprintf("$%.0f/mo Total", a.State.TotalMonthly)
		} else if r.MonthlyCost >= 1.0 {
			costStr = fmt.Sprintf("$%.0f/mo", r.MonthlyCost)
		}
		if r.WasteAmount > 0 {
			costStr = fmt.Sprintf("$%.0f/mo (Save -$%.0f)", r.MonthlyCost, r.WasteAmount)
		}
		line := fmt.Sprintf("%s%s%s  %s  %s  %s  %s%s  %s",
			sel, rowStyle,
			padPlain(r.Name, colName),
			padPlain(r.Kind, colKind),
			padPlain(r.Region, colReg),
			padPlain(r.Status, colStat),
			padPlain(costStr, colCost),
			Reset,
			riskBadge(r.Risk))
		rows = append(rows, line)
	}

	if len(rows) < height-3 {
		rows = append(rows, FgGray+" "+strings.Repeat("·", innerW)+Reset)
		summaryLine := fmt.Sprintf("  %sModule Telemetry:%s %d item(s) │ Est. Burn: %s$%.0f/mo%s │ Reclaimable Savings: %s-$%.0f/mo%s │ High/Crit Risks: %s%d%s",
			FgCyan+Bold, Reset, len(list),
			FgWhite+Bold, modCost, Reset,
			FgGreen+Bold, modWaste, Reset,
			FgOrange+Bold, highRiskCount, Reset)
		rows = append(rows, summaryLine)
	}

	title += fmt.Sprintf(" (%d/%d)", a.SelectedIdx+1, len(list))
	return drawBox(title, FgBlue, rows, width, height)
}

func (a *App) renderBottomInspector(width, height int) []string {
	innerW := width - 4
	if innerW < 40 {
		innerW = 40
	}

	// Module 9: Cloud Audit Time-Machine Inspector
	if a.ActiveCat == gcp.CatTimeMachine {
		if a.SelectedIdx >= len(a.State.AuditTimeline) {
			return drawBox("Cloud Audit Time-Machine", FgPurple, []string{" No Cloud Audit Log event selected."}, width, height)
		}
		ev := a.State.AuditTimeline[a.SelectedIdx]
		var header []string
		header = append(header, fmt.Sprintf(" %s⏪ CLOUD AUDIT TIME-MACHINE:%s Shows WHO changed WHAT in '%s', from WHICH IP, and lets you inspect or revert it.",
			FgPurple+Bold, Reset, a.State.ProjectID))
		header = append(header, fmt.Sprintf(" %sActions:%s Press %s[e / Enter]%s to toggle FULL Raw Audit JSON (%d lines) │ Press %s[r]%s to run Revert/Undo CLI │ %s[[ / ]]%s Scroll",
			FgYellow+Bold, Reset, FgCyan+Bold, Reset, len(ev.FullJSON), FgOrange+Bold, Reset, FgGray, Reset))
		header = append(header, FgGray+" "+strings.Repeat("─", innerW)+Reset)

		var body []string
		if a.ShowFullAuditJSON && len(ev.FullJSON) > 0 {
			body = append(body, FgCyan+Bold+" FULL RAW CLOUD AUDIT LOG JSON PAYLOAD (cloudaudit.googleapis.com):"+Reset)
			for _, jl := range ev.FullJSON {
				body = append(body, "   "+FgGreen+jl+Reset)
			}
		} else {
			body = append(body, fmt.Sprintf(" %sSummary  :%s %s", FgWhite+Bold, Reset, ev.Summary))
			body = append(body, fmt.Sprintf(" %sWhen     :%s %s (%s)   %sCaller IP:%s %s",
				FgCyan+Bold, Reset, ev.Timestamp, ev.TimeAgo, FgYellow+Bold, Reset, ev.CallerIP))
			body = append(body, fmt.Sprintf(" %sWho      :%s %s (%s)",
				FgYellow+Bold, Reset, ev.Actor, ev.ActorType))
			body = append(body, fmt.Sprintf(" %sWhat     :%s %s   %sTarget:%s %s",
				FgMagenta+Bold, Reset, ev.Action, FgCyan+Bold, Reset, ev.Resource))
			body = append(body, "")
			for _, dl := range ev.DiffLines {
				switch {
				case strings.HasPrefix(dl, "+"):
					body = append(body, " "+FgGreen+dl+Reset)
				case strings.HasPrefix(dl, "-"):
					body = append(body, " "+FgRed+dl+Reset)
				case strings.HasPrefix(dl, "@@"):
					body = append(body, " "+FgCyan+Bold+dl+Reset)
				default:
					body = append(body, " "+FgWhite+dl+Reset)
				}
			}
			body = append(body, "")
			body = append(body, FgGray+" "+strings.Repeat("·", innerW)+Reset)
			body = append(body, fmt.Sprintf(" %s1) Press [e] or [Enter] — View Complete Raw JSON Audit Log Entry (%d lines)%s", FgCyan+Bold, len(ev.FullJSON), Reset))
			body = append(body, fmt.Sprintf(" %s2) Press [r] — Execute Suggested Undo / Revert Action for This Change:%s", FgOrange+Bold, Reset))
			body = append(body, fmt.Sprintf("    $ %s%s%s", FgGreen+Bold, ev.Rollback, Reset))
		}

		body = wrapAnsiLines(body, innerW)
		if a.InspectorScroll >= len(body) {
			a.InspectorScroll = maxInt(0, len(body)-1)
		}
		lines := append(header, body[a.InspectorScroll:]...)
		title := fmt.Sprintf("Audit Event Inspector: %s by %s", ev.Action, ev.Actor)
		if a.ShowFullAuditJSON {
			title += " [FULL JSON VIEW]"
		}
		if a.InspectorScroll > 0 {
			title += fmt.Sprintf(" [Scroll +%d]", a.InspectorScroll)
		}
		return drawBox(title, FgPurple, lines, width, height)
	}

	res := a.CurrentResource()
	if res == nil {
		return drawBox("X-Ray Inspector", FgCyan, []string{" No resource selected in this module. Use [1-9] to switch modules or [P] to choose a GCP project."}, width, height)
	}

	tabs := a.moduleSubTabs()
	var tabHeader []string
	for _, t := range tabs {
		if t.id == a.InspectorTab {
			tabHeader = append(tabHeader, BgSelect+FgWhite+Bold+" "+t.label+" "+Reset)
		} else {
			tabHeader = append(tabHeader, FgGray+" "+t.label+" "+Reset)
		}
	}

	var headerLines []string
	headerLines = append(headerLines, " "+strings.Join(tabHeader, "│")+"  "+FgGray+"([ / ] Scroll, [Tab] Switch Tab)"+Reset)
	headerLines = append(headerLines, FgGray+" "+strings.Repeat("─", innerW)+Reset)

	// Dedicated Inline Terminal Renderer
	if a.InspectorTab == TabInlineTerminal {
		return a.renderInlineTerminalBox(res, headerLines, width, height, innerW)
	}

	var contentLines []string

	switch a.InspectorTab {
	case TabOverview:
		// Strict Isolation for Module 8 (FinOps & Cloud Billing): Rich Cloud Billing & Service Cost Breakdown!
		if a.ActiveCat == gcp.CatFinOps {
			var computeSpend, sqlStorageSpend, serverlessSpend, diskWasteSpend float64
			for _, r := range a.State.Resources {
				switch r.Category {
				case gcp.CatGKECompute:
					computeSpend += r.MonthlyCost
				case gcp.CatStorageSQL:
					sqlStorageSpend += r.MonthlyCost
				case gcp.CatCloudRun, gcp.CatSecretsEnv, gcp.CatVertexAI:
					serverlessSpend += r.MonthlyCost
				case gcp.CatFinOps:
					diskWasteSpend += r.MonthlyCost
				}
			}
			tot := maxFloat(1.0, a.State.TotalMonthly)
			bAcct := a.State.BillingAccountID
			if a.State.BillingAccountName != "" {
				bAcct = a.State.BillingAccountName + " (" + a.State.BillingAccountID + ")"
			}
			bStateStr := FgGreen + Bold + "ENABLED (Active Billing)" + Reset
			if !a.State.BillingEnabled {
				bStateStr = FgYellow + Bold + "DISABLED / FREE TIER" + Reset
			}

			contentLines = append(contentLines, FgYellow+Bold+" 💳 CLOUD BILLING & FINOPS PERSPECTIVE FOR PROJECT '"+a.State.ProjectID+"'"+Reset)
			contentLines = append(contentLines, fmt.Sprintf("   • Billing Account & Status  : %s%s%s — %s", FgCyan+Bold, bAcct, Reset, bStateStr))
			contentLines = append(contentLines, fmt.Sprintf("   • Run-Rate Forecast         : %s$%.2f / day%s  │  %s$%.2f / month%s  │  %s$%.0f / year%s (%s)",
				FgCyan+Bold, a.State.TotalMonthly/30.0, Reset,
				FgWhite+Bold, a.State.TotalMonthly, Reset,
				FgPurple+Bold, a.State.TotalMonthly*12.0, Reset, a.State.BillingCurrency))
			contentLines = append(contentLines, fmt.Sprintf("   • Reclaimable Idle Savings  : %s-$%.2f / month (-$%.0f / year)%s",
				FgGreen+Bold, a.State.Reclaimable, a.State.Reclaimable*12.0, Reset))
			contentLines = append(contentLines, "")
			contentLines = append(contentLines, FgWhite+Bold+" 📊 PROJECT SPEND BREAKDOWN BY GCP SERVICE FAMILY:"+Reset)
			renderSpendBar := func(label string, amount float64, color string) string {
				pct := int(amount * 100.0 / tot)
				if pct > 100 {
					pct = 100
				}
				filled := pct * 24 / 100
				bar := strings.Repeat("█", filled) + strings.Repeat("░", 24-filled)
				return fmt.Sprintf("   • %-28s [%s%s%s] %3d%%  %s$%.2f/mo%s ($%.0f/yr)",
					label, color, bar, Reset, pct, FgWhite+Bold, amount, Reset, amount*12.0)
			}
			contentLines = append(contentLines, renderSpendBar("Compute Engine VMs & GKE", computeSpend, FgCyan))
			contentLines = append(contentLines, renderSpendBar("Cloud SQL & GCS Storage", sqlStorageSpend, FgBlue))
			contentLines = append(contentLines, renderSpendBar("Cloud Run, Functions & KMS", serverlessSpend, FgGreen))
			contentLines = append(contentLines, renderSpendBar("Unattached Disks / Waste", diskWasteSpend, FgOrange))

			contentLines = append(contentLines, "")
			contentLines = append(contentLines, FgGray+" "+strings.Repeat("·", innerW)+Reset)
			contentLines = append(contentLines, FgYellow+Bold+" 🔍 SELECTED ITEM COST & OPTIMIZATION: "+res.Name+" ("+res.Kind+")"+Reset)
			if strings.HasPrefix(res.ID, "finops-billing-account-") {
				contentLines = append(contentLines, fmt.Sprintf("   • Billing Summary           : %s", res.Summary))
				contentLines = append(contentLines, fmt.Sprintf("   • Inspect Billing via CLI   : $ %s%s%s", FgGreen+Bold, res.GcloudCmd, Reset))
			} else {
				sharePct := 100.0 * res.MonthlyCost / tot
				contentLines = append(contentLines, fmt.Sprintf("   • Resource Monthly Burn     : %s$%.2f / month%s (%.1f%% of project bill)", FgWhite+Bold, res.MonthlyCost, Reset, sharePct))
				if res.WasteAmount > 0 {
					contentLines = append(contentLines, fmt.Sprintf("   • Reclaimable Monthly Waste : %s-$%.2f / month (Save %.0f%%!)%s",
						FgGreen+Bold, res.WasteAmount, 100.0*res.WasteAmount/maxFloat(1.0, res.MonthlyCost), Reset))
				} else {
					contentLines = append(contentLines, fmt.Sprintf("   • Efficiency Posture        : %sOPTIMAL ($0 idle waste)%s", FgGreen+Bold, Reset))
				}
				contentLines = append(contentLines, fmt.Sprintf("   • Cost Driver Details       : %s", res.Summary))
				contentLines = append(contentLines, fmt.Sprintf("   • 1-Key Optimization [f]    : $ %s%s%s", FgGreen+Bold, res.GcloudCmd, Reset))
			}
			break
		}

		contentLines = append(contentLines, fmt.Sprintf(" %sSummary :%s %s", FgCyan+Bold, Reset, res.Summary))
		if res.URL != "" || res.ServiceAcct != "" {
			contentLines = append(contentLines, fmt.Sprintf(" %sEndpoint:%s %-48s   %sIdentity:%s %s",
				FgCyan+Bold, Reset, res.URL, FgYellow+Bold, Reset, res.ServiceAcct))
		}
		if res.LocalTunnel != "" {
			contentLines = append(contentLines, fmt.Sprintf(" %sActive Local Tunnel:%s %s", FgGreen+Bold, Reset, res.LocalTunnel))
		}

		if len(res.Metrics) > 0 {
			contentLines = append(contentLines, "")
			contentLines = append(contentLines, FgWhite+Bold+" LIVE CONFIGURATION & TELEMETRY:"+Reset)
			var keys []string
			for k := range res.Metrics {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v := res.Metrics[k]
				valColor := FgWhite
				if strings.Contains(v, "EXPOSED") || strings.Contains(v, "DISABLED") || strings.Contains(v, "POWERED OFF") || strings.Contains(v, "editor") || strings.Contains(v, "owner") || strings.Contains(v, "0.0.0.0/0") {
					valColor = FgRed + Bold
				} else if strings.Contains(v, "UNMASKED LIVE") || strings.Contains(v, "enforced") || strings.Contains(v, "POWERED ON") || strings.Contains(v, "READY") {
					valColor = FgGreen + Bold
				}
				contentLines = append(contentLines, fmt.Sprintf("   • %-28s : %s%s%s", k, valColor, v, Reset))
			}
		}

		if len(res.Revisions) > 0 && a.ActiveCat == gcp.CatCloudRun {
			contentLines = append(contentLines, "")
			contentLines = append(contentLines, FgWhite+Bold+" CLOUD RUN REVISION TRAFFIC SPLIT (Press [r] for 1-keystroke rollback):"+Reset)
			barW := 30
			if innerW > 120 {
				barW = 40
			}
			for _, rev := range res.Revisions {
				filled := rev.Percent * barW / 100
				if filled < 0 {
					filled = 0
				}
				if filled > barW {
					filled = barW
				}
				bar := strings.Repeat("#", filled) + strings.Repeat("-", barW-filled)
				c := FgGreen
				if strings.Contains(rev.Status, "DEGRADED") || strings.Contains(rev.Status, "STORM") {
					c = FgRed + Bold
				}
				contentLines = append(contentLines, fmt.Sprintf("   [%s%s%s] %3d%%  %-32s  %s (%s)", c, bar, Reset, rev.Percent, rev.Revision, c+rev.Status+Reset, rev.Created))
			}
		}

		if len(res.EnvVars) > 0 && a.ActiveCat == gcp.CatCloudRun {
			contentLines = append(contentLines, "")
			contentLines = append(contentLines, FgWhite+Bold+" ENVIRONMENT VARIABLES & SECRET BINDINGS (Press [x] to unmask, [y] for full YAML):"+Reset)
			for _, ev := range res.EnvVars {
				val := ev.Value
				if ev.IsExposed && !a.UnmaskSecrets {
					val = "**************** (PLAINTEXT SECRET! Press [x] to peek, [f] to fix)"
				}
				badge := FgGreen + "[ENV]   " + Reset
				if ev.IsSecret {
					badge = FgCyan + "[SECRET]" + Reset
				} else if ev.IsExposed {
					badge = BgAlert + FgRed + Bold + "[LEAKED]" + Reset
				}
				contentLines = append(contentLines, fmt.Sprintf("   %s %-28s = %s", badge, ev.Name, val))
			}
		}

		if height > 14 && res.AIInsight != "" {
			contentLines = append(contentLines, "")
			contentLines = append(contentLines, FgGray+" "+strings.Repeat("·", innerW)+Reset)
			contentLines = append(contentLines, FgMagenta+Bold+" ENGINEER GUIDE & AVAILABLE ACTIONS:"+Reset)
			for _, al := range strings.Split(res.AIInsight, "\n") {
				contentLines = append(contentLines, "   "+FgYellow+al+Reset)
			}
		}

	case TabServiceYAML:
		contentLines = append(contentLines, fmt.Sprintf(" %s📄 LIVE CLOUD RUN / FUNCTION YAML MANIFEST — %s (%s):%s", FgCyan+Bold, res.Name, res.Kind, Reset))
		contentLines = append(contentLines, fmt.Sprintf(" %s(Use [ and ] to scroll YAML, or press [t] for 1:1 Terraform HCL)%s", FgGray, Reset))
		contentLines = append(contentLines, "")
		if res.ManifestYAML != "" {
			for _, yl := range strings.Split(res.ManifestYAML, "\n") {
				trimmed := strings.TrimSpace(yl)
				color := FgWhite
				if strings.HasPrefix(trimmed, "#") {
					color = FgGray
				} else if strings.HasPrefix(trimmed, "apiVersion:") || strings.HasPrefix(trimmed, "kind:") || strings.HasPrefix(trimmed, "metadata:") || strings.HasPrefix(trimmed, "spec:") || strings.HasPrefix(trimmed, "status:") {
					color = FgCyan + Bold
				} else if strings.Contains(yl, ":") {
					color = FgGreen
				}
				contentLines = append(contentLines, "   "+color+yl+Reset)
			}
		} else {
			contentLines = append(contentLines, "   "+FgGray+"YAML manifest is only applicable to Cloud Run & Cloud Functions. Press [t] for 1:1 Terraform HCL."+Reset)
		}

	case TabIAMZero:
		contentLines = append(contentLines, FgPurple+Bold+" ◆ IAM IDENTITY, ROLE CAPABILITIES & LEAST-PRIVILEGE INSPECTOR"+Reset)
		if res.IAMInfo != nil {
			info := res.IAMInfo
			reduction := 0.0
			if info.GrantedPermsCount > 0 {
				reduction = 100.0 * float64(info.GrantedPermsCount-info.UsedPermsCount) / float64(info.GrantedPermsCount)
			}
			contentLines = append(contentLines, fmt.Sprintf("   1. Who (Principal)      : %s%s%s", FgYellow+Bold, info.Principal, Reset))
			contentLines = append(contentLines, fmt.Sprintf("   2. Identity Type        : %s%s%s", FgCyan+Bold, info.PrincipalType, Reset))
			contentLines = append(contentLines, fmt.Sprintf("   3. Granted GCP Role     : %s%s%s", FgRed+Bold, info.Role, Reset))
			contentLines = append(contentLines, fmt.Sprintf("   4. What This Role Does  : %s", info.RoleDescription))
			contentLines = append(contentLines, fmt.Sprintf("   5. Blast-Radius Ratio   : %s~%d Permissions Granted%s vs. %s~%d Minimal Needed%s (%.1f%% Excess Exposure)",
				FgRed+Bold, info.GrantedPermsCount, Reset,
				FgGreen+Bold, info.UsedPermsCount, Reset, reduction))
			if len(info.UnusedRiskPerms) > 0 {
				contentLines = append(contentLines, "")
				contentLines = append(contentLines, FgRed+Bold+"   DANGEROUS CAPABILITIES EXPOSED BY THIS ROLE RIGHT NOW:"+Reset)
				for _, up := range info.UnusedRiskPerms {
					contentLines = append(contentLines, fmt.Sprintf("      ✗ %s%s%s", FgRed, up, Reset))
				}
			}
			contentLines = append(contentLines, "")
			contentLines = append(contentLines, FgGreen+Bold+"   RECOMMENDED LEAST-PRIVILEGE PERMISSIONS FOR WORKLOADS:"+Reset)
			for _, p := range info.ActuallyUsedPerms {
				contentLines = append(contentLines, fmt.Sprintf("      ✓ %s", p))
			}
			contentLines = append(contentLines, "")
			contentLines = append(contentLines, fmt.Sprintf("   %sAction:%s Press %s[f]%s to revoke/downgrade this binding, or %s[t]%s to export 1:1 Terraform HCL.",
				FgCyan+Bold, Reset, FgGreen+Bold, Reset, FgCyan+Bold, Reset))
		} else {
			sa := res.ServiceAcct
			if sa == "" {
				sa = "Project Default / Inherited IAM Policy"
			}
			contentLines = append(contentLines, fmt.Sprintf("   Resource Name          : %s (%s)", res.Name, res.Kind))
			contentLines = append(contentLines, fmt.Sprintf("   Attached Identity      : %s%s%s", FgYellow+Bold, sa, Reset))
			contentLines = append(contentLines, fmt.Sprintf("   Identity Classification: %s", gcpClassifyIdentityNote(sa)))
		}

	case TabTopology:
		contentLines = append(contentLines, FgCyan+Bold+" BLAST-RADIUS & DEPENDENCY TOPOLOGY MAP:"+Reset)
		if len(res.TopologyASCII) > 0 {
			for _, tl := range res.TopologyASCII {
				contentLines = append(contentLines, "   "+tl)
			}
		} else {
			contentLines = append(contentLines, fmt.Sprintf("   [Project: %s] ──► [%s: %s (%s)]", a.State.ProjectID, res.Kind, res.Name, res.Status))
		}
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, fmt.Sprintf(" %sgcloud CLI:%s $ %s%s%s", FgYellow+Bold, Reset, FgGreen, res.GcloudCmd, Reset))

	case TabTerraform:
		contentLines = append(contentLines, fmt.Sprintf(" %s🏗️  1:1 REVERSE-ENGINEERED TERRAFORM HCL (LIVE STATE OF '%s') — Press [w] to save .tf file:%s", FgGreen+Bold, res.Name, Reset))
		contentLines = append(contentLines, fmt.Sprintf(" %s(Use [ and ] to scroll Terraform block)%s", FgGray, Reset))
		contentLines = append(contentLines, "")
		for _, hl := range strings.Split(res.TerraformHCL, "\n") {
			trimmed := strings.TrimSpace(hl)
			color := FgWhite
			if strings.HasPrefix(trimmed, "#") {
				color = FgYellow
			} else if strings.HasPrefix(trimmed, "resource ") {
				color = FgCyan + Bold
			} else if strings.HasSuffix(trimmed, "{") || trimmed == "}" {
				color = FgPurple + Bold
			} else if strings.Contains(hl, "=") {
				color = FgGreen
			}
			contentLines = append(contentLines, "   "+color+hl+Reset)
		}

	case TabLogsAI:
		contentLines = append(contentLines, FgMagenta+Bold+" ENGINEER & SRE ADVISOR:"+Reset)
		for _, al := range strings.Split(res.AIInsight, "\n") {
			contentLines = append(contentLines, "   "+FgYellow+al+Reset)
		}
		contentLines = append(contentLines, "")
		contentLines = append(contentLines, fmt.Sprintf(" %sLIVE CLOUD LOGGING STREAM (%s):%s", FgCyan+Bold, res.Name, Reset))
		if len(res.Logs) > 0 {
			for _, lg := range res.Logs {
				sevColor := FgGreen
				if lg.Severity == "ERROR" || lg.Severity == "CRITICAL" {
					sevColor = FgRed + Bold
				} else if lg.Severity == "WARN" {
					sevColor = FgYellow
				}
				contentLines = append(contentLines, fmt.Sprintf("   %s [%s%-8s%s] (%s) %s", FgGray+lg.Timestamp+Reset, sevColor, lg.Severity, Reset, lg.Component, lg.Message))
			}
		} else {
			contentLines = append(contentLines, "   "+FgGray+"Press [a] to fetch live logs from Cloud Logging for this resource."+Reset)
		}
	}

	// Wrap long lines cleanly across multiple lines before scrolling so no text is truncated with "…"
	contentLines = wrapAnsiLines(contentLines, innerW)

	if a.InspectorScroll >= len(contentLines) {
		a.InspectorScroll = maxInt(0, len(contentLines)-1)
	}
	scrolled := contentLines[a.InspectorScroll:]
	lines := append(headerLines, scrolled...)

	inspectorTitle := fmt.Sprintf("X-Ray Inspector: %s (%s)", res.Name, res.Kind)
	if a.InspectorScroll > 0 {
		inspectorTitle += fmt.Sprintf(" [Scroll +%d]", a.InspectorScroll)
	}
	return drawBox(inspectorTitle, FgCyan, lines, width, height)
}

// renderInlineTerminalBox renders a real, stateful POSIX PTY SSH / shell session directly inside the TUI box.
func (a *App) renderInlineTerminalBox(res *gcp.GCPResource, headerLines []string, width, height, innerW int) []string {
	if isVMResourcePoweredOff(res) {
		allLines := append([]string{}, headerLines...)
		allLines = append(allLines,
			"",
			fmt.Sprintf("   %s⚠️  SSH TERMINAL DISABLED: Compute VM '%s' is currently %s.%s", FgRed+Bold, res.Name, res.Status, Reset),
			fmt.Sprintf("   %sPress [f] or [r] to Power On ('gcloud compute instances start %s') before opening an SSH session.%s", FgYellow, res.Name, Reset),
		)
		return drawBox(fmt.Sprintf("⌘ Live SSH Terminal: %s (POWERED OFF)", res.Name), FgRed, allLines, width, height)
	}

	sess := a.PTYSessions[res.ID]

	var statusHdr string
	borderColor := FgCyan
	if a.InlineTermActive {
		borderColor = FgGreen
		statusHdr = fmt.Sprintf(" %s🟢 LIVE SSH / SHELL TERMINAL (%s)%s — All keys go to remote PTY │ %s[Ctrl+F]%s Toggle Full-Height │ %s[Esc / Ctrl+]]%s Detach",
			FgGreen+Bold, res.Name, Reset, FgYellow+Bold, Reset, FgOrange+Bold, Reset)
	} else {
		statusHdr = fmt.Sprintf(" %s⚪ SSH / SHELL TERMINAL DETACHED (%s)%s — Press %s[s / Enter]%s to attach │ Press %s[S]%s to switch Direct SSH / IAP Tunnel",
			FgYellow+Bold, res.Name, Reset, FgGreen+Bold, Reset, FgCyan+Bold, Reset)
	}

	topLines := append([]string{}, headerLines...)
	topLines = append(topLines, statusHdr)
	topLines = append(topLines, FgGray+" "+strings.Repeat("─", innerW)+Reset)

	bodyRows := height - 2
	ptyRows := bodyRows - len(topLines)
	if ptyRows < 4 {
		ptyRows = 4
	}
	ptyCols := innerW - 2
	if ptyCols < 40 {
		ptyCols = 40
	}

	if sess == nil {
		allLines := append(topLines,
			"",
			fmt.Sprintf("   %sNo active SSH / Shell session for '%s'. Press [s] or [Enter] to choose Direct SSH or IAP Tunnel!%s", FgYellow+Bold, res.Name, Reset),
		)
		return drawBox(fmt.Sprintf("⌘ Live SSH Terminal: %s (%s)", res.Name, res.Region), borderColor, allLines, width, height)
	}

	// Dynamically sync PTY window dimensions (TIOCSWINSZ) with the exact TUI pane size!
	sess.Resize(ptyCols, ptyRows)
	ptyLines, remoteTitle, exited, exitMsg := sess.RenderLines(a.InlineTermActive)

	allLines := append(topLines, ptyLines...)
	if exited {
		allLines = append(allLines, fmt.Sprintf(" %s[PTY Session Ended: %s — Press [s] or [S] to start a new Direct SSH or IAP Tunnel session]%s", FgYellow+Bold, exitMsg, Reset))
	}

	boxTitle := fmt.Sprintf("⌘ Live SSH / Shell PTY: %s (%s)", res.Name, res.Region)
	if remoteTitle != "" {
		boxTitle = fmt.Sprintf("⌘ %s [%s]", remoteTitle, res.Name)
	}
	if a.InlineTermActive {
		boxTitle += " [LIVE PTY CONNECTED]"
	}
	if a.TermFullscreen {
		boxTitle += " [FULL-HEIGHT]"
	}
	return drawBox(boxTitle, borderColor, allLines, width, height)
}

func gcpClassifyIdentityNote(sa string) string {
	if strings.Contains(sa, "-compute@developer.gserviceaccount.com") {
		return "Google Default Compute Service Account (Shared across project VMs & Cloud Run services)"
	}
	if strings.Contains(sa, "gserviceaccount.com") {
		return "Dedicated Service Account"
	}
	return "Inherited Project IAM Policy"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (a *App) renderHelpModal(width, height int) []string {
	pageTabs := []string{
		"[1] Keyboard & Actions",
		"[2] Modules 1-9 Guide",
		"[3] CLI & Workflow Examples",
	}
	var hdr []string
	for i, pt := range pageTabs {
		if i == a.HelpPage {
			hdr = append(hdr, BgSelect+FgWhite+Bold+" "+pt+" "+Reset)
		} else {
			hdr = append(hdr, FgGray+" "+pt+" "+Reset)
		}
	}

	var lines []string
	lines = append(lines, " "+strings.Join(hdr, " │ ")+"   "+FgYellow+"(Tab / ← / → to switch page, Esc to close)"+Reset)
	lines = append(lines, FgGray+" "+strings.Repeat("─", maxInt(10, width-4))+Reset)

	switch a.HelpPage {
	case 0:
		lines = append(lines,
			FgCyan+Bold+"  NAVIGATION & VIEWPORT SCROLLING"+Reset,
			"    ↑/↓ or j/k          Move selection up / down in the active resource table",
			"    PgUp / PgDn         Page-scroll main resource table up / down (or Ctrl+U / Ctrl+D)",
			"    [ / ]               Scroll bottom X-Ray Inspector pane up / down",
			"    ←/→ or h/l          Switch to previous / next GCP module in the left sidebar",
			"    1 - 9               Jump directly to GCP Module 1..9",
			"    Tab                 Cycle bottom X-Ray Inspector tabs for the current module",
			"    v/s/y/z/t/a         Jump directly to Inspector sub-tab (v:Overview, s:Inline Terminal, y:CloudRun YAML, t:1:1 Terraform, a:Cloud Logs)",
			"    /                   Open live fuzzy search filter across resource names, regions, kinds & statuses",
			"    :                   Open k9s-style Command Palette (:proj, :run, :iam, :gke, :sql, :sec, :net, :ai, :cost, :diff, :iap)",
			"",
			FgGreen+Bold+"  1-KEYSTROKE ENGINEER ACTIONS"+Reset,
			"    s / e / Enter       Open Embedded Inline Terminal (on VMs: opens Direct SSH vs IAP Tunnel selector + IAP auto-setup)",
			"    S                   Open VM SSH Mode Selector (Direct SSH, IAP Tunnel, or Configure IAP Best Practices)",
			"    P                   Open Interactive GCP Project Selector Modal (list, search & switch projects)",
			"    t                   View 1:1 Reverse-Engineered Terraform HCL mapped from live GCP state (press [w] to save .tf)",
			"    y                   View live Knative service.yaml for Cloud Run & Cloud Functions (Module [1])",
			"    b                   IMMEDIATELY BLOCK (disable) or re-enable a VPC Firewall Rule in Module [6]",
			"    f                   Apply Security / Least-Privilege / FinOps remediation (opens [y/N] confirmation modal)",
			"    r                   Rollback Cloud Run traffic or execute Undo/Revert command for a Cloud Audit Log event",
		)
	case 1:
		lines = append(lines,
			FgCyan+Bold+"  COMPLETE GUIDE TO THE 9 GCP COCKPIT MODULES"+Reset,
			"    [1 :run]  Cloud Run & Functions   View env vars, full Knative service.yaml [y], inline HTTP probe/shell [s] & 1:1 Terraform [t].",
			"    [2 :iam]  IAM Policies & Roles    Plain-English breakdown of WHO has access, WHAT their role allows, blast radius & 1-key revoke [f].",
			"    [3 :gke]  GKE & Compute VMs       Embedded Inline SSH Terminal [s]/[e] right inside the bottom pane + 1:1 VM Terraform [t].",
			"    [4 :sql]  Cloud SQL & Storage     Inline GCS Bucket file browser ('ls -l', 'du', 'cat <file>') & Cloud SQL console [s]/[e].",
			"    [5 :sec]  Secrets & KMS Vault     Audit Secret Manager rotation policies, creation timestamps & decrypt live payloads [x].",
			"    [6 :net]  VPC, NAT & Firewalls    Inspect exposed ports/CIDRs, IMMEDIATELY BLOCK rules [b], or lock 0.0.0.0/0 to Google IAP [f].",
			"    [7 :ai]   Vertex AI & GenAI       Detect enabled Vertex/Gemini APIs & export 1:1 google_project_service Terraform [t].",
			"    [8 :cost] FinOps & Cost Guide     Clean monthly cost breakdown across all project resources, waste analysis & 1-key savings [f].",
			"    [9 :diff] Audit Time-Machine      Who changed what & when, toggle full raw Audit Log JSON [e], or run context-aware Revert CLI [r].",
		)
	case 2:
		lines = append(lines,
			FgCyan+Bold+"  PRACTICAL WORKFLOW EXAMPLES"+Reset,
			"    • Inline SSH on a Compute VM       : Go to [3 :gke], select VM, press [s] or [e], type 'whoami' or 'df -h' & press [Enter] inline!",
			"    • Reverse-Engineer 1:1 Terraform   : Select any VM, Bucket, SQL, GKE, or Firewall rule and press [t] (then [w] to save .tf)",
			"    • List & Switch GCP Projects       : Press [P] (or type :proj) to open the Project Selector Modal, filter, and press [Enter]",
			"    • Browse Files in a GCS Bucket     : Go to [4 :sql], select gs://bucket, press [s] or [e] to run 'ls -l' or 'cat file.txt' inline",
			"    • Immediately Block a Firewall Rule: Go to [6 :net], select firewall rule, press [b] -> [y] to disable it immediately in GCP",
		)
	}

	return drawBox("⌘ lazygcp Extended Help & Reference Manual (Page "+fmt.Sprintf("%d/3", a.HelpPage+1)+")", FgYellow, lines, width, height)
}
