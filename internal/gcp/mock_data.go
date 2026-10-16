package gcp

import "time"

// BuildMockState constructs a rich, realistic production GCP state for instant demo/sandbox exploration.
func BuildMockState(detectedProject, detectedAccount string) *ProjectState {
	if detectedProject == "" {
		detectedProject = "acme-fintech-prod-01"
	}
	if detectedAccount == "" {
		detectedAccount = "damian@gde-cloud.dev"
	}

	resources := []GCPResource{
		// 1. CLOUD RUN & FUNCTIONS
		{
			ID:          "run-checkout-api",
			Name:        "checkout-payment-api",
			Category:    CatCloudRun,
			Kind:        "CloudRun v2",
			Region:      "europe-west1",
			Status:      "DEGRADED (503s)",
			Risk:        RiskCritical,
			MonthlyCost: 1420.00,
			WasteAmount: 890.00,
			Summary:     "High 503 rate after rev-0042 deploy • DB connection pool exhaustion + over-privileged default SA",
			URL:         "https://checkout-payment-api-ew1a.a.run.app",
			ServiceAcct: "104928192-compute@developer.gserviceaccount.com",
			Metrics: map[string]string{
				"RPS":          "1,480 req/s",
				"Error Rate":   "14.8% (503/500)",
				"p99 Latency":  "2,840 ms (SLA Breach)",
				"Concurrency":  "80 (Max Instances: 250)",
				"Min/Max Inst": "15 min / 250 max (CPU always allocated)",
				"Ingress":      "all (Unprotected - Missing Cloud Armor)",
			},
			Revisions: []TrafficSplit{
				{Revision: "checkout-payment-api-00042-bad", Percent: 80, Status: "DEGRADED (503 Pool Exhausted)", Created: "18m ago", Image: "ew1-docker.pkg.dev/acme/pay/api:v2.4.1"},
				{Revision: "checkout-payment-api-00041-ok", Percent: 20, Status: "HEALTHY", Created: "3d ago", Image: "ew1-docker.pkg.dev/acme/pay/api:v2.4.0"},
				{Revision: "checkout-payment-api-00040-ok", Percent: 0, Status: "RETIRING", Created: "9d ago", Image: "ew1-docker.pkg.dev/acme/pay/api:v2.3.9"},
			},
			EnvVars: []EnvBinding{
				{Name: "DB_HOST", Value: "10.24.0.15:5432", IsSecret: false},
				{Name: "DB_MAX_CONNS", Value: "50 (x250 instances = 12,500 conns!)", IsSecret: false},
				{Name: "STRIPE_LIVE_SECRET_KEY", Value: "sk_live_51Nx92J...89aK", IsSecret: false, IsExposed: true},
				{Name: "JWT_SIGNING_SECRET", Value: "projects/acme/secrets/jwt-key/versions/3", IsSecret: true, SecretRef: "jwt-key:v3"},
			},
			IAMInfo: &IAMBindingInfo{
				Principal:         "104928192-compute@developer.gserviceaccount.com",
				Role:              "roles/editor (Default Compute SA)",
				GrantedPermsCount: 6842,
				UsedPermsCount:    4,
				ActuallyUsedPerms: []string{
					"cloudsql.instances.connect",
					"secretmanager.versions.access",
					"pubsub.topics.publish",
					"logging.logEntries.create",
				},
				UnusedRiskPerms: []string{
					"storage.objects.delete (*all buckets*)",
					"iam.serviceAccountKeys.create",
					"compute.instances.delete",
					"bigquery.tables.export",
				},
				LastUsed:        "2 seconds ago",
				SourceCodeFiles: []string{"src/db/pool.go:42", "src/secrets/loader.go:19", "src/events/pubsub.go:88"},
				Risk:            RiskCritical,
			},
			TopologyASCII: []string{
				"  [🌍 Public Internet (0.0.0.0/0)]",
				"          │  ⚠️ No Cloud Armor WAF / Rate Limit",
				"          ▼",
				"  [🚀 Cloud Run: checkout-payment-api]",
				"     ├── 80% ──► rev-00042-bad (🔴 14.8% 503 Errors)",
				"     ├── 20% ──► rev-00041-ok  (🟢 Healthy)",
				"     ├── 🔑 Identity: Default Compute SA (🔴 roles/editor - 6,842 perms!)",
				"     ├── ⚠️ Env Leak: Plaintext STRIPE_LIVE_SECRET_KEY",
				"     └── 🔌 VPC Connector (e2-micro Saturated 99% CPU)",
				"          │",
				"          ▼",
				"  [🗄️ Cloud SQL: prod-orders-pg16] (🔴 500/500 Max Connections Reached!)",
			},
			TerraformHCL: `# 🛡️ lazygcp Hardened & Least-Privilege Terraform Export
resource "google_service_account" "checkout_sa" {
  account_id   = "checkout-payment-sa"
  display_name = "Least-Privilege Checkout Service Account"
}

resource "google_project_iam_custom_role" "checkout_minimal_role" {
  role_id     = "checkoutMinimalRunner"
  title       = "IAM-Zero Synthesized Role (4/6842 permissions)"
  permissions = [
    "cloudsql.instances.connect",
    "secretmanager.versions.access",
    "pubsub.topics.publish",
    "logging.logEntries.create",
  ]
}

resource "google_cloud_run_v2_service" "checkout_payment_api" {
  name     = "checkout-payment-api"
  location = "europe-west1"
  ingress  = "INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER"

  template {
    service_account = google_service_account.checkout_sa.email
    scaling {
      min_instance_count = 2
      max_instance_count = 40
    }
    containers {
      image = "ew1-docker.pkg.dev/acme/pay/api:v2.4.0"
      env {
        name  = "DB_MAX_CONNS"
        value = "5"
      }
      env {
        name = "STRIPE_LIVE_SECRET_KEY"
        value_source {
          secret_key_ref {
            secret  = "stripe-live-key"
            version = "latest"
          }
        }
      }
    }
  }
}`,
			GcloudCmd: "gcloud run services update-traffic checkout-payment-api --to-revisions=checkout-payment-api-00041-ok=100 --region=europe-west1",
			Logs: []LogEntry{
				{Timestamp: "17:09:51.102", Severity: "ERROR", Component: "rev-00042", Message: "FATAL: remaining connection slots are reserved for non-replication superuser connections (SQLSTATE 53300)", TraceID: "tr-99182a"},
				{Timestamp: "17:09:52.410", Severity: "WARN", Component: "vpc-conn", Message: "Serverless VPC Access connector 'ew1-vpc-conn' bandwidth throttled at 100Mbps", TraceID: "tr-99183b"},
				{Timestamp: "17:09:54.008", Severity: "CRITICAL", Component: "run-http", Message: "POST /v1/checkout/charge 503 Service Unavailable (2844ms) - upstream timeout", TraceID: "tr-99184c"},
				{Timestamp: "17:09:55.812", Severity: "INFO", Component: "rev-00041", Message: "POST /v1/checkout/charge 200 OK (41ms)", TraceID: "tr-99185d"},
			},
			AIInsight: "🤖 Gemini SRE Root-Cause Analysis:\n1. Revision 00042 raised DB_MAX_CONNS to 50 across 250 max instances (12,500 potential connections vs Cloud SQL max_connections=500). Press [r] to instant-rollback 100% traffic to rev-00041-ok.\n2. Plaintext STRIPE_LIVE_SECRET_KEY detected in container env. Press [t] to export Secret Manager HCL patch.\n3. Service uses Default Compute SA with roles/editor (6,842 perms, only 4 used). Press [z] for IAM-Zero surgical lockdown.",
		},
		{
			ID:          "run-webhook-worker",
			Name:        "pubsub-webhook-ingestor",
			Category:    CatCloudRun,
			Kind:        "CloudRun v2",
			Region:      "europe-west1",
			Status:      "RETRY STORM ($)",
			Risk:        RiskHigh,
			MonthlyCost: 2340.00,
			WasteAmount: 1980.00,
			Summary:     "Infinite Pub/Sub NACK retry loop without Dead-Letter Topic • Burning $78/day in wasted CPU",
			URL:         "https://pubsub-webhook-ingestor-ew1a.a.run.app",
			ServiceAcct: "webhook-worker@acme-fintech-prod-01.iam.gserviceaccount.com",
			Metrics: map[string]string{
				"RPS":         "3,100 req/s (94% Retries!)",
				"Error Rate":  "94.2% (400 Poison Pill Payload)",
				"Dead Letter": "NONE CONFIGURED",
				"CPU Alloc":   "4 vCPU / 8GiB (CPU Utilization: 6%)",
			},
			Revisions: []TrafficSplit{
				{Revision: "pubsub-webhook-ingestor-00019", Percent: 100, Status: "RETRY_STORM", Created: "2d ago", Image: "ew1-docker.pkg.dev/acme/webhooks:v1.9.0"},
			},
			EnvVars: []EnvBinding{
				{Name: "PUBSUB_SUBSCRIPTION", Value: "projects/acme/subscriptions/stripe-events-sub", IsSecret: false},
			},
			TopologyASCII: []string{
				"  [📨 Pub/Sub Topic: stripe-events]",
				"          │  🔄 No Dead-Letter Queue (Poison-pill message retrying 50,000x/hr!)",
				"          ▼",
				"  [🚀 Cloud Run: pubsub-webhook-ingestor] (💸 Burning $2,340/mo - 94% 500s)",
			},
			TerraformHCL: `resource "google_pubsub_subscription" "stripe_events_sub" {
  name  = "stripe-events-sub"
  topic = google_pubsub_topic.stripe_events.id

  dead_letter_policy {
    dead_letter_topic     = google_pubsub_topic.stripe_events_dlq.id
    max_delivery_attempts = 5
  }
  retry_policy {
    minimum_backoff = "10s"
    maximum_backoff = "600s"
  }
}`,
			GcloudCmd: "gcloud pubsub subscriptions update stripe-events-sub --dead-letter-topic=stripe-events-dlq --max-delivery-attempts=5",
			Logs: []LogEntry{
				{Timestamp: "17:09:50.110", Severity: "ERROR", Component: "ingestor", Message: "Unmarshal error on msgID=88192019: unexpected EOF (DeliveryAttempt: 4,812)", TraceID: "tr-dlq-01"},
				{Timestamp: "17:09:51.210", Severity: "ERROR", Component: "ingestor", Message: "Unmarshal error on msgID=88192019: unexpected EOF (DeliveryAttempt: 4,813)", TraceID: "tr-dlq-02"},
			},
			AIInsight: "🤖 Gemini FinOps Alert:\nPoison-pill message #88192019 has been retried 4,813 times because subscription 'stripe-events-sub' has no Dead-Letter Topic (DLQ). Attaching a DLQ with max_delivery_attempts=5 saves $1,980/month immediately.",
		},
		{
			ID:          "run-invoice-pdf",
			Name:        "invoice-pdf-renderer",
			Category:    CatCloudRun,
			Kind:        "CloudRun Function",
			Region:      "us-central1",
			Status:      "READY",
			Risk:        RiskHealthy,
			MonthlyCost: 38.00,
			WasteAmount: 0,
			Summary:     "Healthy scale-to-zero Cloud Run gen2 function • Least-privilege SA & VPC-SC protected",
			URL:         "https://invoice-pdf-renderer-uc1.a.run.app",
			ServiceAcct: "invoice-pdf-sa@acme-fintech-prod-01.iam.gserviceaccount.com",
			Metrics: map[string]string{
				"RPS":         "12 req/s",
				"Error Rate":  "0.01%",
				"p99 Latency": "180 ms",
			},
			Revisions: []TrafficSplit{
				{Revision: "invoice-pdf-renderer-00012", Percent: 100, Status: "HEALTHY", Created: "14d ago", Image: "uc1-docker.pkg.dev/acme/pdf:v1.2.0"},
			},
			TerraformHCL: `# Resource is already compliant with Google Cloud Well-Architected Framework (Grade A+)`,
			GcloudCmd:    "gcloud run services describe invoice-pdf-renderer --region=us-central1",
			Logs: []LogEntry{
				{Timestamp: "17:09:40.000", Severity: "INFO", Component: "pdf-worker", Message: "Rendered invoice INV-2026-9941 in 142ms", TraceID: "tr-pdf-1"},
			},
			AIInsight: "✅ Grade A+ Architecture: Uses dedicated Service Account with 2 permissions, scale-to-zero enabled, and Secret Manager references.",
		},

		// 2. IAM-ZERO & SECURITY
		{
			ID:          "iam-default-compute",
			Name:        "104928192-compute@developer.gserviceaccount.com",
			Category:    CatIAMZero,
			Kind:        "ServiceAccount (Default)",
			Region:      "global",
			Status:      "OVER-PRIVILEGED (99.9%)",
			Risk:        RiskCritical,
			MonthlyCost: 0,
			Summary:     "Granted 6,842 permissions (roles/editor) • AST & Audit Logs prove only 4 permissions used!",
			ServiceAcct: "104928192-compute@developer.gserviceaccount.com",
			Metrics: map[string]string{
				"Granted Role":      "roles/editor",
				"Granted Perms":     "6,842 permissions",
				"Actually Used":     "4 permissions (90-day Cloud Audit Log + AST Scan)",
				"Excess Blast Risk": "99.94% unnecessary privileges",
				"User-Managed Keys": "2 Active JSON Keys (1 older than 410 days!)",
			},
			IAMInfo: &IAMBindingInfo{
				Principal:         "104928192-compute@developer.gserviceaccount.com",
				Role:              "roles/editor",
				GrantedPermsCount: 6842,
				UsedPermsCount:    4,
				ActuallyUsedPerms: []string{
					"cloudsql.instances.connect",
					"secretmanager.versions.access",
					"pubsub.topics.publish",
					"logging.logEntries.create",
				},
				UnusedRiskPerms: []string{
					"resourcemanager.projects.setIamPolicy",
					"storage.buckets.delete",
					"iam.serviceAccountKeys.create",
					"bigquery.datasets.delete",
					"compute.firewalls.delete",
				},
				LastUsed:        "Active now (checkout-payment-api)",
				SourceCodeFiles: []string{"src/db/pool.go:42", "src/secrets/loader.go:19", "src/events/pubsub.go:88"},
				Risk:            RiskCritical,
			},
			TopologyASCII: []string{
				"  [🔑 104928192-compute@developer.gserviceaccount.com]",
				"     ├── 🟢 ACTUALLY USED IN CODE (4 perms):",
				"     │     ├─ cloudsql.instances.connect    (src/db/pool.go:42)",
				"     │     ├─ secretmanager.versions.access (src/secrets/loader.go:19)",
				"     │     ├─ pubsub.topics.publish         (src/events/pubsub.go:88)",
				"     │     └─ logging.logEntries.create     (runtime stdout)",
				"     │",
				"     └── 🔴 UNUSED TOXIC BLAST RADIUS (6,838 excess perms!):",
				"           ├─ 💥 Can delete ALL 14 GCS Buckets (incl. customer-kyc-vault)",
				"           ├─ 💥 Can dump ALL 28 Secret Manager keys",
				"           ├─ 💥 Can mint new persistent JSON Service Account keys",
				"           └─ 💥 2 leaked/unrotated JSON keys active since 2025",
			},
			TerraformHCL: `# 🛡️ IAM-Zero Synthesized Least-Privilege Policy (Eliminates 6,838 excess permissions)
resource "google_project_iam_custom_role" "iam_zero_checkout" {
  role_id     = "iamZeroCheckoutRuntime"
  title       = "IAM-Zero Minimal Runtime Role"
  description = "Auto-synthesized by lazygcp IAM-Zero from AST + 90d Cloud Audit Logs"
  permissions = [
    "cloudsql.instances.connect",
    "secretmanager.versions.access",
    "pubsub.topics.publish",
    "logging.logEntries.create",
  ]
}

resource "google_project_iam_member" "bind_minimal" {
  project = "acme-fintech-prod-01"
  role    = google_project_iam_custom_role.iam_zero_checkout.id
  member  = "serviceAccount:${google_service_account.checkout_sa.email}"
}`,
			GcloudCmd: "gcloud iam roles create iamZeroCheckoutRuntime --project=acme-fintech-prod-01 --permissions=cloudsql.instances.connect,secretmanager.versions.access,pubsub.topics.publish,logging.logEntries.create",
			AIInsight: "🛡️ IAM-Zero AST + Audit Verdict:\nIf an attacker exploits an SSRF or dependency vulnerability in 'checkout-payment-api', they can query http://169.254.169.254/computeMetadata/v1/instance/service-accounts/default/token and exfiltrate your entire project. Press [f] to apply the 4-permission surgical custom role.",
		},
		{
			ID:          "iam-github-ci-key",
			Name:        "github-actions-deployer@acme-fintech-prod-01.iam",
			Category:    CatIAMZero,
			Kind:        "ServiceAccount + Static Key",
			Region:      "global",
			Status:      "STATIC JSON KEY LEAK RISK",
			Risk:        RiskHigh,
			MonthlyCost: 0,
			Summary:     "Uses long-lived JSON key in GitHub Actions instead of keyless Workload Identity Federation (WIF)",
			ServiceAcct: "github-actions-deployer@acme-fintech-prod-01.iam.gserviceaccount.com",
			Metrics: map[string]string{
				"Granted Role": "roles/owner",
				"Auth Method":  "Static JSON Key (Key ID: 8f91a2b... age: 294 days)",
				"Recommended":  "Keyless Workload Identity Federation (OIDC)",
			},
			IAMInfo: &IAMBindingInfo{
				Principal:         "github-actions-deployer@acme-fintech-prod-01.iam.gserviceaccount.com",
				Role:              "roles/owner",
				GrantedPermsCount: 7410,
				UsedPermsCount:    11,
				ActuallyUsedPerms: []string{
					"run.services.update",
					"run.services.get",
					"artifactregistry.repositories.uploadArtifacts",
					"iam.serviceAccounts.actAs",
				},
				UnusedRiskPerms: []string{
					"billing.accounts.update",
					"resourcemanager.projects.delete",
					"orgpolicy.policy.set",
				},
				LastUsed:        "4 hours ago (GitHub Actions runner IP)",
				SourceCodeFiles: []string{".github/workflows/deploy-prod.yml:38"},
				Risk:            RiskHigh,
			},
			TerraformHCL: `# 🛡️ IAM-Zero Keyless Workload Identity Federation (WIF) Upgrade
resource "google_iam_workload_identity_pool" "github_pool" {
  workload_identity_pool_id = "github-actions-pool"
}

resource "google_iam_workload_identity_pool_provider" "github_oidc" {
  workload_identity_pool_id          = google_iam_workload_identity_pool.github_pool.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-provider"
  attribute_condition                = "assertion.repository_owner == 'acme-fintech'"
  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
  }
}`,
			GcloudCmd: "gcloud iam service-accounts keys delete 8f91a2b4910c --iam-account=github-actions-deployer@acme-fintech-prod-01.iam.gserviceaccount.com",
			AIInsight: "🔑 Replace exported JSON Service Account key in .github/workflows/deploy-prod.yml with keyless OIDC Workload Identity Federation and scope role from roles/owner down to Cloud Run + Artifact Registry.",
		},

		// 3. GKE & COMPUTE VMs
		{
			ID:          "gke-core-autopilot",
			Name:        "gke-prod-eu-west1",
			Category:    CatGKECompute,
			Kind:        "GKE Standard Cluster",
			Region:      "europe-west1-b",
			Status:      "PUBLIC ENDPOINT",
			Risk:        RiskHigh,
			MonthlyCost: 1890.00,
			WasteAmount: 740.00,
			Summary:     "Control plane public without Master Authorized Networks • Workload Identity disabled on node pool",
			Metrics: map[string]string{
				"K8s Version":       "1.31.2-gke.1100",
				"Nodes":             "12x n2-standard-8 (Avg CPU Request: 18%)",
				"Workload Identity": "DISABLED (Nodes share default SA)",
				"Control Plane":     "Public IP (0.0.0.0/0 allowed)",
			},
			TopologyASCII: []string{
				"  [☸️ GKE Cluster: gke-prod-eu-west1]",
				"     ├── ⚠️ Control Plane API: Publicly reachable without Authorized CIDR",
				"     ├── 💸 NodePool 'default-pool': 12x n2-standard-8 (82% CPU idle -> $740/mo waste)",
				"     └── 🔴 Legacy Metadata Server enabled (Pod -> Node SA privilege escalation risk)",
			},
			TerraformHCL: `resource "google_container_cluster" "gke_prod" {
  name     = "gke-prod-eu-west1"
  location = "europe-west1"
  enable_autopilot = true

  private_cluster_config {
    enable_private_nodes    = true
    enable_private_endpoint = false
  }
  master_authorized_networks_config {
    cidr_blocks {
      cidr_block   = "10.0.0.0/8"
      display_name = "Corp VPN & Bastion"
    }
  }
}`,
			GcloudCmd: "gcloud container clusters get-credentials gke-prod-eu-west1 --zone europe-west1-b",
			AIInsight: "☸️ Press [p] to open an instant IAP Kubectl Proxy tunnel, or [t] to generate the GKE Workload Identity & Master Authorized Networks hardening patch.",
		},
		{
			ID:          "gce-legacy-bastion",
			Name:        "legacy-jumpbox-vm-01",
			Category:    CatGKECompute,
			Kind:        "Compute Engine VM",
			Region:      "europe-west1-b",
			Status:      "POWERED ON (RUNNING)",
			Risk:        RiskCritical,
			MonthlyCost: 195.00,
			WasteAmount: 195.00,
			Summary:     "POWERED ON (RUNNING) • Public IP 34.77.118.42 • Port 22 open to 0.0.0.0/0 • Press [s] for Inline SSH",
			Metrics: map[string]string{
				"Power State":  "POWERED ON (RUNNING) — Inline SSH Ready",
				"Machine Type": "n2-standard-4 (CPU avg: 0.4%)",
				"External IP":  "34.77.118.42 (Port 22 open to world!)",
				"Internal IP":  "10.132.0.14",
				"Shielded VM":  "Disabled",
			},
			TerraformHCL: `# Remove external IP and use IAP TCP Forwarding (35.235.240.0/20)
resource "google_compute_firewall" "allow_iap_ssh" {
  name          = "allow-ssh-via-iap-only"
  network       = "prod-vpc"
  source_ranges = ["35.235.240.0/20"]
  allow {
    protocol = "tcp"
    ports    = ["22"]
  }
}`,
			GcloudCmd: "gcloud compute ssh legacy-jumpbox-vm-01 --zone=europe-west1-b --tunnel-through-iap",
			AIInsight: "🔥 VM is POWERED ON (RUNNING). Press [s] to open an interactive stateful SSH terminal directly inside lazygcp, or [e] to execute a one-off remote command.",
		},
		{
			ID:          "gce-stopped-batch",
			Name:        "staging-batch-worker-02",
			Category:    CatGKECompute,
			Kind:        "Compute Engine VM",
			Region:      "europe-west1-b",
			Status:      "POWERED OFF (STOPPED)",
			Risk:        RiskLow,
			MonthlyCost: 18.00,
			WasteAmount: 18.00,
			Summary:     "POWERED OFF (STOPPED) • Inline SSH disabled while VM is powered off • Boot disk still billed ($18/mo)",
			Metrics: map[string]string{
				"Power State":  "POWERED OFF (STOPPED) — SSH Disabled (Start VM first)",
				"Machine Type": "e2-standard-4",
				"Internal IP":  "10.132.0.29",
				"External IP":  "None (Private IAP Only)",
			},
			TerraformHCL: `resource "google_compute_instance" "staging_batch_worker_02" {
  name           = "staging-batch-worker-02"
  machine_type   = "e2-standard-4"
  zone           = "europe-west1-b"
  desired_status = "TERMINATED"
}`,
			GcloudCmd: "gcloud compute instances start staging-batch-worker-02 --zone=europe-west1-b --project=" + detectedProject,
			AIInsight: "⏸️ VM 'staging-batch-worker-02' is currently POWERED OFF (STOPPED). Inline SSH terminal ([s]) and remote command execution ([e]) are automatically disabled until the VM is started.",
		},

		// 4. CLOUD SQL & STORAGE
		{
			ID:          "sql-prod-orders",
			Name:        "prod-orders-pg16",
			Category:    CatStorageSQL,
			Kind:        "Cloud SQL PostgreSQL 16",
			Region:      "europe-west1",
			Status:      "CONN POOL 100% FULL",
			Risk:        RiskCritical,
			MonthlyCost: 980.00,
			WasteAmount: 0,
			Summary:     "500/500 connections saturated • No HA Regional Failover • Public IP enabled with 0.0.0.0/0 SSL off",
			Metrics: map[string]string{
				"Tier":            "db-custom-8-32768",
				"Connections":     "500 / 500 (100% Saturated - Rejecting clients)",
				"High Avail (HA)": "ZONAL (Single Point of Failure!)",
				"Backups & PITR":  "Automated Backups ON, Point-in-Time Recovery OFF",
			},
			TopologyASCII: []string{
				"  [🗄️ Cloud SQL: prod-orders-pg16]",
				"     ├── 🔴 Active Connections: 500 / 500 (Rejecting new connections from checkout-api)",
				"     ├── ⚠️ Availability: ZONAL (Zone outage = total data downtime)",
				"     └── 💡 Fix: Enable Managed Connection Pooling / PgBouncer + REGIONAL HA",
			},
			TerraformHCL: `resource "google_sql_database_instance" "prod_orders_pg16" {
  name             = "prod-orders-pg16"
  database_version = "POSTGRES_16"
  region           = "europe-west1"

  settings {
    tier              = "db-custom-8-32768"
    availability_type = "REGIONAL"
    ip_configuration {
      ipv4_enabled    = false
      private_network = google_compute_network.prod_vpc.id
      ssl_mode        = "ENCRYPTED_ONLY"
    }
    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
    }
  }
}`,
			GcloudCmd: "cloud-sql-proxy acme-fintech-prod-01:europe-west1:prod-orders-pg16 --port 5432 --auto-iam-authn",
			AIInsight: "🔌 Press [p] to launch background Cloud SQL Auth Proxy with automatic IAM DB authentication on localhost:5432, or [e] to execute an interactive SQL query!",
		},
		{
			ID:          "gcs-kyc-documents",
			Name:        "gs://acme-customer-kyc-passports-prod",
			Category:    CatStorageSQL,
			Kind:        "Cloud Storage Bucket",
			Region:      "eu (Multi-Region)",
			Status:      "PUBLIC (allUsers)!",
			Risk:        RiskCritical,
			MonthlyCost: 410.00,
			WasteAmount: 185.00,
			Summary:     "CRITICAL GDPR BREACH: allUsers has roles/storage.objectViewer on 48,000 passport scans! No Lifecycle rule",
			Metrics: map[string]string{
				"Public Access":     "EXPOSED (allUsers -> storage.objectViewer)",
				"Objects":           "48,912 PDF/JPEG files (18.4 TB)",
				"Encryption":        "Google-managed (Missing CMEK Required by Policy)",
				"Lifecycle Policy":  "NONE (14 TB untouched > 2 years = $185/mo waste)",
				"Soft Delete / Ver": "Disabled",
			},
			TerraformHCL: `resource "google_storage_bucket" "kyc_passports" {
  name                        = "acme-customer-kyc-passports-prod"
  location                    = "EU"
  public_access_prevention    = "enforced"
  uniform_bucket_level_access = true

  lifecycle_rule {
    condition { age = 90 }
    action {
      type          = "SetStorageClass"
      storage_class = "ARCHIVE"
    }
  }
}`,
			GcloudCmd: "gcloud storage buckets update gs://acme-customer-kyc-passports-prod --public-access-prevention",
			AIInsight: "🚨 GDPR Art. 32 & CIS 5.1 Violation: Bucket is publicly readable via allUsers! Press [e] to list bucket objects or [f] to enforce public_access_prevention.",
		},

		// 5. SECRETS & KMS VAULT
		{
			ID:          "sec-stripe-live",
			Name:        "stripe-live-webhook-secret",
			Category:    CatSecretsEnv,
			Kind:        "Secret Manager",
			Region:      "global",
			Status:      "UNROTATED (412d)",
			Risk:        RiskMedium,
			MonthlyCost: 1.20,
			Summary:     "Secret version v1 has not been rotated in 412 days • Accessed by 3 over-privileged Service Accounts",
			Metrics: map[string]string{
				"Latest Version": "v1 (Created 412 days ago)",
				"Rotation":       "Not configured",
				"Consumers":      "checkout-payment-api, pubsub-webhook-ingestor, default-compute-sa",
				"Masked Value":   "whsec_99182********************a81b (Press [x] to peek)",
			},
			TerraformHCL: `resource "google_secret_manager_secret" "stripe_webhook" {
  secret_id = "stripe-live-webhook-secret"
  rotation {
    rotation_period = "7776000s" # 90 days
  }
}`,
			GcloudCmd: "gcloud secrets versions access latest --secret=stripe-live-webhook-secret",
			AIInsight: "🔑 Secret is consumed as plaintext env var in checkout-payment-api instead of mounted SecretRef, and has no 90-day rotation schedule.",
		},

		// 6. VPC, NAT & CLOUD ARMOR
		{
			ID:          "net-vpc-connector",
			Name:        "allow-ssh-world-0-0-0-0",
			Category:    CatNetworkVPC,
			Kind:        "VPC Firewall (INGRESS ALLOW)",
			Region:      "global",
			Status:      "OPEN TO WORLD (0.0.0.0/0)",
			Risk:        RiskCritical,
			MonthlyCost: 310.00,
			WasteAmount: 140.00,
			Summary:     "INGRESS ALLOW on tcp:22, tcp:3389 from 0.0.0.0/0 • Press [b] to Immediately Quarantine & Block",
			Metrics: map[string]string{
				"Network":           "prod-vpc",
				"Direction & Act":   "INGRESS / ALLOW (Priority: 1000)",
				"Allowed Ports":     "tcp:22, tcp:3389",
				"Source CIDRs":      "0.0.0.0/0 (Public Internet!)",
				"Target Tags":       "ALL INSTANCES IN VPC",
				"Emergency Action":  "Press [b] to quarantine rule (--action=DENY --priority=100)",
			},
			TopologyASCII: []string{
				"  [🌍 Public Internet (0.0.0.0/0)]",
				"          │",
				"          ▼",
				"  [🔥 Firewall Rule: allow-ssh-world-0-0-0-0 (INGRESS ALLOW tcp:22,3389)]",
				"          │",
				"          └──► [🖥️ All Compute VMs in prod-vpc] (🔴 Direct SSH/RDP Brute-Force Exposure!)",
			},
			TerraformHCL: `resource "google_compute_firewall" "quarantine_ssh_world" {
  name          = "allow-ssh-world-0-0-0-0"
  network       = "prod-vpc"
  direction     = "INGRESS"
  priority      = 100
  source_ranges = ["0.0.0.0/0"]
  deny {
    protocol = "all"
  }
}`,
			GcloudCmd: "gcloud compute firewall-rules describe allow-ssh-world-0-0-0-0 --project=" + detectedProject,
			BlockCmd:  "gcloud compute firewall-rules update allow-ssh-world-0-0-0-0 --project=" + detectedProject + " --priority=100 --rules=all --action=DENY",
			AIInsight: "🔥 CRITICAL EXPOSURE: Firewall rule allows unrestricted world ingress (0.0.0.0/0) on SSH/RDP. Press [b] to immediately block and quarantine this rule.",
		},

		// 7. VERTEX AI & AGENTS
		{
			ID:          "vertex-support-agent",
			Name:        "customer-support-rag-agent (Gemini 2.5 Pro)",
			Category:    CatVertexAI,
			Kind:        "Vertex AI Agent + Endpoint",
			Region:      "europe-west1",
			Status:      "PROMPT INJECTION + $ BURN",
			Risk:        RiskCritical,
			MonthlyCost: 4820.00,
			WasteAmount: 3410.00,
			Summary:     "No Model Armor prompt-injection shield • Context Caching OFF (re-sending 90k token manual every request!)",
			Metrics: map[string]string{
				"Model":           "gemini-2.5-pro (100% traffic - No Flash routing)",
				"Context Caching": "DISABLED (Re-billing 92,000 static system tokens per call!)",
				"Model Armor":     "DISABLED (Vulnerable to Prompt Injection & PII leak)",
				"Agent SA Role":   "roles/storage.admin (Can read/write all buckets!)",
				"Token Burn Rate": "$160 / day ($3,410/mo reclaimable via Context Cache + Flash)",
			},
			TopologyASCII: []string{
				"  [💬 User Chat Input] ──► ⚠️ No Model Armor (Prompt Injection / Token Sponge Attack!)",
				"          │",
				"          ▼",
				"  [🤖 Vertex AI Agent: customer-support-rag-agent]",
				"     ├── 💸 Sending 92,000 uncached tokens/req to Gemini 2.5 Pro ($4,820/mo)",
				"     └── 🔴 Agent Tool SA has roles/storage.admin (Can exfiltrate gs://acme-customer-kyc!)",
			},
			TerraformHCL: `# 🛡️ Vertex AI Model Armor + Context Caching FinOps Optimization
resource "google_model_armor_template" "agent_shield" {
  location    = "europe-west1"
  template_id = "prod-agent-guardrails"
  filter_config {
    pi_and_jailbreak_filter_settings {
      filter_enforcement = "ENABLED"
      confidence_level   = "MEDIUM_AND_ABOVE"
    }
    sdp_settings {
      basic_config { filter_enforcement = "ENABLED" }
    }
  }
}`,
			GcloudCmd: "gcloud alpha model-armor templates list --location=europe-west1",
			AIInsight: "🤖 GenAI FinOps & SAIF Audit:\n1. Enabling Gemini Context Caching on the 92k-token product manual + routing tier-1 queries to Gemini 2.5 Flash cuts your monthly Vertex AI bill from $4,820/mo to $1,410/mo (-$3,410/mo!).\n2. Enable Model Armor to block prompt-injection tool abuse.",
		},

		// 8. FINOPS & CLOUD BILLING
		{
			ID:          "finops-billing-overview",
			Name:        "💳 Cloud Billing (Acme Corp Production Billing)",
			Category:    CatFinOps,
			Kind:        "Cloud Billing Account",
			Region:      "global",
			Status:      "ACTIVE ($12,422/mo)",
			Risk:        RiskMedium,
			MonthlyCost: 0,
			WasteAmount: 0,
			Summary:     "Account: billingAccounts/01A8F2-99C4E1-48B102 • Burn Rate: $414.07/day ($12,422/mo) • Reclaimable Waste: $8,678/mo",
			Metrics: map[string]string{
				"Billing Account":   "Acme Corp Production Billing (billingAccounts/01A8F2-99C4E1-48B102)",
				"Billing Status":    "BILLING ENABLED (ACTIVE) • Currency: USD",
				"Daily Burn Rate":   "$414.07 / day",
				"Monthly Run-Rate":  "$12,422.20 / month (Projected Annual: $149,066 / yr)",
				"Identified Waste":  "$8,678.00 / month ($104,136 / yr savings available)",
				"Top Cost Drivers":  "Vertex AI ($4,820/mo) • Cloud Run ($3,798/mo) • GKE & VMs ($2,103/mo)",
			},
			TerraformHCL: `# 💰 Google Cloud Billing Budget & 50%/90%/100% Spend Alert Guardrail
resource "google_billing_budget" "project_monthly_budget" {
  billing_account = "01A8F2-99C4E1-48B102"
  display_name    = "acme-fintech-prod-01-monthly-budget"

  budget_filter {
    projects = ["projects/acme-fintech-prod-01"]
  }

  amount {
    specified_amount {
      currency_code = "USD"
      units         = "10000"
    }
  }

  threshold_rules { threshold_percent = 0.5 }
  threshold_rules { threshold_percent = 0.9 }
  threshold_rules { threshold_percent = 1.0 }
}`,
			GcloudCmd: "gcloud billing projects describe " + detectedProject,
			AIInsight: "💳 Cloud Billing Perspective:\n• Linked Billing Account: Acme Corp Production Billing (billingAccounts/01A8F2-99C4E1-48B102)\n• Current Run-Rate: $414.07/day ($12,422.20/mo)\n• Press [t] to export a google_billing_budget Terraform guardrail.",
		},
		{
			ID:          "finops-zombie-disks",
			Name:        "14x Unattached pd-ssd Persistent Disks & 6 Static IPs",
			Category:    CatFinOps,
			Kind:        "Zombie Compute & Storage Waste",
			Region:      "multi-region",
			Status:      "100% IDLE WASTE",
			Risk:        RiskMedium,
			MonthlyCost: 1120.00,
			WasteAmount: 1120.00,
			Summary:     "14 orphaned SSD persistent disks (6.4 TB total) from deleted GKE clusters + 6 unused reserved external IPs",
			Metrics: map[string]string{
				"Orphaned Disks":   "14x pd-ssd (Unattached > 60 days) = $1,088/mo",
				"Unused Static IP": "6x Reserved External IPv4 = $32/mo",
				"Total Reclaim":    "$1,120.00 / month ($13,440 / year)",
			},
			TerraformHCL: `# Snapshot & delete unattached zombie disks to reclaim $1,120/mo immediately`,
			GcloudCmd:    "gcloud compute disks list --filter=\"-users:*\" --format=\"table(name,zone,sizeGb,type)\"",
			AIInsight:    "💸 Run the generated gcloud cleanup script or press [e] to snapshot to Archive Storage ($7/mo) and delete all 14 unattached SSD volumes (-$1,120/mo).",
		},
	}

	auditEvents := []AuditDiffEvent{
		{
			ID:        "evt-901",
			TimeAgo:   "18m ago",
			Timestamp: "2026-10-15 16:51:12 UTC",
			Actor:     "github-actions-deployer@acme-fintech-prod-01.iam",
			ActorType: "CI_PIPELINE",
			CallerIP:  "140.82.112.44",
			Action:    "run.services.replaceService",
			Resource:  "checkout-payment-api (europe-west1)",
			Risk:      RiskCritical,
			Summary:   "Deployed rev-00042 with DB_MAX_CONNS=50 and plaintext STRIPE_LIVE_SECRET_KEY",
			DiffLines: []string{
				"@@ CloudRun Service: checkout-payment-api @@",
				"- image: ew1-docker.pkg.dev/acme/pay/api:v2.4.0",
				"+ image: ew1-docker.pkg.dev/acme/pay/api:v2.4.1",
				"- env.DB_MAX_CONNS: \"5\"",
				"+ env.DB_MAX_CONNS: \"50\"   # <-- CAUSED CLOUD SQL 500/500 CONN EXHAUSTION!",
				"+ env.STRIPE_LIVE_SECRET_KEY: \"sk_live_51Nx92J...\" # <-- PLAINTEXT SECRET LEAK!",
			},
			InspectCmd: "gcloud logging read 'protoPayload.serviceName=\"run.googleapis.com\"' --limit=1 --format=json",
			Rollback:   "gcloud run services update-traffic checkout-payment-api --to-revisions=checkout-payment-api-00041-ok=100 --region=europe-west1",
		},
		{
			ID:        "evt-902",
			TimeAgo:   "47m ago",
			Timestamp: "2026-10-15 16:22:04 UTC",
			Actor:     "dev-intern@acme-cloud.io",
			ActorType: "HUMAN (Console ClickOps)",
			CallerIP:  "84.19.41.102",
			Action:    "storage.setIamPermissions",
			Resource:  "gs://acme-customer-kyc-passports-prod",
			Risk:      RiskCritical,
			Summary:   "Granted roles/storage.objectViewer to allUsers on KYC passport bucket",
			DiffLines: []string{
				"@@ GCS Bucket IAM Policy: gs://acme-customer-kyc-passports-prod @@",
				"  bindings:",
				"  - role: roles/storage.objectViewer",
				"    members:",
				"    - serviceAccount:kyc-verifier@acme-fintech-prod-01.iam.gserviceaccount.com",
				"+   - allUsers   # <-- CRITICAL: MADE 48,000 PASSPORT SCANS PUBLIC TO INTERNET!",
			},
			InspectCmd: "gcloud storage buckets get-iam-policy gs://acme-customer-kyc-passports-prod --format=json",
			Rollback:   "gcloud storage buckets remove-iam-policy-binding gs://acme-customer-kyc-passports-prod --member=allUsers --role=roles/storage.objectViewer",
		},
		{
			ID:        "evt-903",
			TimeAgo:   "2h ago",
			Timestamp: "2026-10-15 15:04:50 UTC",
			Actor:     "admin@acme-cloud.io",
			ActorType: "HUMAN (Console ClickOps)",
			CallerIP:  "91.210.14.8",
			Action:    "compute.firewalls.patch",
			Resource:  "allow-ssh-world-0-0-0-0",
			Risk:      RiskHigh,
			Summary:   "Opened TCP:22 & TCP:3389 to 0.0.0.0/0 on prod-vpc",
			DiffLines: []string{
				"@@ VPC Firewall: allow-ssh-world-0-0-0-0 (tcp:22,3389) @@",
				"- sourceRanges: [\"35.235.240.0/20\"] # Identity-Aware Proxy (IAP)",
				"+ sourceRanges: [\"0.0.0.0/0\"]        # <-- EXPOSED PORT 22 TO WORLD!",
			},
			InspectCmd: "gcloud compute firewall-rules describe allow-ssh-world-0-0-0-0 --format=json",
			Rollback:   "gcloud compute firewall-rules update allow-ssh-world-0-0-0-0 --source-ranges=35.235.240.0/20",
		},
	}

	var totalCost, totalWaste float64
	for _, r := range resources {
		totalCost += r.MonthlyCost
		totalWaste += r.WasteAmount
	}

	return &ProjectState{
		ProjectID:          detectedProject,
		ProjectList:        []string{detectedProject, "acme-fintech-staging-02", "acme-ai-sandbox-labs", "gcp-dojo-chaos-arena"},
		Region:             "europe-west1",
		ActiveAccount:      detectedAccount,
		LiveMode:           false,
		BillingAccountID:   "billingAccounts/01A8F2-99C4E1-48B102",
		BillingAccountName: "Acme Corp Production Billing",
		BillingEnabled:     true,
		BillingCurrency:    "USD",
		TotalMonthly:       totalCost,
		Reclaimable:        totalWaste,
		SecurityScore:      54,
		Grade:              "D+",
		ActiveTunnels:      make(map[string]string),
		Resources:          resources,
		AuditTimeline:      auditEvents,
		LastSync:           time.Now(),
	}
}
