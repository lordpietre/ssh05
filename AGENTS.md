# trzsz-ssh (tssh) — Agent Guide

## Project
Drop-in OpenSSH client replacement with TUI login prompt, trzsz/zmodem file transfer, UDP mode (QUIC/KCP), and batch login. Go module `github.com/trzsz/trzsz-ssh`.

## Key commands
```
go build -o ./bin/ ./cmd/tssh
go test -v -count=1 ./tssh    # must run from repo root
make test                      # same as above (prefers `gotest` on unix)
make install                   # copies ./bin/tssh to /usr/bin/tssh
```

## Toolchain
- **Go 1.25** required (`go.mod` declares `go 1.25.0`)
- No linter or typecheck config exists (no `.golangci.yml`, no `go vet` in CI)
- CGO must be **disabled** for all builds (`CGO_ENABLED=0`)
- All forked dependencies live under `github.com/trzsz/` (go-arg, ssh_config, promptui, quic-go, kcp-go, smux, etc.)

## Entrypoints
- **Binary**: `cmd/tssh/main.go` → `tssh.TsshMain(argv)` in `tssh/main.go`
- **Library**: `tssh.SshLogin(args)` in `tssh/ssh.go`
- **Internal packages**: `internal/table/`, `internal/krb5/`, `internal/ssh/`

## Architecture notes
- All business logic lives in `tssh/` (70 `.go` files). Main package is thin.
- Version constant `kTsshVersion` in `tssh/version.go:36`
- Exit codes defined in `tssh/comm.go:50-70` (e.g. `kExitCodeLoginFailed=16`)
- `#!!` prefix enables tssh-specific options in `~/.ssh/config` (standard ssh treats it as comment)
- `enc` prefix on config values means encrypted with `tssh --enc-secret`
- Android/Termux quirk: `cmd/tssh/main.go` adjusts `os.Args` when `TERMUX_EXEC__PROC_SELF_EXE` is set
- Keepalive defaults: `ServerAliveInterval=30` (was 0), configurable via `~/.tssh.conf` `defaultServerAliveInterval`. Set `#!! ServerAliveInterval 0` in `~/.ssh/config` to disable.
- `~/.tssh.conf` settings: `promptThemeLayout=tiny|simple|table`, `promptDefaultMode=search`, `promptPageSize=N`, `promptDetailItems=...`, `defaultServerAliveInterval=N`
- FRP (Fast Reverse Proxy) integration: `frp/` directory is gitignored; `Makefile` builds frps/frpc if source present, otherwise `make download-frp` fetches v0.69.1 binaries
- Debian packaging: `debian/` directory contains packaging files for .deb/.rpm builds via GoReleaser

## CI workflows (`.github/workflows/`)
| Workflow | Trigger |
|---|---|
| `gotest.yml` | push / PR (3 OS matrix + win7 compat build) |
| `pre-release.yml` | push to `main` (GoReleaser snapshot → "dev" release) |
| `publish.yml` | GitHub Release published (full multi-platform release) |

## Testing
- 13 test files in `tssh/`, 1 in `internal/table/`
- `-count=1` disables test caching; always run from repo root
- Integration-level tests depend on SSH server fixtures within the repo
- Run single test: `go test -v -count=1 -run TestName ./tssh`
- Run single package: `go test -v -count=1 ./internal/table`
- Known baseline failure: `TestTableExample` in `internal/table` fails on a
  color-capable environment (pre-existing, unrelated to Phase 0 changes)

## TRSS Phase 0 / integration
- Source design: `TRSS_SDD_v0.2.md`
- Audit: `docs/audit/phase-0-audit.md`
- ADRs: `docs/adr/README.md` (ADR-01 accepted; ADR-02 accepted for prototype)
- Shared contracts: `contracts/` (Go) and `contracts/ts/` (TypeScript)
- IPC transport: `internal/ipc/` (Unix socket, framing, server/client, tests)
- Supervisor: `internal/supervisor/` (backend, SSH runner, IPC service)
- Daemon: `cmd/trssd/` (`make trssd`)
- UI IPC client / TabStore: `prototype/terminal/src/ipc/`, `src/tabs/`
- Integration status: `docs/integration/status.md`
- Prototype: `prototype/terminal` (Bun 1.4.5 + `@opentui/core` 0.8.2, pinned)
- Evidence/results: `docs/phase-0/README.md`
- Toolchain used locally (not on PATH by default):
  `~/.local/go/bin/go` and `~/.bun/bin/bun`
- Prototype commands:
  ```
  cd prototype/terminal
  bun install
  bun run typecheck
  bun run typecheck:contracts
  bun test
  bun run smoke      # tmux + vim + htop + local sshd + resize
  ```
  or `make phase0` from the repo root (requires `bun` on `PATH`).
- Contract checks:
  ```
  CGO_ENABLED=0 go test -count=1 ./contracts ./internal/ipc ./internal/supervisor
  make contracts-check
  ```
- Supervisor build: `make trssd` -> `bin/trssd`.
- Supervisor run (Unix socket at `$XDG_RUNTIME_DIR/trss/control.sock`):
  `./bin/trssd --ssh-config ~/.ssh/config`
- H-01 is resolved (`generateToken` fails closed; see `tssh/frp_proxy_test.go`).
- H-02 is resolved (`ensurePrivateDir` forces FRP config/log dirs to `0700`).
- H-05 is resolved (remote FRP copy uses quoted `sh -c` arguments and tests).
- Phase 0 does **not** change runtime behavior of `tssh`; the runtime changes so
  far are the H-01 token fix, H-02 private FRP directories and H-05 remote-copy
  quoting. Respect the status in each ADR: ADR-01, ADR-02 (prototype) and
  ADR-03 (transport) are accepted; the rest remain open.

## Release
- GoReleaser multi-platform: linux, darwin, windows, android, freebsd; 386/amd64/arm/arm64/loong64
- Excludes: windows/arm, darwin/arm, android/arm, android/386, android/amd64, freebsd/arm, freebsd/386
- Win7 builds use fork `github.com/thongtech/go-legacy-win7`
