package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/damiansztankowski/lazygcp/internal/gcp"
)

type InspectorSubTab int

const (
	TabOverview InspectorSubTab = iota
	TabInlineTerminal
	TabServiceYAML
	TabIAMZero
	TabTopology
	TabTerraform
	TabLogsAI
)

type App struct {
	State             *gcp.ProjectState
	Width             int
	Height            int
	ActiveCat         gcp.ResourceCategory
	SelectedIdx       int
	TableOffset       int
	InspectorTab      InspectorSubTab
	InspectorScroll   int
	FocusPane         int
	FilterMode        bool
	FilterQuery       string
	CommandMode       bool
	CommandQuery      string
	ShowHelpModal     bool
	HelpPage          int
	ShowProjectModal  bool
	ProjectModalIdx   int
	ProjectFilter     string
	ShowSSHModal      bool
	SSHModalIdx       int
	SSHModalPromptIAP bool
	ConfirmModal      bool
	ConfirmTitle      string
	ConfirmCommand    string
	ConfirmKind       string // "exec", "tunnel", "secret", "block"
	InlineTermActive  bool
	TermFullscreen    bool
	PTYSessions       map[string]*PTYSession
	PTYNotifyCh       chan struct{}
	ShowFullAuditJSON bool
	UnmaskSecrets     bool
	StatusBanner      string
	StreamCounter     int
	ProjectIdx        int
}

func NewApp(state *gcp.ProjectState) *App {
	app := &App{
		State:        state,
		Width:        110,
		Height:       30,
		ActiveCat:    gcp.CatCloudRun,
		SelectedIdx:  0,
		InspectorTab: TabOverview,
		FocusPane:    1,
		PTYSessions:  make(map[string]*PTYSession),
		PTYNotifyCh:  make(chan struct{}, 16),
		StatusBanner: "lazygcp ready! [1-9] Modules, [s] Live SSH Terminal, [P] Choose Project, [t] 1:1 Terraform, [b] Block FW, [?] Help",
	}
	app.updateTermSize()
	return app
}

// ActivePTYSession returns the live PTYSession for the currently selected resource, if any.
func (a *App) ActivePTYSession() *PTYSession {
	res := a.CurrentResource()
	if res == nil {
		return nil
	}
	return a.PTYSessions[res.ID]
}

// CloseAllPTYSessions cleans up any running SSH/shell PTY child processes when lazygcp exits.
func (a *App) CloseAllPTYSessions() {
	for _, s := range a.PTYSessions {
		if s != nil {
			s.Close()
		}
	}
}

// FilteredProjectList returns the list of GCP projects matching ProjectFilter in the [P] Project Picker Modal.
func (a *App) FilteredProjectList() []string {
	q := strings.ToLower(strings.TrimSpace(a.ProjectFilter))
	var out []string
	for _, p := range a.State.ProjectList {
		if q == "" || strings.Contains(strings.ToLower(p), q) {
			out = append(out, p)
		}
	}
	if len(out) == 0 && a.State.ProjectID != "" && q == "" {
		out = []string{a.State.ProjectID}
	}
	return out
}

func (a *App) FilteredResources() []*gcp.GCPResource {
	var list []*gcp.GCPResource
	q := strings.ToLower(strings.TrimSpace(a.FilterQuery))
	for i := range a.State.Resources {
		r := &a.State.Resources[i]
		if a.ActiveCat == gcp.CatFinOps {
			if r.MonthlyCost <= 0 && r.WasteAmount <= 0 && r.Category != gcp.CatFinOps {
				continue
			}
		} else if r.Category != a.ActiveCat {
			continue
		}
		if q != "" {
			hay := strings.ToLower(r.Name + " " + r.Kind + " " + r.Status + " " + r.Summary + " " + r.Region)
			if !strings.Contains(hay, q) {
				continue
			}
		}
		list = append(list, r)
	}
	if a.ActiveCat == gcp.CatFinOps {
		sort.SliceStable(list, func(i, j int) bool {
			iBilling := strings.HasPrefix(list[i].ID, "finops-billing-account-")
			jBilling := strings.HasPrefix(list[j].ID, "finops-billing-account-")
			if iBilling != jBilling {
				return iBilling
			}
			if list[i].WasteAmount != list[j].WasteAmount {
				return list[i].WasteAmount > list[j].WasteAmount
			}
			return list[i].MonthlyCost > list[j].MonthlyCost
		})
	} else if a.ActiveCat == gcp.CatIAMZero {
		sort.SliceStable(list, func(i, j int) bool {
			iPosture := strings.HasPrefix(list[i].ID, "iam-security-posture-")
			jPosture := strings.HasPrefix(list[j].ID, "iam-security-posture-")
			if iPosture != jPosture {
				return iPosture
			}
			return list[i].Risk < list[j].Risk
		})
	}
	return list
}

func (a *App) CurrentResource() *gcp.GCPResource {
	list := a.FilteredResources()
	if len(list) == 0 {
		return nil
	}
	if a.SelectedIdx >= len(list) {
		a.SelectedIdx = len(list) - 1
	}
	if a.SelectedIdx < 0 {
		a.SelectedIdx = 0
	}
	return list[a.SelectedIdx]
}

func (a *App) RunInteractive() error {
	fd := int(os.Stdin.Fd())
	origState, err := enableRawMode(fd)
	if err != nil {
		fmt.Println(a.RenderFrame())
		return nil
	}
	defer func() {
		a.CloseAllPTYSessions()
		disableRawMode(fd, origState)
	}()

	fmt.Print("\x1b[?1049h\x1b[?25l\x1b[?7l\x1b[2J")
	defer fmt.Print("\x1b[?7h\x1b[?25h\x1b[?1049l")

	a.updateTermSize()

	sigCh := make(chan os.Signal, 1)
	notifySignals(sigCh)

	rawInputCh := make(chan []byte, 32)
	go func() {
		buf := make([]byte, 128)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil || n == 0 {
				return
			}
			cp := make([]byte, n)
			copy(cp, buf[:n])
			rawInputCh <- cp
		}
	}()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	a.draw()

	for {
		select {
		case sig := <-sigCh:
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				return nil
			}
			fmt.Print("\x1b[2J")
			a.updateTermSize()
			a.draw()

		case <-a.PTYNotifyCh:
			// Drain any queued notifications and redraw live SSH PTY output immediately
			for {
				select {
				case <-a.PTYNotifyCh:
				default:
					goto drawPTY
				}
			}
		drawPTY:
			if sess := a.ActivePTYSession(); sess != nil && sess.Exited && a.InlineTermActive {
				a.InlineTermActive = false
				a.TermFullscreen = false
				a.StatusBanner = fmt.Sprintf("SSH / Shell PTY session for '%s' ended (%s). Press [s] to reconnect.", sess.ResourceID, sess.ExitMsg)
			}
			a.draw()

		case <-ticker.C:
			a.StreamCounter++
			a.draw()

		case raw := <-rawInputCh:
			if a.InlineTermActive {
				a.handlePTYRawInput(raw)
				a.draw()
				continue
			}
			key := parseKey(raw)
			if a.handleKey(key) {
				return nil
			}
			a.draw()
		}
	}
}

// handlePTYRawInput forwards raw keystrokes directly into the live SSH PTY process (just like k9s shell!).
func (a *App) handlePTYRawInput(raw []byte) {
	sess := a.ActivePTYSession()
	if sess == nil || sess.Exited {
		a.InlineTermActive = false
		a.TermFullscreen = false
		return
	}

	// Single-byte control shortcuts to manage the embedded PTY pane without killing SSH:
	// - Ctrl+] (0x1d) or Esc (0x1b alone): detach focus from SSH PTY back to lazygcp navigation (session stays alive!)
	// - Ctrl+F (0x06): toggle Full-Height K9s Shell View vs Split-Pane View
	if len(raw) == 1 {
		switch raw[0] {
		case 0x1d, 0x1b: // Ctrl+] or standalone Esc
			a.InlineTermActive = false
			a.TermFullscreen = false
			a.StatusBanner = "Detached from live SSH PTY (session still running in background!). Press [s] to re-attach."
			return
		case 0x06: // Ctrl+F toggles Full-Height K9s Shell View vs Split Pane
			a.TermFullscreen = !a.TermFullscreen
			if a.TermFullscreen {
				a.StatusBanner = "Expanded Live SSH PTY to Full-Height View (Press Ctrl+F for split view, Esc or Ctrl+] to detach)."
			} else {
				a.StatusBanner = "Restored Live SSH PTY to Split-Pane View (Press Ctrl+F for full-height view, Esc or Ctrl+] to detach)."
			}
			return
		}
	}

	// Forward all other keystrokes (letters, Enter, Tab, Backspace, Arrow keys, Ctrl+C, Ctrl+D, Ctrl+L, etc.) directly to the SSH PTY!
	_ = sess.WriteRaw(raw)
}

func (a *App) draw() {
	a.updateTermSize()
	frame := a.RenderFrame()
	fmt.Print("\x1b[H" + frame)
}

func parseKey(b []byte) string {
	if len(b) == 1 {
		switch b[0] {
		case 3:
			return "ctrl+c"
		case 4:
			return "ctrl+d"
		case 6:
			return "ctrl+f"
		case 12:
			return "ctrl+l"
		case 21:
			return "ctrl+u"
		case 13, 10:
			return "enter"
		case 27:
			return "esc"
		case 9:
			return "tab"
		case 127, 8:
			return "backspace"
		default:
			return string(b[0])
		}
	}
	if len(b) >= 3 && b[0] == 27 && b[1] == '[' {
		switch b[2] {
		case 'A':
			return "up"
		case 'B':
			return "down"
		case 'C':
			return "right"
		case 'D':
			return "left"
		case '5':
			return "pgup"
		case '6':
			return "pgdn"
		}
	}
	return string(b)
}

func (a *App) handleKey(key string) bool {
	if key == "ctrl+c" {
		return true
	}

	// 1. Interactive [y/N] Safety Confirmation Modal for Live GCP Mutations
	if a.ConfirmModal {
		switch key {
		case "y", "Y", "enter":
			a.ConfirmModal = false
			a.executeConfirmedAction()
		case "n", "N", "esc", "q":
			a.ConfirmModal = false
			a.StatusBanner = "Cancelled action."
		}
		return false
	}

	// 1b. Interactive SSH Connection Mode Selector Modal (Direct SSH vs IAP Tunnel + Best-Practice IAP Setup)
	if a.ShowSSHModal {
		res := a.CurrentResource()
		if res == nil {
			a.ShowSSHModal = false
			a.SSHModalPromptIAP = false
			return false
		}
		if a.SSHModalPromptIAP {
			switch key {
			case "y", "Y", "enter":
				a.ShowSSHModal = false
				a.SSHModalPromptIAP = false
				a.startVMSSHPTY(res, "configure-iap")
			case "d", "D":
				a.ShowSSHModal = false
				a.SSHModalPromptIAP = false
				a.startVMSSHPTY(res, "direct")
			case "n", "N", "esc", "q":
				a.SSHModalPromptIAP = false
				a.StatusBanner = "Cancelled IAP auto-configuration. Choose Direct SSH [1] or press [Esc] to close."
			}
			return false
		}
		switch key {
		case "esc", "q":
			a.ShowSSHModal = false
			a.SSHModalPromptIAP = false
			a.StatusBanner = "Closed SSH connection mode selector."
		case "up", "k":
			if a.SSHModalIdx > 0 {
				a.SSHModalIdx--
			}
		case "down", "j":
			if a.SSHModalIdx < 2 {
				a.SSHModalIdx++
			}
		case "1":
			a.ShowSSHModal = false
			a.startVMSSHPTY(res, "direct")
		case "2":
			if !a.isIAPConfiguredForVM(res) {
				a.SSHModalIdx = 1
				a.SSHModalPromptIAP = true
				a.StatusBanner = "IAP TCP Forwarding is not detected for this VPC. Press [y / Enter] to configure IAP per Google Cloud Best Practices!"
			} else {
				a.ShowSSHModal = false
				a.startVMSSHPTY(res, "iap")
			}
		case "3", "c", "C":
			a.ShowSSHModal = false
			a.SSHModalPromptIAP = false
			a.startVMSSHPTY(res, "configure-iap")
		case "enter":
			switch a.SSHModalIdx {
			case 0:
				a.ShowSSHModal = false
				a.startVMSSHPTY(res, "direct")
			case 1:
				if !a.isIAPConfiguredForVM(res) {
					a.SSHModalPromptIAP = true
					a.StatusBanner = "IAP TCP Forwarding is not detected for this VPC. Press [y / Enter] to configure IAP per Google Cloud Best Practices!"
				} else {
					a.ShowSSHModal = false
					a.startVMSSHPTY(res, "iap")
				}
			case 2:
				a.ShowSSHModal = false
				a.SSHModalPromptIAP = false
				a.startVMSSHPTY(res, "configure-iap")
			}
		}
		return false
	}

	// 2. Interactive Project Selector Modal ([P])
	if a.ShowProjectModal {
		projs := a.FilteredProjectList()
		switch key {
		case "esc":
			a.ShowProjectModal = false
			a.ProjectFilter = ""
			a.StatusBanner = "Closed project selector."
		case "up", "k":
			if a.ProjectModalIdx > 0 {
				a.ProjectModalIdx--
			}
		case "down", "j":
			if a.ProjectModalIdx < len(projs)-1 {
				a.ProjectModalIdx++
			}
		case "pgup", "ctrl+u":
			a.ProjectFilter = ""
			a.ProjectModalIdx = 0
		case "enter":
			if len(projs) > 0 {
				if a.ProjectModalIdx >= len(projs) {
					a.ProjectModalIdx = len(projs) - 1
				}
				chosen := projs[a.ProjectModalIdx]
				a.ShowProjectModal = false
				a.ProjectFilter = ""
				a.switchToProject(chosen)
			} else {
				a.ShowProjectModal = false
			}
		case "backspace":
			if len(a.ProjectFilter) > 0 {
				_, size := utf8.DecodeLastRuneInString(a.ProjectFilter)
				a.ProjectFilter = a.ProjectFilter[:len(a.ProjectFilter)-size]
				a.ProjectModalIdx = 0
			}
		default:
			if len(key) == 1 && key[0] >= 32 && key[0] <= 126 {
				a.ProjectFilter += key
				a.ProjectModalIdx = 0
			}
		}
		return false
	}

	// 3. Help Manual Modal ([?])
	if a.ShowHelpModal {
		switch key {
		case "esc", "?", "q", "enter":
			a.ShowHelpModal = false
		case "tab", "right", "l", "n":
			a.HelpPage = (a.HelpPage + 1) % 3
		case "left", "h", "p":
			a.HelpPage = (a.HelpPage + 2) % 3
		case "1":
			a.HelpPage = 0
		case "2":
			a.HelpPage = 1
		case "3":
			a.HelpPage = 2
		}
		return false
	}

	if a.FilterMode {
		switch key {
		case "esc", "enter":
			a.FilterMode = false
		case "backspace":
			if len(a.FilterQuery) > 0 {
				_, size := utf8.DecodeLastRuneInString(a.FilterQuery)
				a.FilterQuery = a.FilterQuery[:len(a.FilterQuery)-size]
			}
		default:
			if len(key) == 1 && key[0] >= 32 && key[0] <= 126 {
				a.FilterQuery += key
				a.SelectedIdx = 0
				a.TableOffset = 0
			}
		}
		return false
	}

	if a.CommandMode {
		switch key {
		case "esc":
			a.CommandMode = false
			a.CommandQuery = ""
		case "enter":
			cmd := strings.TrimSpace(strings.ToLower(a.CommandQuery))
			a.CommandMode = false
			a.CommandQuery = ""
			return a.executeColonCommand(cmd)
		case "backspace":
			if len(a.CommandQuery) > 0 {
				_, size := utf8.DecodeLastRuneInString(a.CommandQuery)
				a.CommandQuery = a.CommandQuery[:len(a.CommandQuery)-size]
			}
		default:
			if len(key) == 1 && key[0] >= 32 && key[0] <= 126 {
				a.CommandQuery += key
			}
		}
		return false
	}

	switch key {
	case "q":
		return true
	case "?":
		a.ShowHelpModal = !a.ShowHelpModal
		a.HelpPage = 0
	case "/":
		a.FilterMode = true
	case ":":
		a.CommandMode = true
		a.CommandQuery = ""
	case "esc":
		a.FilterQuery = ""
		a.InspectorScroll = 0
		a.ShowFullAuditJSON = false
		a.InlineTermActive = false
		a.TermFullscreen = false
		a.StatusBanner = "Filter & scroll reset."
	case "tab":
		a.cycleModuleInspectorTab()
	case "]":
		a.InspectorScroll += 4
	case "[":
		if a.InspectorScroll >= 4 {
			a.InspectorScroll -= 4
		} else {
			a.InspectorScroll = 0
		}
	case "pgdn", "ctrl+d":
		maxItems := a.currentListCount()
		a.SelectedIdx = minInt(maxItems-1, a.SelectedIdx+8)
		a.InspectorScroll = 0
	case "pgup", "ctrl+u":
		if a.SelectedIdx >= 8 {
			a.SelectedIdx -= 8
		} else {
			a.SelectedIdx = 0
		}
		a.InspectorScroll = 0
	case "1":
		a.switchCategory(gcp.CatCloudRun)
	case "2":
		a.switchCategory(gcp.CatIAMZero)
	case "3":
		a.switchCategory(gcp.CatGKECompute)
	case "4":
		a.switchCategory(gcp.CatStorageSQL)
	case "5":
		a.switchCategory(gcp.CatSecretsEnv)
	case "6":
		a.switchCategory(gcp.CatNetworkVPC)
	case "7":
		a.switchCategory(gcp.CatVertexAI)
	case "8":
		a.switchCategory(gcp.CatFinOps)
	case "9":
		a.switchCategory(gcp.CatTimeMachine)
	case "up", "k":
		if a.SelectedIdx > 0 {
			a.SelectedIdx--
			a.InspectorScroll = 0
			if a.InspectorTab == TabInlineTerminal && isVMResourcePoweredOff(a.CurrentResource()) {
				a.InspectorTab = TabOverview
			}
		}
	case "down", "j":
		maxItems := a.currentListCount()
		if a.SelectedIdx < maxItems-1 {
			a.SelectedIdx++
			a.InspectorScroll = 0
			if a.InspectorTab == TabInlineTerminal && isVMResourcePoweredOff(a.CurrentResource()) {
				a.InspectorTab = TabOverview
			}
		}
	case "left", "h":
		if a.ActiveCat > 0 {
			a.switchCategory(a.ActiveCat - 1)
		}
	case "right", "l":
		if a.ActiveCat < gcp.CatTimeMachine {
			a.switchCategory(a.ActiveCat + 1)
		}
	case "v":
		a.InspectorTab = TabOverview
		a.InlineTermActive = false
		a.TermFullscreen = false
		a.InspectorScroll = 0
	case "y":
		if a.ActiveCat == gcp.CatTimeMachine {
			a.ShowFullAuditJSON = !a.ShowFullAuditJSON
			a.InspectorScroll = 0
		} else if a.ActiveCat == gcp.CatCloudRun {
			a.InspectorTab = TabServiceYAML
			a.InlineTermActive = false
			a.TermFullscreen = false
			a.InspectorScroll = 0
			a.StatusBanner = "Viewing Cloud Run / Function YAML Manifest. Use [ and ] to scroll."
		} else {
			a.StatusBanner = "YAML manifest view [y] is only for Cloud Run & Functions (Module [1]). Press [t] for 1:1 Terraform!"
		}
	case "z":
		if a.ActiveCat == gcp.CatIAMZero || a.ActiveCat == gcp.CatCloudRun {
			a.InspectorTab = TabIAMZero
			a.InlineTermActive = false
			a.TermFullscreen = false
			a.InspectorScroll = 0
			a.StatusBanner = "IAM & Role Blast-Radius Inspector active."
		} else {
			a.StatusBanner = "Switch to Module [2] IAM Policies or press [t] for 1:1 Terraform HCL."
		}
	case "g":
		a.InspectorTab = TabTopology
		a.InlineTermActive = false
		a.TermFullscreen = false
		a.InspectorScroll = 0
		a.StatusBanner = "Live Blast-Radius & Dependency Topology Map."
	case "t":
		a.InspectorTab = TabTerraform
		a.InlineTermActive = false
		a.TermFullscreen = false
		a.InspectorScroll = 0
		a.StatusBanner = "1:1 Reverse-Engineered Terraform HCL (mapped from live GCP state)! Press [w] to write .tf file."
	case "a":
		a.InspectorTab = TabLogsAI
		a.InlineTermActive = false
		a.TermFullscreen = false
		a.InspectorScroll = 0
		if a.State.LiveMode {
			if res := a.CurrentResource(); res != nil && len(res.Logs) == 0 {
				a.StatusBanner = fmt.Sprintf("Fetching live Cloud Logging entries for %s...", res.Name)
				a.draw()
				res.Logs = gcp.FetchLiveResourceLogs(a.State.ProjectID, res)
			}
		}
		a.StatusBanner = "Live Cloud Logging & SRE Analysis active."
	case "s", "e", "enter":
		a.openLivePTYOrInspect()
	case "S":
		a.openSSHModal()
	case "b":
		a.triggerFirewallBlock()
	case "r":
		a.triggerRollbackOrShift()
	case "R":
		if a.State.LiveMode {
			a.StatusBanner = fmt.Sprintf("Refreshing live GCP resources for project %s via HTTP/2 REST...", a.State.ProjectID)
			a.draw()
			if fresh, err := gcp.FetchLiveProjectState(a.State.ProjectID); err == nil {
				a.State = fresh
				a.StatusBanner = fmt.Sprintf("Refreshed project '%s' (%d live resources).", a.State.ProjectID, len(a.State.Resources))
			}
		}
	case "f":
		a.triggerAutoFix()
	case "p":
		a.triggerTunnelProxy()
	case "x":
		a.triggerSecretUnmask()
	case "P":
		a.openProjectSelectorModal()
	case "w":
		a.exportTerraformFile()
	}

	return false
}

// switchCategory cleanly isolates modules so VM commands/tabs never bleed into FinOps or other modules.
func (a *App) switchCategory(cat gcp.ResourceCategory) {
	a.ActiveCat = cat
	a.SelectedIdx = 0
	a.TableOffset = 0
	a.InspectorTab = TabOverview
	a.InspectorScroll = 0
	a.InlineTermActive = false
	a.TermFullscreen = false
	a.ShowFullAuditJSON = false
}

func isVMResourcePoweredOff(res *gcp.GCPResource) bool {
	if res == nil {
		return false
	}
	if strings.HasPrefix(res.ID, "gce-") || strings.Contains(res.Kind, "VM") {
		if !strings.Contains(res.Status, "POWERED ON") && res.Status != "RUNNING" {
			return true
		}
	}
	return false
}

func (a *App) cycleModuleInspectorTab() {
	a.InlineTermActive = false
	a.TermFullscreen = false
	a.InspectorScroll = 0
	res := a.CurrentResource()
	switch a.ActiveCat {
	case gcp.CatCloudRun:
		order := []InspectorSubTab{TabOverview, TabServiceYAML, TabInlineTerminal, TabTerraform, TabLogsAI}
		a.InspectorTab = nextTabInOrder(a.InspectorTab, order)
	case gcp.CatGKECompute:
		if isVMResourcePoweredOff(res) {
			order := []InspectorSubTab{TabOverview, TabTerraform, TabLogsAI}
			a.InspectorTab = nextTabInOrder(a.InspectorTab, order)
		} else {
			order := []InspectorSubTab{TabOverview, TabInlineTerminal, TabTerraform, TabLogsAI}
			a.InspectorTab = nextTabInOrder(a.InspectorTab, order)
		}
	case gcp.CatStorageSQL:
		order := []InspectorSubTab{TabOverview, TabInlineTerminal, TabTerraform, TabLogsAI}
		a.InspectorTab = nextTabInOrder(a.InspectorTab, order)
	case gcp.CatIAMZero:
		order := []InspectorSubTab{TabOverview, TabIAMZero, TabTerraform}
		a.InspectorTab = nextTabInOrder(a.InspectorTab, order)
	case gcp.CatFinOps:
		order := []InspectorSubTab{TabOverview, TabTerraform}
		a.InspectorTab = nextTabInOrder(a.InspectorTab, order)
	default:
		order := []InspectorSubTab{TabOverview, TabTerraform, TabLogsAI}
		a.InspectorTab = nextTabInOrder(a.InspectorTab, order)
	}
}

func nextTabInOrder(curr InspectorSubTab, order []InspectorSubTab) InspectorSubTab {
	for i, t := range order {
		if t == curr {
			return order[(i+1)%len(order)]
		}
	}
	return order[0]
}

func (a *App) openProjectSelectorModal() {
	a.ShowProjectModal = true
	a.ProjectFilter = ""
	a.ProjectModalIdx = 0
	for i, p := range a.State.ProjectList {
		if p == a.State.ProjectID {
			a.ProjectModalIdx = i
			break
		}
	}
	a.StatusBanner = "Select a GCP Project with ↑/↓ (or type to filter) and press [Enter] to switch."
}

func (a *App) switchToProject(targetProj string) {
	if targetProj == "" {
		return
	}
	if a.State.LiveMode {
		a.StatusBanner = fmt.Sprintf("Switching to live GCP project '%s' (HTTP/2 parallel discovery)...", targetProj)
		a.draw()
		if fresh, err := gcp.FetchLiveProjectState(targetProj); err == nil {
			if len(a.State.ProjectList) > 0 {
				fresh.ProjectList = a.State.ProjectList
			}
			a.State = fresh
			a.SelectedIdx = 0
			a.TableOffset = 0
			a.InspectorTab = TabOverview
			a.InspectorScroll = 0
			a.InlineTermActive = false
			a.TermFullscreen = false
			a.StatusBanner = fmt.Sprintf("✅ Switched live GCP Project -> %s (%d resources loaded)", targetProj, len(fresh.Resources))
		} else {
			a.StatusBanner = fmt.Sprintf("⚠️ Failed to switch to project '%s': %v", targetProj, err)
		}
	} else {
		a.State.ProjectID = targetProj
		a.StatusBanner = fmt.Sprintf("Switched active GCP Project context -> %s", a.State.ProjectID)
	}
}

func (a *App) currentListCount() int {
	if a.ActiveCat == gcp.CatTimeMachine {
		return len(a.State.AuditTimeline)
	}
	return len(a.FilteredResources())
}

func (a *App) executeColonCommand(cmd string) bool {
	switch cmd {
	case "q", "quit", "exit":
		return true
	case "help", "h", "?":
		a.ShowHelpModal = true
		a.HelpPage = 0
	case "proj", "project", "projects", "p":
		a.openProjectSelectorModal()
	case "ssh", "shell", "connect", "s", "exec", "sql", "ls", "e":
		a.openLivePTYOrInspect()
	case "iap":
		a.openSSHModal()
	case "yaml", "manifest", "y":
		if a.ActiveCat == gcp.CatCloudRun {
			a.InspectorTab = TabServiceYAML
		}
	case "tf", "terraform", "t":
		a.InspectorTab = TabTerraform
	case "block", "b":
		a.triggerFirewallBlock()
	case "run", "cloudrun", "1":
		a.switchCategory(gcp.CatCloudRun)
	case "iam", "zero", "sa", "2":
		a.switchCategory(gcp.CatIAMZero)
	case "gke", "vm", "compute", "3":
		a.switchCategory(gcp.CatGKECompute)
	case "storage", "gcs", "4":
		a.switchCategory(gcp.CatStorageSQL)
	case "sec", "secrets", "5":
		a.switchCategory(gcp.CatSecretsEnv)
	case "net", "vpc", "nat", "fw", "6":
		a.switchCategory(gcp.CatNetworkVPC)
	case "ai", "vertex", "gemini", "7":
		a.switchCategory(gcp.CatVertexAI)
	case "finops", "cost", "waste", "8":
		a.switchCategory(gcp.CatFinOps)
	case "diff", "rewind", "audit", "9":
		a.switchCategory(gcp.CatTimeMachine)
	default:
		a.StatusBanner = fmt.Sprintf("Command Executed: ':%s' -> Inspected %d live resources.", cmd, len(a.State.Resources))
	}
	return false
}

// openSSHModal opens the interactive SSH Connection Mode Selector (Direct SSH vs IAP Tunnel + Best-Practice IAP Setup) for a running VM.
func (a *App) openSSHModal() {
	res := a.CurrentResource()
	if res == nil {
		return
	}
	if !strings.HasPrefix(res.ID, "gce-") && !strings.Contains(res.Kind, "VM") {
		a.openLivePTYOrInspect()
		return
	}
	if isVMResourcePoweredOff(res) {
		a.InlineTermActive = false
		a.TermFullscreen = false
		a.InspectorTab = TabOverview
		a.StatusBanner = fmt.Sprintf("⚠️ Cannot open SSH terminal: VM '%s' is currently %s. Press [f] or [r] to Power On the VM first!", res.Name, res.Status)
		return
	}
	a.ShowSSHModal = true
	a.SSHModalPromptIAP = false
	// Default cursor to IAP Tunnel [2] if the VM has no External IP or IAP is already configured; otherwise Direct SSH [1]
	netInfo := ""
	if res.Metrics != nil {
		netInfo = res.Metrics["4. Network Exposure"]
	}
	if strings.Contains(netInfo, "No External IP") || a.isIAPConfiguredForVM(res) {
		a.SSHModalIdx = 1
	} else {
		a.SSHModalIdx = 0
	}
	a.StatusBanner = fmt.Sprintf("Choose SSH mode for '%s': [1] Direct SSH, [2] IAP Tunnel SSH, or [3] Configure IAP Best Practices.", res.Name)
}

func (a *App) vmNetworkName(res *gcp.GCPResource) string {
	if res != nil && res.Metrics != nil {
		if v, ok := res.Metrics["5. VPC & Subnetwork"]; ok && v != "" {
			parts := strings.Split(v, "/")
			net := strings.TrimSpace(parts[0])
			if net != "" {
				return net
			}
		}
	}
	return "default"
}

// isIAPConfiguredForVM checks whether the VM's VPC network has an active firewall rule allowing Google IAP TCP forwarding (35.235.240.0/20 -> port 22).
func (a *App) isIAPConfiguredForVM(res *gcp.GCPResource) bool {
	if res == nil {
		return false
	}
	vpc := a.vmNetworkName(res)
	for _, r := range a.State.Resources {
		if r.Category != gcp.CatNetworkVPC {
			continue
		}
		if strings.Contains(r.Status, "DISABLED") || strings.Contains(r.Status, "BLOCKED") {
			continue
		}
		if r.Region != "" && r.Region != vpc {
			continue
		}
		srcRanges := ""
		ports := ""
		if r.Metrics != nil {
			srcRanges = r.Metrics["4. Source CIDR Ranges"]
			ports = r.Metrics["3. Allowed Protocols"]
		}
		if (strings.Contains(srcRanges, "35.235.240.0/20") || strings.Contains(r.Summary, "35.235.240.0/20") || strings.Contains(strings.ToLower(r.Name), "iap")) &&
			(strings.Contains(ports, "22") || strings.Contains(ports, "ALL") || strings.Contains(r.Status, "22") || strings.Contains(strings.ToLower(r.Name), "ssh") || strings.Contains(strings.ToLower(r.Name), "iap")) {
			return true
		}
	}
	return false
}

// startVMSSHPTY launches an inline PTY SSH session to the selected Compute VM using Direct SSH, IAP Tunnel, or Automatic Best-Practice IAP Configuration + IAP Tunnel.
func (a *App) startVMSSHPTY(res *gcp.GCPResource, mode string) {
	if res == nil {
		return
	}
	// Close any previous PTY session for this VM when launching a newly chosen SSH mode
	if existing, ok := a.PTYSessions[res.ID]; ok && existing != nil {
		existing.Close()
		delete(a.PTYSessions, res.ID)
	}

	proj := a.State.ProjectID
	vpc := a.vmNetworkName(res)
	acct := a.State.ActiveAccount
	if acct == "" || strings.Contains(acct, " ") {
		acct = "$(gcloud config get-value account 2>/dev/null)"
	}

	var title string
	var cmdStr string

	switch mode {
	case "direct":
		title = fmt.Sprintf("ssh://%s (%s) [Direct SSH]", res.Name, res.Region)
		cmdStr = fmt.Sprintf(
			"printf '\\033[1;36m⚡ Connecting via Direct SSH to Compute VM %s (%s) in project %s...\\033[0m\\r\\n'; "+
				"printf '\\033[0;90m$ gcloud compute ssh %s --zone=%s --project=%s\\033[0m\\r\\n'; "+
				"gcloud compute ssh %s --zone=%s --project=%s",
			res.Name, res.Region, proj,
			res.Name, res.Region, proj,
			res.Name, res.Region, proj,
		)
	case "iap":
		title = fmt.Sprintf("ssh://%s (%s) [IAP Tunnel]", res.Name, res.Region)
		cmdStr = fmt.Sprintf(
			"printf '\\033[1;36m🔒 Connecting via Identity-Aware Proxy (IAP Tunnel) to Compute VM %s (%s)...\\033[0m\\r\\n'; "+
				"printf '\\033[0;90m$ gcloud compute ssh %s --zone=%s --project=%s --tunnel-through-iap\\033[0m\\r\\n'; "+
				"gcloud compute ssh %s --zone=%s --project=%s --tunnel-through-iap",
			res.Name, res.Region,
			res.Name, res.Region, proj,
			res.Name, res.Region, proj,
		)
	case "configure-iap":
		title = fmt.Sprintf("ssh://%s (%s) [IAP Setup + Tunnel]", res.Name, res.Region)
		fwRuleName := "allow-ssh-ingress-from-iap"
		if vpc != "" && vpc != "default" {
			fwRuleName = fmt.Sprintf("allow-ssh-ingress-from-iap-%s", vpc)
		}
		cmdStr = fmt.Sprintf(
			"printf '\\033[1;36m🛡️  Configuring Google Cloud IAP Best Practices for project %s (VPC: %s)...\\033[0m\\r\\n'; "+
				"printf '\\033[1;33m[Step 1/3] Enabling Identity-Aware Proxy API (iap.googleapis.com)...\\033[0m\\r\\n'; "+
				"gcloud services enable iap.googleapis.com --project=%s; "+
				"printf '\\033[1;33m[Step 2/3] Ensuring IAP TCP Forwarding Firewall Rule (%s: 35.235.240.0/20 -> tcp:22 on VPC %s)...\\033[0m\\r\\n'; "+
				"gcloud compute firewall-rules describe %s --project=%s >/dev/null 2>&1 || "+
				"gcloud compute firewall-rules create %s --network=%s --direction=INGRESS --action=ALLOW --rules=tcp:22 --source-ranges=35.235.240.0/20 --description=\"Allow SSH via Google Cloud Identity-Aware Proxy (IAP) TCP Forwarding\" --project=%s; "+
				"printf '\\033[1;33m[Step 3/3] Granting Least-Privilege IAM Role (roles/iap.tunnelResourceAccessor) to %s...\\033[0m\\r\\n'; "+
				"gcloud projects add-iam-policy-binding %s --member=\"user:%s\" --role=\"roles/iap.tunnelResourceAccessor\" --condition=None >/dev/null 2>&1 || true; "+
				"printf '\\033[1;32m✅ IAP Best-Practice Configuration complete! Launching IAP Tunnel SSH to %s...\\033[0m\\r\\n\\r\\n'; "+
				"gcloud compute ssh %s --zone=%s --project=%s --tunnel-through-iap",
			proj, vpc,
			proj,
			fwRuleName, vpc,
			fwRuleName, proj,
			fwRuleName, vpc, proj,
			acct,
			proj, acct,
			res.Name,
			res.Name, res.Region, proj,
		)
		// Record the newly created IAP firewall rule in local state so subsequent checks see IAP as configured
		a.State.Resources = append(a.State.Resources, gcp.GCPResource{
			ID:       "fw-" + fwRuleName,
			Name:     fwRuleName,
			Category: gcp.CatNetworkVPC,
			Kind:     "INGRESS (P:1000)",
			Region:   vpc,
			Status:   "ALLOW 35.235.240.0/20 -> [tcp:22]",
			Summary:  fmt.Sprintf("VPC: %s │ INGRESS ALLOW │ Sources: 35.235.240.0/20 (Google IAP) -> Ports: tcp:22", vpc),
			Risk:     gcp.RiskHealthy,
			Metrics: map[string]string{
				"1. VPC Network":        vpc,
				"2. Direction & Action": "INGRESS / ALLOW (Priority: 1000)",
				"3. Allowed Protocols":  "tcp:22",
				"4. Source CIDR Ranges": "35.235.240.0/20",
				"5. Rule State":         "ENABLED (IAP Best Practice)",
			},
		})
	}

	cols := a.Width - 36
	rows := a.Height / 2
	if cols < 60 {
		cols = 80
	}
	if rows < 10 {
		rows = 16
	}

	a.InspectorTab = TabInlineTerminal
	a.InlineTermActive = true

	sess, err := NewPTYSession(res.ID, title, cmdStr, cols, rows, a.PTYNotifyCh)
	if err != nil {
		a.InlineTermActive = false
		a.StatusBanner = fmt.Sprintf("⚠️ Failed to open SSH PTY session: %v", err)
		return
	}
	a.PTYSessions[res.ID] = sess
	a.StatusBanner = fmt.Sprintf("🟢 Live SSH PTY (%s) started for '%s'! (Ctrl+F=Full-Height, Esc/Ctrl+]=Detach, [S]=Switch SSH/IAP Mode)", mode, res.Name)
}

// openLivePTYOrInspect spawns or focuses a real, stateful POSIX PTY SSH / shell session inside lazygcp.
func (a *App) openLivePTYOrInspect() {
	if a.ActiveCat == gcp.CatTimeMachine {
		a.ShowFullAuditJSON = !a.ShowFullAuditJSON
		a.InspectorScroll = 0
		if a.ShowFullAuditJSON {
			a.StatusBanner = "Showing full raw Cloud Audit Log JSON payload in Inspector. Use [ and ] to scroll, or press [r] to revert."
		} else {
			a.StatusBanner = "Showing summarized Cloud Audit Log diff."
		}
		return
	}

	if a.ActiveCat == gcp.CatFinOps {
		a.StatusBanner = "FinOps & Cloud Billing View: Press [f] to execute cost optimization or [t] to view 1:1 Terraform HCL."
		return
	}

	res := a.CurrentResource()
	if res == nil {
		return
	}

	// If a Compute VM is not Powered On (RUNNING), do NOT activate the inline SSH terminal!
	if isVMResourcePoweredOff(res) {
		a.InlineTermActive = false
		a.TermFullscreen = false
		a.InspectorTab = TabOverview
		a.StatusBanner = fmt.Sprintf("⚠️ Cannot open SSH terminal: VM '%s' is currently %s. Press [f] or [r] to Power On the VM first!", res.Name, res.Status)
		return
	}

	// If an active PTY session already exists for this resource and hasn't exited, simply re-focus it!
	if existing, ok := a.PTYSessions[res.ID]; ok && existing != nil && !existing.Exited {
		a.InspectorTab = TabInlineTerminal
		a.InlineTermActive = true
		a.StatusBanner = fmt.Sprintf("🟢 Re-attached to live SSH/Shell terminal for '%s' (Ctrl+F=Full-Height, Esc/Ctrl+]=Detach, [S]=Change SSH/IAP Mode).", res.Name)
		return
	}

	// For Compute Engine VMs, open the SSH Mode Selector Modal (Direct SSH vs IAP Tunnel + IAP Best-Practices Setup)!
	if strings.HasPrefix(res.ID, "gce-") || strings.Contains(res.Kind, "VM") {
		a.openSSHModal()
		return
	}

	a.InspectorTab = TabInlineTerminal
	a.InlineTermActive = true

	proj := a.State.ProjectID
	var cmdStr string
	var title string

	switch {
	case strings.HasPrefix(res.ID, "gke-") || strings.Contains(res.Kind, "GKE"):
		title = fmt.Sprintf("gke://%s (%s)", res.Name, res.Region)
		cmdStr = fmt.Sprintf(
			"printf '\\033[1;36m⚡ Authenticating kubectl to GKE Cluster %s (%s)...\\033[0m\\r\\n'; "+
				"gcloud container clusters get-credentials %s --location=%s --project=%s && kubectl get nodes -o wide; "+
				"CLOUDSDK_CORE_PROJECT=%s exec ${SHELL:-/bin/zsh} -i",
			res.Name, res.Region, res.Name, res.Region, proj, proj,
		)

	case strings.HasPrefix(res.ID, "gcs-") || strings.Contains(res.Kind, "Bucket"):
		bucketURI := res.Name
		if !strings.HasPrefix(bucketURI, "gs://") {
			bucketURI = "gs://" + bucketURI
		}
		title = bucketURI
		cmdStr = fmt.Sprintf(
			"printf '\\033[1;36m⚡ Live GCS Bucket Shell (%s) — Listing objects (use gcloud storage ls/cat/cp):\\033[0m\\r\\n'; "+
				"gcloud storage ls -l %s --project=%s; "+
				"BUCKET=%q CLOUDSDK_CORE_PROJECT=%s exec ${SHELL:-/bin/zsh} -i",
			bucketURI, bucketURI, proj, bucketURI, proj,
		)

	case strings.HasPrefix(res.ID, "sql-") || strings.Contains(res.Kind, "Cloud SQL"):
		title = fmt.Sprintf("sql://%s (%s)", res.Name, res.Region)
		cmdStr = fmt.Sprintf(
			"printf '\\033[1;36m⚡ Live Cloud SQL Shell (%s) — Listing databases & users:\\033[0m\\r\\n'; "+
				"gcloud sql databases list --instance=%s --project=%s; "+
				"gcloud sql users list --instance=%s --project=%s; "+
				"printf '\\033[1;33mTip: Run `gcloud sql connect %s --user=postgres --project=%s` below for interactive psql/mysql prompt.\\033[0m\\r\\n'; "+
				"CLOUDSDK_CORE_PROJECT=%s exec ${SHELL:-/bin/zsh} -i",
			res.Name, res.Name, proj, res.Name, proj, res.Name, proj, proj,
		)

	default:
		title = fmt.Sprintf("shell://%s", res.Name)
		cmdStr = fmt.Sprintf(
			"printf '\\033[1;36m⚡ Live Cloud Shell for %s (%s) in project %s\\033[0m\\r\\n'; "+
				"SERVICE_URL=%q CLOUDSDK_CORE_PROJECT=%s exec ${SHELL:-/bin/zsh} -i",
			res.Name, res.Kind, proj, res.URL, proj,
		)
	}

	cols := a.Width - 36
	rows := a.Height / 2
	if cols < 60 {
		cols = 80
	}
	if rows < 10 {
		rows = 16
	}

	sess, err := NewPTYSession(res.ID, title, cmdStr, cols, rows, a.PTYNotifyCh)
	if err != nil {
		a.InlineTermActive = false
		a.StatusBanner = fmt.Sprintf("⚠️ Failed to open PTY session: %v", err)
		return
	}
	a.PTYSessions[res.ID] = sess
	a.StatusBanner = fmt.Sprintf("🟢 Live SSH/Shell PTY started for '%s'! (Ctrl+F=Toggle Full-Height View, Esc/Ctrl+]=Detach)", res.Name)
}

func (a *App) triggerFirewallBlock() {
	res := a.CurrentResource()
	if res == nil {
		return
	}
	if res.Category != gcp.CatNetworkVPC || res.BlockCmd == "" {
		a.StatusBanner = "Immediate Block [b] is for VPC Firewall rules (Module [6]). Switch to [6] to block/disable firewall rules."
		return
	}
	a.ConfirmModal = true
	a.ConfirmKind = "block"
	if strings.Contains(res.BlockCmd, "--no-disabled") {
		a.ConfirmTitle = fmt.Sprintf("Re-enable disabled VPC Firewall Rule '%s'?", res.Name)
	} else {
		a.ConfirmTitle = fmt.Sprintf("IMMEDIATELY BLOCK (Disable) VPC Firewall Rule '%s'?", res.Name)
	}
	a.ConfirmCommand = res.BlockCmd
}

func (a *App) triggerRollbackOrShift() {
	if a.ActiveCat == gcp.CatTimeMachine {
		if a.SelectedIdx < len(a.State.AuditTimeline) {
			ev := a.State.AuditTimeline[a.SelectedIdx]
			a.ConfirmModal = true
			a.ConfirmKind = "exec"
			a.ConfirmTitle = fmt.Sprintf("Execute Undo / Revert Action for Audit Event '%s' (%s)?", ev.Action, ev.ID)
			a.ConfirmCommand = ev.Rollback
		}
		return
	}
	res := a.CurrentResource()
	if res == nil {
		return
	}
	if a.State.LiveMode {
		a.ConfirmModal = true
		a.ConfirmKind = "exec"
		a.ConfirmTitle = fmt.Sprintf("Execute live gcloud action on '%s'?", res.Name)
		a.ConfirmCommand = res.GcloudCmd
		return
	}
}

func (a *App) triggerAutoFix() {
	res := a.CurrentResource()
	if res == nil {
		return
	}
	if a.State.LiveMode {
		a.ConfirmModal = true
		a.ConfirmKind = "exec"
		a.ConfirmTitle = fmt.Sprintf("Execute live remediation / optimization command on '%s'?", res.Name)
		a.ConfirmCommand = res.GcloudCmd
		return
	}
}

func (a *App) triggerTunnelProxy() {
	res := a.CurrentResource()
	if res == nil {
		return
	}
	proj := a.State.ProjectID
	if a.State.LiveMode {
		var tunCmd string
		switch {
		case strings.HasPrefix(res.ID, "gce-") || strings.Contains(res.Kind, "VM"):
			tunCmd = fmt.Sprintf("gcloud compute start-iap-tunnel %s 22 --local-host-port=localhost:8443 --zone=%s --project=%s", res.Name, res.Region, proj)
		case strings.HasPrefix(res.ID, "sql-") || strings.Contains(res.Kind, "Cloud SQL"):
			tunCmd = fmt.Sprintf("cloud-sql-proxy %s:%s:%s --port=5432 --auto-iam-authn", proj, res.Region, res.Name)
		case strings.HasPrefix(res.ID, "run-") || strings.Contains(res.Kind, "CloudRun"):
			tunCmd = fmt.Sprintf("gcloud run services proxy %s --port=8080 --region=%s --project=%s", res.Name, res.Region, proj)
		default:
			a.StatusBanner = fmt.Sprintf("Press [s] to open the Live SSH/Shell PTY for '%s'.", res.Name)
			return
		}
		a.ConfirmModal = true
		a.ConfirmKind = "tunnel"
		a.ConfirmTitle = fmt.Sprintf("Start background localhost port-forward proxy for '%s'?", res.Name)
		a.ConfirmCommand = tunCmd
		return
	}
}

func (a *App) triggerSecretUnmask() {
	res := a.CurrentResource()
	if res != nil && a.State.LiveMode && res.Category == gcp.CatSecretsEnv {
		a.ConfirmModal = true
		a.ConfirmKind = "secret"
		a.ConfirmTitle = fmt.Sprintf("Fetch & unmask live Secret Manager payload for '%s'?", res.Name)
		a.ConfirmCommand = res.GcloudCmd
		return
	}
	a.UnmaskSecrets = !a.UnmaskSecrets
	if a.UnmaskSecrets {
		a.StatusBanner = "UNMASKED: Secret Manager & Env values shown temporarily."
	} else {
		a.StatusBanner = "MASKED: Secret Manager & Env values hidden."
	}
}

func (a *App) executeConfirmedAction() {
	res := a.CurrentResource()
	switch a.ConfirmKind {
	case "secret":
		if res != nil {
			a.StatusBanner = fmt.Sprintf("Fetching latest secret version for '%s'...", res.Name)
			a.draw()
			val, err := gcp.FetchLiveSecretPayload(a.State.ProjectID, res.Name)
			if err != nil {
				a.StatusBanner = fmt.Sprintf("⚠️ Secret fetch failed: %v", err)
			} else {
				res.Metrics["4. Live Payload"] = val + " (UNMASKED LIVE)"
				a.UnmaskSecrets = true
				a.StatusBanner = fmt.Sprintf("🔓 Fetched live payload for secret '%s'!", res.Name)
			}
		}
	case "tunnel":
		if res != nil {
			pid, err := gcp.StartBackgroundTunnelProcess(a.ConfirmCommand)
			if err != nil {
				a.StatusBanner = fmt.Sprintf("⚠️ Tunnel start error: %v", err)
			} else {
				tun := fmt.Sprintf("PID %d (%s)", pid, a.ConfirmCommand)
				res.LocalTunnel = tun
				a.State.ActiveTunnels[res.Name] = fmt.Sprintf("PID %d", pid)
				a.StatusBanner = fmt.Sprintf("🔌 Started background tunnel PID %d for '%s'!", pid, res.Name)
			}
		}
	case "block":
		a.StatusBanner = fmt.Sprintf("Executing Firewall Block/Unblock: %s ...", a.ConfirmCommand)
		a.draw()
		lines, err := gcp.ExecuteLiveShellCommandMultiLine(a.ConfirmCommand)
		if res != nil {
			res.LastExecTitle = "Firewall Block/Unblock Result"
			res.LastExecOutput = lines
			if err == nil {
				if strings.Contains(a.ConfirmCommand, "--no-disabled") {
					res.Status = "ACTIVE (RE-ENABLED)"
					res.BlockCmd = fmt.Sprintf("gcloud compute firewall-rules update %s --disabled --project=%s", res.Name, a.State.ProjectID)
				} else {
					res.Status = "BLOCKED / DISABLED"
					res.Risk = gcp.RiskHealthy
					res.BlockCmd = fmt.Sprintf("gcloud compute firewall-rules update %s --no-disabled --project=%s", res.Name, a.State.ProjectID)
				}
			}
		}
		if err != nil {
			a.StatusBanner = fmt.Sprintf("⚠️ Firewall update error: %v", err)
		} else {
			a.StatusBanner = fmt.Sprintf("🛡️ Firewall rule '%s' updated! Status is now %s.", res.Name, res.Status)
		}
	default:
		a.StatusBanner = fmt.Sprintf("Executing: %s ...", a.ConfirmCommand)
		a.draw()
		lines, err := gcp.ExecuteLiveShellCommandMultiLine(a.ConfirmCommand)
		if a.ActiveCat == gcp.CatTimeMachine && a.SelectedIdx < len(a.State.AuditTimeline) {
			a.State.AuditTimeline[a.SelectedIdx].FullJSON = lines
			a.ShowFullAuditJSON = true
			a.InspectorScroll = 0
		}
		if err != nil {
			a.StatusBanner = fmt.Sprintf("⚠️ Command error: %v", err)
		} else {
			a.StatusBanner = fmt.Sprintf("✅ Executed live action (%d lines output).", len(lines))
		}
	}
}

func (a *App) exportTerraformFile() {
	res := a.CurrentResource()
	if res == nil {
		return
	}
	filename := fmt.Sprintf("%s.tf", strings.ReplaceAll(strings.TrimPrefix(res.Name, "gs://"), "/", "-"))
	_ = os.WriteFile(filename, []byte(res.TerraformHCL+"\n"), 0644)
	a.StatusBanner = fmt.Sprintf("💾 Exported 1:1 Terraform HCL to ./%s!", filename)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
