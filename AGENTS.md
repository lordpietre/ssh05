# ssh05 — Agent Guide

## Project
Drop-in OpenSSH client replacement with TUI login prompt, trzsz/zmodem file transfer, UDP mode (QUIC/KCP), and batch login. Go module `github.com/lordpietre/ssh05`.

Vendor: **Dinámicas Electrónicas** · Soporte: `dinamicas.electronicas@gmail.com`.
La identidad pública es `ssh05`; el supervisor es `ssh05d` y la UI vive en
`prototype/terminal/src/ssh05.ts`. Los nombres `TRSS_*` y `~/.config/trss` se
aceptan solo como compatibilidad heredada.

## Key commands
```
go build -o ./bin/ ./cmd/ssh05
go test -v -count=1 ./ssh05    # must run from repo root
make test                      # same as above (prefers `gotest` on unix)
make bench                     # benchmarks de IPC, backend y FRP
make trzsz-smoke               # descarga trzsz real vía supervisor (sshd de pruebas)
make ssh05d                    # supervisor local -> ./bin/ssh05d
make verify                    # tests, contratos y smoke completo P1/P2
make deb                       # .deb para Ubuntu y Debian (amd64/arm64) en dist/
make install                   # copies ./bin/ssh05 y ./bin/ssh05d to /usr/bin
```

## Toolchain
- **Go 1.25** required (`go.mod` declares `go 1.25.0`)
- No linter or typecheck config exists (no `.golangci.yml`, no `go vet` in CI)
- CGO must be **disabled** for all builds (`CGO_ENABLED=0`)
- All forked dependencies live under `github.com/trzsz/` (go-arg, ssh_config, promptui, quic-go, kcp-go, smux, etc.)

## Entrypoints
- **Binary**: `cmd/ssh05/main.go` → `ssh05.Ssh05Main(argv)` in `ssh05/main.go`
- **Library**: `ssh05.SshLogin(args)` in `ssh05/ssh.go`
- **Internal packages**: `internal/table/`, `internal/krb5/`, `internal/ssh/`

## Architecture notes
- All business logic lives in `ssh05/` (70 `.go` files). Main package is thin.
- Version constant `kSSH05Version` in `ssh05/version.go:36`
- Exit codes defined in `ssh05/comm.go:50-70` (e.g. `kExitCodeLoginFailed=16`)
- `#!!` prefix enables ssh05-specific options in `~/.ssh/config` (standard ssh treats it as comment)
- `enc` prefix on config values means encrypted with `ssh05 --enc-secret`
- Android/Termux quirk: `cmd/ssh05/main.go` adjusts `os.Args` when `TERMUX_EXEC__PROC_SELF_EXE` is set
- Keepalive defaults: `ServerAliveInterval=30` (was 0), configurable via `~/.tssh.conf` `defaultServerAliveInterval`. Set `#!! ServerAliveInterval 0` in `~/.ssh/config` to disable.
- tmux en restauración/reconexión es solo `attach`; si la sesión no existe se abre
  una shell y se elimina el binding (nunca se recrea en silencio).
- `TSSH_DISABLE_DOCKER=1` o `SSH05_DISABLE_DOCKER=1` evita descubrir contenedores Docker.
- Compatibilidad: se mantienen `~/.tssh.conf`, `TSSH_*` y `~/.tssh/` aunque el binario sea `ssh05`.
- `SshLogin` comparte config: init serializado, conexiones de la misma config en paralelo
  (`acquireConfigRead`); los defaults de `ssh_config` y UDP tienen mutex propio.
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
- 13 test files in `ssh05/`, 1 in `internal/table/`
- `-count=1` disables test caching; always run from repo root
- Integration-level tests depend on SSH server fixtures within the repo
- Run single test: `go test -v -count=1 -run TestName ./ssh05`
- Run single package: `go test -v -count=1 ./internal/table`
- Known baseline failure: `TestTableExample` in `internal/table` fails on a
  color-capable environment (pre-existing, unrelated to Phase 0 changes)

## ssh05 Phase 0 / integration
- Source design: `SSH05_SDD_v0.3.md`
- Audit: `docs/audit/phase-0-audit.md`
- ADRs: `docs/adr/README.md` (ADR-01 accepted; ADR-02 accepted for prototype)
- Shared contracts: `contracts/` (Go) and `contracts/ts/` (TypeScript)
- IPC transport: `internal/ipc/` (Unix socket, framing, server/client, tests)
- Supervisor: `internal/supervisor/` (backend, SSH runner, IPC service)
- Daemon: `cmd/ssh05d/` (`make ssh05d`)
- UI IPC client / TabStore: `prototype/terminal/src/ipc/`, `src/tabs/`
- UI OpenTUI: `prototype/terminal/src/ssh05.ts` (`bun run ui`). El `.deb`
  instala `ssh05-ui` con su runtime Bun y dependencias, y arranca `ssh05d` si
  el socket no existe.
  - `Ctrl+G` activa/desactiva tmux remoto antes de abrir un host.
  - `Ctrl+P` abre el selector de workspaces; `n` crea uno nuevo.
  - Barra de acciones visible (`Menú`, `Hosts`, `Nuevo`, `WS`, `Rutas`,
    `Diag`, `Transferir`, `FRP`, `Prefs`, `Ayuda`, `Salir`). `Tab` la enfoca,
    `←/→` navega, `Enter` activa, `Esc` sale; también acepta ratón.
  - `Menú` abre la lista completa de acciones navegable y clicable.
  - Menús, submenús, listas y formularios se navegan con `↑↓←→` (`←/↑`
    anterior, `→/↓` siguiente); `Espacio` cambia opciones y `Enter` acepta.
  - `Ctrl+K` abre la paleta de comandos; incluye diagnosticar host y exportar
    un informe saneado.
  - `Ctrl+N` en la lista de hosts abre el alta de un host y lo guarda en
    `ssh_config`; Enter guarda y conecta.
  - `--workspace <nombre>` o `SSH05_WORKSPACE_NAME`; `SSH05_WORKSPACE_DIR`.
  - `Ctrl+I` importa/recarga KeePass si `ssh05d` tiene KDBX configurado.
  `F1` ayuda con todos los atajos.
  `F2` favorito, `F3` color, `F4` etiquetas; overrides en
  `~/.config/ssh05/hosts.json` (`SSH05_HOST_OVERRIDES`).
  Atajos configurables (incl. `favorite`, `color`, `labels`) en
  `~/.config/ssh05/keybindings.json`.
  Preferencias de ancho/densidad/pestañas en `~/.config/ssh05/preferences.json`
  (`SSH05_PREFERENCES`); se editan desde la paleta.
  - Barra de pestañas con overflow (`‹N`/`N›`), activa visible y
    `Ctrl+Shift+←/→` para reordenar.
  - Tras caer la UI, `sessions.list`/`sessions.attach` recuperan sesiones vivas.
  - Workspace en `~/.config/ssh05/workspace.json` (`SSH05_WORKSPACE` override).
  - Al reiniciar, las pestañas quedan pendientes; Enter reconecta y reengancha
    tmux si corresponde.
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
  bun run ui         # OpenTUI UI against a running ./bin/ssh05d
  ```
  or `make phase0` from the repo root (requires `bun` on `PATH`).
- UI smoke (starts sshd/ssh05d automatically): `make ui-smoke` (3 sesiones,
  tmux/reattach, restore de workspace y 10 pestañas con overflow).
- Fase 1 P0: `make phase1-smoke` (host key estricta + UI smoke).
- P1 completa: `make p1-smoke` (host key + UI + FRP real del supervisor).
- FRP real supervisor: `make ssh05d-frp-smoke`.
- FRP smoke (requiere `make download-frp`): `make frp-smoke`.
- Contract checks:
  ```
  CGO_ENABLED=0 go test -count=1 ./contracts ./internal/ipc ./internal/supervisor
  make contracts-check
  ```
- Supervisor build: `make ssh05d` -> `bin/ssh05d`.
- Supervisor run (Unix socket at `$XDG_RUNTIME_DIR/ssh05/control.sock`):
  `./bin/ssh05d --ssh-config ~/.ssh/config`
- Reconexión configurable: `--reconnect-max N --reconnect-delay 500ms`.
- Auditoría saneada: `--audit-log /ruta/audit.log --audit-max-bytes N --audit-max-files N`.
- Límites: `--max-sessions N`, `--max-pending-bytes N`.
- Overrides de hosts: `--hosts-overrides /ruta/hosts.json`.
- Transferencias: `--scp scp --sftp sftp`; panel «Transferencias» en la paleta.
  `--trzsz` (por defecto) y `--zmodem` habilitan `trz`/`tsz` y `rz`/`sz` dentro
  de las sesiones; `--download-path` fija el destino (por defecto
  `~/Downloads` si existe). Smoke real: `make trzsz-smoke`.
- Diagnóstico de red: `--nmap nmap`; Nmap exige confirmación explícita.
- Supervisor: `--docker` incluye contenedores; por defecto se ocultan. Los `.deb`
  instalan `usr/lib/systemd/user/ssh05d.service`.
  ASN/RDAP usan Team Cymru DNS y `rdap.org`.
- FRP: `--frp-server`, `--frp-port`, `--frp-token`, `--frpc`, `--frpc-sha256`, `--frps-listen`, `--frps`, `--frps-sha256`, `--frp-token-force`, `--frp-config`; panel `Ctrl+F` (`s` inicia/para frps local).
- KeePass opcional:
  `./bin/ssh05d --ssh-config ~/.ssh/config --keepass-db ~/db.kdbx --keepass-key-file ~/db.key`
  o `--keepass-password-command 'pass show keepass'` (askpass; el secreto
  no se persiste). `--vault-idle-timeout 15m` bloquea el vault tras
  inactividad y exige reimportar; `vault.unlockRequest` lo reabre.
- H-01 is resolved (`generateToken` fails closed; see `ssh05/frp_proxy_test.go`).
- H-02 is resolved (`ensurePrivateDir` forces FRP config/log dirs to `0700`).
- H-03 is resolved: FRP tokens are AES-GCM encrypted at rest with a local
  `~/.config/ssh05/frp.key` (`0600`); remote TOML files are copied with `0600`.
- H-04 is resolved: tcp/udp/stcp/xtcp, dedicated STCP/XTCP secret, local/remote
  version gating, `frpc verify` for all four types and real TCP/STCP/XTCP/UDP
  tunnels.
- H-05 is resolved (remote FRP copy uses quoted `sh -c` arguments and tests).
- Phase 0 does **not** change runtime behavior of `ssh05`; the runtime changes so
  far are the H-01 token fix, H-02 private FRP directories, H-03 encrypted FRP
  tokens, H-04 proxy types and H-05 remote-copy quoting. Respect the status in
  each ADR: ADR-01, ADR-02 (prototype) and ADR-03 are accepted; the rest remain
  open.

## Release
- GoReleaser multi-platform: linux, darwin, windows, android, freebsd; 386/amd64/arm/arm64/loong64
- Excludes: windows/arm, darwin/arm, android/arm, android/386, android/amd64, freebsd/arm, freebsd/386
- Win7 builds use fork `github.com/thongtech/go-legacy-win7`
