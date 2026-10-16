# Security Policy 🛡️

## Security & Credential Architecture

`lazygcp` is designed from the ground up for security-conscious SREs, Cloud Architects, and DevSecOps engineers:

1. **Zero Telemetry / Zero Third-Party Network Calls:**
   `lazygcp` has **zero external Go dependencies** and never phones home or transmits telemetry. Every API call goes strictly to official `*.googleapis.com` endpoints or your local `gcloud` CLI.
2. **Delegated Authentication (ADC / `gcloud`):**
   `lazygcp` never stores long-lived service account private keys on disk. It reads short-lived OAuth2 access tokens directly from your active `gcloud auth` / Application Default Credentials session in memory.
3. **Secret Masking by Default:**
   Sensitive environment variables and Secret Manager payloads are masked (`********`) by default in all TUI views and require explicit operator action (`[x]`) to unmask in-memory.

---

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| `0.4.x` | :white_check_mark: |
| `< 0.4` | :x:                |

---

## Reporting a Vulnerability

If you discover a security vulnerability in `lazygcp` (for example, unintended credential exposure in logs, command injection in CLI wrappers, or terminal escape sequence vulnerabilities in the VT100 parser):

1. **Do NOT open a public GitHub issue.**
2. Please report it privately via **[GitHub Security Advisories](https://github.com/damiansztankowski/lazygcp/security/advisories/new)** on this repository.
3. Include steps to reproduce, affected OS/architecture, and potential impact.

We aim to acknowledge all security reports within **48 hours** and release a patched version within **7 days** of verification.
