# Contributing to `lazygcp` ⚡

Thank you for your interest in contributing to **`lazygcp`**! Whether you are fixing a bug, adding a new Google Cloud module capability, improving the embedded VT100 SSH terminal, or refining 1:1 Reverse-Terraform generation, your contributions are welcome.

---

## 🏗️ Core Engineering Principles

1. **Zero External Go Dependencies (`go.mod`):**
   `lazygcp` is engineered using 100% pure Go standard library (`net/http`, `os/exec`, `syscall`, `encoding/json`). Do not add third-party packages to `go.mod` unless discussed and approved in an issue first.
2. **Cross-Platform Compilation (`darwin`, `linux`, `windows`):**
   OS-specific terminal or PTY syscalls must be isolated behind Go build tags:
   - `internal/tui/pty_bsd.go` (`//go:build darwin || freebsd || openbsd || netbsd`)
   - `internal/tui/pty_linux.go` (`//go:build linux`)
   - `internal/tui/pty_windows.go` (`//go:build windows`)
   - `internal/tui/term_unix.go` & `internal/tui/term_windows.go`
3. **Safe by Default:**
   Read/discovery operations run automatically in parallel on startup or `[R]` refresh. Any mutating action (such as emergency firewall block `[b]`, revision rollback `[r]`, or disk cleanup `[e]`) must clearly report the exact `gcloud` command executed.

---

## 🛠️ Local Development Setup

### Prerequisites
- **Go 1.22+** (tested up to Go 1.26+)
- **`gcloud` CLI** (optional for `--demo` mode; required for live GCP project discovery)

### Build, Run & Test
```bash
# Build native binary
make build

# Run with simulated production environment (no GCP credentials required)
make demo

# Run unit tests & vet checks
go vet ./...
go test -v ./...

# Verify cross-compilation across macOS, Linux, and Windows (amd64 & arm64)
make release-check
```

---

## 📝 Commit Message Convention

We follow **Conventional Commits** for automated changelog generation via GoReleaser:

- `feat(tui): add full-height toggle for inline VT100 SSH session`
- `feat(finops): query Cloud Billing API v1 for daily and monthly burn rate`
- `fix(gke): block inline SSH activation when Compute VM is powered off`
- `docs(readme): document keyboard shortcuts and module capabilities`
- `chore(ci): update GitHub Actions release matrix`

---

## 🔄 Pull Request Process

1. Fork the repository and create your branch from `main` (`git checkout -b feat/my-feature`).
2. Ensure `go vet ./...`, `go test ./...`, and `make release-check` pass with zero errors.
3. Update `README.md` or the in-app Help Modal (`internal/tui/render.go`) if you introduce new keybindings or commands.
4. Open a Pull Request referencing any relevant issue numbers.

---

## 📜 License

By contributing to `lazygcp`, you agree that your contributions will be licensed under the [Apache License, Version 2.0](./LICENSE).
