# Plan: TUI ncurses completo

## ✅ Ya implementado en el TUI ncurses

### Action bar (botones)
- [New] — formulario nuevo host inline
- [Search] — filtro de hosts
- [Select] — multi-select para batch login
- [Enter] / [Win] [Tab] [Pane] — login individual o batch
- [Config] — editor de `~/.tssh.conf`
- [Quit] — salir

### Context menu (← sobre host)
- **Edit** — formulario inline: Alias, HostName, Port, User, Password, PasswordCommand, PassphraseCommand, IdentityFile, ProxyJump, RemoteCommand, Group
- **Tunnels** — gestor de túneles SSH (manual/auto, scan, Docker, persistencia) con soporte L/R/D
- **Download** — formulario: paths remotos + ruta local, ejecuta `ssh05 -t --client --download-path ...`
- **Upload** — formulario: paths locales + ruta remota, ejecuta `ssh05 --upload-file ...`
- **Install trzsz** — instala trzsz en el host remoto
- **Install tsshd** — instala tsshd en el host remoto
- **Delete** — confirmación y eliminación del host

### Config form (11 opciones)
| Opción | Descripción |
|---|---|
| `ServerAliveInterval` | Keepalive en segundos |
| `SetTerminalTitle` | Auto-set titulo terminal |
| `DragFileUploadCommand` | Comando drag & drop |
| `DefaultDownloadPath` | Ruta descarga por defecto |
| `PromptThemeLayout` | Tema (tiny/simple/table) |
| `PromptDefaultMode` | Modo inicial (search/normal) |
| `PromptPageSize` | Registros por página |
| `Language` | Idioma (english/chinese) |
| `ConfigPath` | Ruta config SSH |
| `ExConfigPath` | Ruta config extendida |
| `UseOpenSSHConfig` | Usar `ssh -G` para effective config |

### Otras features TUI
- Búsqueda/filtro con keywords
- Group labels filtering
- Multi-select + batch login (tmux/iTerm2/WT)
- Temas con colores personalizables
- Paginación (`PromptPageSize`)
- Panel de detalle del host
- Favoritos / last login tracking
- Show/hide system hosts
- Ayuda con tecla `?`
- Túneles: creación manual, auto-scan (ss/netstat), Docker, persistencia JSON, L/R/D
- Upload file desde context menu
- Install trzsz/tsshd desde context menu
- Password managers (PasswordCommand, PassphraseCommand) en Edit form
- ScrollOffset + availableHeight seguro

---

## ❌ Pendiente de implementar en el TUI

### 1. Más campos en Edit form

Actualmente edita: Alias, HostName, Port, User, Password, IdentityFile, ProxyJump, RemoteCommand, Group.
Faltan campos por host:

| Campo | Prioridad |
|---|---|
| `UdpMode` (yes/QUIC/KCP/no) | Media |
| `EnableTrzsz` (yes/no) | Media |
| `EnableZmodem` (yes/no) | Media |
| `EnableDragFile` (yes/no) | Media |
| `ForwardAgent` (yes/no) | Media |
| `TotpSecret1..N` / `OtpCommand1..N` | Media |
| `ForwardX11` (yes/no) | Baja |
| `ConnectTimeout` | Baja |
| `ServerAliveCountMax` | Baja |
| `DnsSrvName` | Baja |
| `HideHost` | Baja |
| `ConsoleEscapeTime` | Baja |
| `EnableWaypipe` | Baja |
| `EnableOSC52` | Baja |
| `Compression` | Baja |

### 2. Más opciones en Config form

Actualmente cubre 11 de ~17 opciones.

| Opción | Prioridad |
|---|---|
| `DefaultUploadPath` | Media |
| `ProgressColorPair` | Media |
| `PromptDetailItems` | Media |
| `PromptCursorIcon` | Baja |
| `PromptSelectedIcon` | Baja |

### 3. Password managers externos

Formulario para configurar `PasswordCommand`, `PassphraseCommand`, etc. con
soporte de tokens `%n`, `%h`, `%r`, `%p`.

### 4. Install tools desde el TUI

Botón o acción que ejecute:
- `--install-trzsz` en el host seleccionado
- `--install-tsshd` en el host seleccionado

### 5. UDP mode global / por host

- Config global: UdpMode, TsshdPath, TsshdPort, UdpAliveTimeout, etc.
- Por host: toggle UDP/KCP/QUIC desde Edit form

### 6. Expect automation editor

Formulario para configurar:
- `ExpectCount`, `ExpectTimeout`
- `ExpectPattern1..N`, `ExpectSendPass1..N`, `ExpectSendText1..N`
- `ExpectSendTotp1..N`, `ExpectSendOtp1..N`
- `CtrlExpect*`

### 7. Reconnect / Debug / TraceLog desde TUI

Opciones globales en Config form:
- `--reconnect` al iniciar sesión
- `--debug` mode
- `--tracelog` mode

### 8. Opciones de ~/.ssh/password (exConfig)

Actualmente el TUI solo escribe `encPassword` al crear/editar hosts.
Falta editor para:
- `encPassphrase` / `Passphrase`
- `encQuestionAnswer1..N` / `QuestionAnswer1..N`
- `encTotpSecret1..N` / `TotpSecret1..N`
- `encOtpCommand1..N` / `OtpCommand1..N`

---

## Resumen archivos

| Archivo | Rol |
|---|---|
| `ssh05/host_model.go` | TUI principal (~3077 lines) |
| `ssh05/args.go` | CLI flags (180 lines) |
| `ssh05/config.go` | Config system (~931 lines) |
| `ssh05/tools.go` | Tools: enc-secret, list-hosts |
| `ssh05/tools_new_host.go` | New host CLI flow |
| `ssh05/tools_upload.go` | trz upload exec |
| `ssh05/trzsz.go` | trzsz/zmodem filter |
| `ssh05/udp.go` | UDP/KCP connection |
| `ssh05/dns.go` | Custom DNS |
| `ssh05/waypipe.go` | Wayland integration |
| `ssh05/console.go` | SSH escape console |
| `ssh05/expect.go` | Expect automation |
| `ssh05/otp.go` | TOTP/OTP auth |
| `ssh05/forward*.go` | Port forwarding |
| `ssh05/agent.go` | SSH agent |
| `ssh05/krb5.go` | Kerberos auth |
| `ssh05/algos.go` | Algorithm config |
| `ssh05/auth_method.go` | Auth methods |
| `ssh05/known_hosts.go` | Known hosts |
| `ssh05/lang.go` | i18n |
| `ssh05/login.go` | SSH login logic |

---

## Prioridades

1. ✅ **Alta** — Ampliar Edit form (IdentityFile, ProxyJump, RemoteCommand, GroupLabels)
2. ✅ **Alta** — Ampliar Config form (ConfigPath, ExConfigPath, UseOpenSSHConfig)
3. ✅ **Alta** — Upload file en context menu
4. ✅ **Media** — Port forwarding avanzado (L/R/D)
5. ✅ **Media** — Password managers externos
6. ✅ **Media** — Install tools desde TUI
7. **Baja** — UDP por host, Expect editor, Debug/Reconnect toggles
8. **Baja** — exConfig editor completo
