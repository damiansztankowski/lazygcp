package main

import (
	"strings"
	"testing"

	"github.com/damiansztankowski/lazygcp/internal/gcp"
	"github.com/damiansztankowski/lazygcp/internal/tui"
)

func TestBuildMockStateAndTUI(t *testing.T) {
	proj := gcp.BuildMockState("acme-fintech-prod-01", "sre@acme.io")
	if proj == nil {
		t.Fatal("expected non-nil project state")
	}
	if proj.ProjectID != "acme-fintech-prod-01" {
		t.Errorf("expected project ID acme-fintech-prod-01, got %s", proj.ProjectID)
	}
	if len(proj.Resources) == 0 {
		t.Error("expected demo project to contain resources across modules 1-9")
	}
	if !proj.BillingEnabled || proj.BillingAccountID == "" {
		t.Error("expected demo project to include active Cloud Billing telemetry")
	}

	var sawPoweredOnVM, sawPoweredOffVM, sawBillingCard, sawIAMResource bool
	for _, r := range proj.Resources {
		if r.Category == gcp.CatGKECompute && strings.Contains(r.Kind, "Compute") {
			if strings.Contains(r.Status, "POWERED ON") {
				sawPoweredOnVM = true
			}
			if strings.Contains(r.Status, "POWERED OFF") {
				sawPoweredOffVM = true
			}
		}
		if r.Category == gcp.CatFinOps && strings.Contains(r.Kind, "Cloud Billing") {
			sawBillingCard = true
		}
		if r.Category == gcp.CatIAMZero && r.IAMInfo != nil {
			sawIAMResource = true
		}
	}
	if !sawPoweredOnVM || !sawPoweredOffVM {
		t.Errorf("expected both POWERED ON and POWERED OFF VMs in demo state (got on=%v, off=%v)", sawPoweredOnVM, sawPoweredOffVM)
	}
	if !sawBillingCard {
		t.Error("expected Cloud Billing overview row in Module 8 (CatFinOps)")
	}
	if !sawIAMResource {
		t.Error("expected live IAM policy & security assessment resources in Module 2 (CatIAMZero)")
	}

	app := tui.NewApp(proj)
	frame := app.RenderFrame()
	if !strings.Contains(frame, "acme-fintech-prod-01") {
		t.Error("expected rendered TUI frame to display project ID")
	}
	if !strings.Contains(frame, "[a] Cloud Logs") {
		t.Error("expected inspector tab bar to display '[a] Cloud Logs'")
	}
	if strings.Contains(frame, "Logs & AI") {
		t.Error("did not expect 'Logs & AI' in inspector tab bar")
	}

	// Verify SSH Connection Mode Selector Modal & IAP Best-Practices Prompt on Module 3 (Compute VM)
	app.ActiveCat = gcp.CatGKECompute
	app.ShowSSHModal = true
	sshFrame := app.RenderFrame()
	if !strings.Contains(sshFrame, "Direct SSH") || !strings.Contains(sshFrame, "IAP Tunnel SSH") {
		t.Error("expected SSH Mode Selector Modal to offer both Direct SSH and IAP Tunnel SSH")
	}
	app.SSHModalPromptIAP = true
	iapPromptFrame := app.RenderFrame()
	if !strings.Contains(iapPromptFrame, "35.235.240.0/20") || !strings.Contains(iapPromptFrame, "iap.tunnelResourceAccessor") {
		t.Error("expected IAP Best-Practices Configuration prompt to show 35.235.240.0/20 firewall rule and roles/iap.tunnelResourceAccessor")
	}
}
