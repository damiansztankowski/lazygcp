package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/damiansztankowski/lazygcp/internal/gcp"
	"github.com/damiansztankowski/lazygcp/internal/tui"
)

const version = "0.4.0"

func main() {
	args := os.Args[1:]

	if hasFlag(args, "--version") || hasFlag(args, "-v") || (len(args) > 0 && args[0] == "version") {
		fmt.Printf("lazygcp v%s — Live Terminal UI (TUI), Inline SSH Cockpit, Security/IAM Auditor & FinOps Engine for Google Cloud\n", version)
		return
	}
	if hasFlag(args, "--help") || hasFlag(args, "-h") || (len(args) > 0 && args[0] == "help") {
		printExtendedHelp()
		return
	}

	overrideProject := getFlagValue(args, "--project", "-p")
	useDemo := hasFlag(args, "--demo")
	isSnapshot := hasFlag(args, "--snapshot")

	var (
		state *gcp.ProjectState
		err   error
	)

	if isSnapshot {
		if useDemo {
			proj, acct := gcp.TryDetectGcloudProject()
			state = gcp.BuildMockState(proj, acct)
		} else {
			state, err = gcp.FetchLiveProjectState(overrideProject)
			if err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  Live GCP connection warning: %v\n", err)
			}
		}
	} else {
		state, err = tui.BootWithAnimation(overrideProject, useDemo)
		if err != nil && state == nil {
			fmt.Print("\x1b[?25h\x1b[?1049l")
			fmt.Fprintf(os.Stderr, "⚠️  Live GCP connection warning: %v\n", err)
			return
		}
	}

	app := tui.NewApp(state)

	if !useDemo && state != nil {
		app.StatusBanner = fmt.Sprintf("LIVE GCP REST API: Project '%s' (%d resources). [1-9] Modules, [s] Live SSH, [P] Project, [t] 1:1 Terraform, [?] Help.",
			state.ProjectID, len(state.Resources))
	}

	if len(args) > 0 {
		selectCategoryByArg(app, args[0])
	}

	if hasFlag(args, "--snapshot") {
		for i, a := range args {
			if a == "--snapshot" && i+1 < len(args) {
				selectCategoryByArg(app, args[i+1])
			}
		}
		fmt.Println(app.RenderFrame())
		return
	}

	_ = app.RunInteractive()
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func getFlagValue(args []string, longFlag, shortFlag string) string {
	for i, a := range args {
		if (a == longFlag || a == shortFlag) && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, longFlag+"=") {
			return strings.TrimPrefix(a, longFlag+"=")
		}
		if strings.HasPrefix(a, shortFlag+"=") {
			return strings.TrimPrefix(a, shortFlag+"=")
		}
	}
	return ""
}

func selectCategoryByArg(app *tui.App, arg string) {
	switch strings.ToLower(arg) {
	case "run", "cloudrun", "1":
		app.ActiveCat = gcp.CatCloudRun
	case "iam", "security", "zero", "iam-zero", "2":
		app.ActiveCat = gcp.CatIAMZero
		app.InspectorTab = tui.TabOverview
	case "gke", "compute", "vm", "3":
		app.ActiveCat = gcp.CatGKECompute
	case "sql", "gcs", "storage", "4":
		app.ActiveCat = gcp.CatStorageSQL
	case "sec", "secrets", "5":
		app.ActiveCat = gcp.CatSecretsEnv
	case "net", "vpc", "6":
		app.ActiveCat = gcp.CatNetworkVPC
	case "ai", "vertex", "7":
		app.ActiveCat = gcp.CatVertexAI
	case "finops", "cost", "waste", "8":
		app.ActiveCat = gcp.CatFinOps
	case "rewind", "diff", "audit", "9":
		app.ActiveCat = gcp.CatTimeMachine
	}
}

func printExtendedHelp() {
	c := tui.FgCyan + tui.Bold
	y := tui.FgYellow + tui.Bold
	g := tui.FgGreen + tui.Bold
	gr := tui.FgGray
	r := tui.Reset

	fmt.Printf(`%s╭──────────────────────────────────────────────────────────────────────────────────────────────╮%s
%s│ ⚡ lazygcp v%s — Live Terminal Cockpit, Inline SSH, IAM Auditor & FinOps for Google Cloud  │%s
%s╰──────────────────────────────────────────────────────────────────────────────────────────────╯%s

%sOVERVIEW%s
  %slazygcp%s is a blazing-fast, zero-dependency Terminal UI (TUI) for Google Cloud Platform
  (inspired by %sk9s%s and %slazydocker%s). It connects directly to your active %sgcloud%s project
  and concurrently audits Cloud Run, Project IAM Policies & Service Accounts, GKE, Compute VMs,
  Cloud SQL, GCS Buckets, Secret Manager, VPC Firewalls, Vertex AI, Cloud Billing, and Audit Logs.

%sSYNOPSIS%s
  %slazygcp%s [MODULE] [FLAGS]

%sCORE COMMANDS & DIRECT MODULE LAUNCHERS%s
  %slazygcp%s                     Launch interactive TUI connected to your active gcloud project
  %slazygcp run%s                 Boot directly into Module [1] Cloud Run & Functions
  %slazygcp iam%s                 Boot directly into Module [2] Live Project IAM & Security Assessment
  %slazygcp gke%s                 Boot directly into Module [3] GKE Clusters & Compute VMs (Inline SSH)
  %slazygcp sql%s                 Boot directly into Module [4] Cloud SQL & GCS Storage Buckets
  %slazygcp sec%s                 Boot directly into Module [5] Secret Manager & KMS Vault
  %slazygcp net%s                 Boot directly into Module [6] VPC Networks, NAT & Firewalls
  %slazygcp ai%s                  Boot directly into Module [7] Vertex AI & Enabled GenAI APIs
  %slazygcp cost%s                Boot directly into Module [8] Cloud Billing & FinOps Waste Radar
  %slazygcp rewind%s              Boot directly into Module [9] Cloud Audit Time-Machine (git-diff view)

%sFLAGS%s
  %s-p, --project <PROJECT_ID>%s  Override the active gcloud project and query <PROJECT_ID>
  %s--demo%s                      Launch in offline simulation mode with realistic production incidents
                              (great for live stage demos, workshops, or testing without credentials)
  %s--snapshot [MODULE]%s         Render a single non-interactive ANSI dashboard frame to stdout and exit
  %s-h, --help%s                  Show this extended reference manual with examples
  %s-v, --version%s               Print lazygcp version

%sTHE 9 INTERACTIVE COCKPIT MODULES (KEYS 1-9 OR :COMMAND)%s
  %s[1] :run%s    Cloud Run & Functions   Revisions, traffic splits, YAML [y], shell [s] & secret scan
  %s[2] :iam%s    IAM Policies & Roles    Live project IAM bindings, Service Accounts, blast-radius & fix [f]
  %s[3] :gke%s    GKE & Compute VMs       Explicit VM power states, inline PTY SSH [s] & GKE telemetry
  %s[4] :sql%s    Cloud SQL & Storage     Inline SQL query prompt [e], GCS file browser [e] & SQL Proxy [p]
  %s[5] :sec%s    Secrets & KMS Vault     Secret Manager versions, creation timestamps & live unmask [x]
  %s[6] :net%s    VPC, NAT & Firewalls    VPC firewall rules, 1-key emergency block [b] & 0.0.0.0/0 checks
  %s[7] :ai%s     Vertex AI & GenAI       Enabled Vertex AI / Gemini APIs, token burn & Model Armor checks
  %s[8] :cost%s   FinOps & Cloud Billing  Cloud Billing API v1 burn rate, service spend bars & idle waste
  %s[9] :diff%s   Audit Time-Machine      Live cloudaudit.googleapis.com stream formatted as git diffs

%s1-KEYSTROKE ENGINEER ACTIONS (INSIDE TUI)%s
  %sP (Shift+p)%s   Open Interactive GCP Project Switcher Modal (search & switch projects live)
  %sR (Shift+r)%s   Force immediate live refresh from Google Cloud HTTP/2 REST APIs
  %ss / Enter%s     Open Embedded Inline POSIX PTY SSH / Shell Terminal (running VMs, GKE, SQL, GCS)
  %sCtrl+F / Esc%s  Toggle Full-Height SSH Terminal view / Detach back to table navigation
  %sb%s             Immediately Block / Quarantine an exposed VPC Firewall Rule (Module 6)
  %sr%s             Cloud Run traffic rollback / Audit diff rollback (opens [y/N] Safety Modal)
  %sf%s             Apply IAM Least-Privilege & FinOps remediation (opens [y/N] Safety Modal)
  %st / w%s         View 1:1 Reverse-Engineered Terraform HCL / Write .tf file to disk
  %sx%s             Fetch & unmask live Secret Manager payload (prompts [y/N] before API call)
  %s?%s             Open interactive 3-page Extended Help Modal inside the TUI

%sPRACTICAL EXAMPLES%s
  %s# 1. Inspect your live active GCP project in the interactive terminal cockpit%s
  $ lazygcp

  %s# 2. Assess live IAM Policies, Roles & Security Posture on a specific GCP project%s
  $ lazygcp iam -p my-production-gcp-project

  %s# 3. Inspect GKE & Compute VMs and open an inline SSH session%s
  $ lazygcp gke -p my-production-gcp-project

  %s# 4. Inspect recent Cloud Audit Log changes (who changed what in the last few hours)%s
  $ lazygcp rewind -p my-production-gcp-project

  %s# 5. Launch offline demo mode%s
  $ lazygcp --demo
`,
		c, r, c, version, r, c, r,
		y, r, c, r, g, r, g, r, c, r,
		y, r, c, r,
		y, r, g, r, g, r, g, r, g, r, g, r, g, r, g, r, g, r, g, r, g, r,
		y, r, c, r, c, r, c, r, c, r, c, r,
		y, r, c, r, c, r, c, r, c, r, c, r, c, r, c, r, c, r, c, r,
		y, r, y, r, y, r, g, r, g, r, c, r, g, r, g, r, c, r, y, r, y, r,
		y, r, gr, r, gr, r, gr, r, gr, r, gr, r)
}
