package gcp

import (
	"bytes"
	"os/exec"
	"strings"
	"time"
)

// ResourceCategory represents the 9 core GCP operational modules in lazygcp.
type ResourceCategory int

const (
	CatCloudRun ResourceCategory = iota
	CatIAMZero
	CatGKECompute
	CatStorageSQL
	CatSecretsEnv
	CatNetworkVPC
	CatVertexAI
	CatFinOps
	CatTimeMachine
)

type CategoryMeta struct {
	ID       ResourceCategory
	Key      string
	Alias    string
	Title    string
	Icon     string
	Subtitle string
}

var Categories = []CategoryMeta{
	{CatCloudRun, "1", "run", "Cloud Run & Functions", "🚀", "Services, Functions, Env Vars, Live YAML [y] & Shell [s]"},
	{CatIAMZero, "2", "iam", "IAM Policies & Roles", "◆", "Who Has Access, Role Capabilities, Blast-Radius & Fix [f]"},
	{CatGKECompute, "3", "gke", "GKE & Compute VMs", "⎈", "Clusters, VMs, Direct SSH [s] & Remote Command Exec [e]"},
	{CatStorageSQL, "4", "sql", "Cloud SQL & Storage", "▤", "Run SQL Queries [e], List/Read Bucket Files [e] & Connect [s]"},
	{CatSecretsEnv, "5", "sec", "Secrets & KMS Vault", "🔑", "Secret Manager Versions, Rotation Policy & Live Unmask [x]"},
	{CatNetworkVPC, "6", "net", "VPC, NAT & Firewalls", "🌐", "Firewall Rules, Immediate Block [b], IAP Lock [f] & VPCs"},
	{CatVertexAI, "7", "ai", "Vertex AI & GenAI", "🤖", "Enabled AI APIs, Gemini Token Burn & Model Armor"},
	{CatFinOps, "8", "cost", "FinOps & Cost Guide", "💸", "Monthly Bill Breakdown, Why It Costs & 1-Key Optimization [f]"},
	{CatTimeMachine, "9", "diff", "Audit Time-Machine", "⏪", "Who Changed What, Full Audit JSON [e] & Revert Action [r]"},
}

type RiskLevel string

const (
	RiskCritical RiskLevel = "CRITICAL"
	RiskHigh     RiskLevel = "HIGH"
	RiskMedium   RiskLevel = "MEDIUM"
	RiskLow      RiskLevel = "LOW"
	RiskHealthy  RiskLevel = "HEALTHY"
)

type TrafficSplit struct {
	Revision string
	Percent  int
	Status   string
	Created  string
	Image    string
}

type EnvBinding struct {
	Name      string
	Value     string
	IsSecret  bool
	SecretRef string
	IsExposed bool
}

type IAMBindingInfo struct {
	Principal         string
	PrincipalType     string
	Role              string
	RoleDescription   string
	GrantedPermsCount int
	UsedPermsCount    int
	ActuallyUsedPerms []string
	UnusedRiskPerms   []string
	LastUsed          string
	SourceCodeFiles   []string
	Risk              RiskLevel
}

type LogEntry struct {
	Timestamp string
	Severity  string // INFO, WARN, ERROR, CRITICAL
	Component string
	Message   string
	TraceID   string
}

type AuditDiffEvent struct {
	ID         string
	TimeAgo    string
	Timestamp  string
	Actor      string
	ActorType  string
	CallerIP   string
	Action     string
	Resource   string
	Risk       RiskLevel
	Summary    string
	DiffLines  []string
	InspectCmd string
	Rollback   string
	FullJSON   []string
}

type GCPResource struct {
	ID             string
	Name           string
	Category       ResourceCategory
	Kind           string
	Region         string
	Status         string
	Risk           RiskLevel
	MonthlyCost    float64
	WasteAmount    float64
	Summary        string
	URL            string
	ServiceAcct    string
	Metrics        map[string]string
	Revisions      []TrafficSplit
	EnvVars        []EnvBinding
	IAMInfo        *IAMBindingInfo
	TopologyASCII  []string
	TerraformHCL   string
	ManifestYAML   string
	GcloudCmd      string
	BlockCmd       string
	Logs           []LogEntry
	AIInsight      string
	LocalTunnel    string
	LastExecTitle  string
	LastExecOutput []string
}

type ProjectState struct {
	ProjectID          string
	ProjectList        []string
	Region             string
	ActiveAccount      string
	LiveMode           bool
	BillingAccountID   string
	BillingAccountName string
	BillingEnabled     bool
	BillingCurrency    string
	TotalMonthly       float64
	Reclaimable        float64
	SecurityScore      int
	Grade              string
	ActiveTunnels      map[string]string
	Resources          []GCPResource
	AuditTimeline      []AuditDiffEvent
	LastSync           time.Time
}

// TryDetectGcloudProject checks if gcloud has an active project on the machine.
func TryDetectGcloudProject() (project string, account string) {
	ctxProj := runQuickCmd("gcloud", "config", "get-value", "project")
	ctxAcct := runQuickCmd("gcloud", "config", "get-value", "account")
	if ctxProj == "" || ctxProj == "(unset)" {
		ctxProj = "acme-fintech-prod-01"
	}
	if ctxAcct == "" || ctxAcct == "(unset)" {
		ctxAcct = "sre-lead@acme-cloud.io"
	}
	return ctxProj, ctxAcct
}

func runQuickCmd(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out.String())
}
