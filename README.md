## tssh — fork de trzsz-ssh v0.1.25

Cliente SSH con interfaz TUI ncurses, reemplazo directo de OpenSSH. Fork de [trzsz-ssh](https://github.com/trzsz/trzsz-ssh) con gestión visual de hosts, túneles y edición inline.

### Características principales

| Categoría | Features |
|---|---|
| **TUI Login** | Interfaz interactiva con búsqueda, filtro, selección múltiple, paginación, 3 temas (tiny/simple/table), colores personalizables, iconos, etiquetas de grupo |
| **Gestión de hosts** | Menu contextual (`←`): Editar, Túneles, Copiar, Favoritos, Forzar IPv4/IPv6, ver historial. Diálogo inline para editar alias/host/puerto/user/password |
| **Túneles SSH** | `←` → Tunnels → Manual o Automático. Escanea puertos (`ss` / `netstat` / Docker). Persistencia en `~/.tssh_tunnels`. Atajo `d` para borrar |
| **Batch Login** | Multi-select + `Ctrl+P` (paneles), `Ctrl+W` (ventanas), `Ctrl+T` (tabs) en tmux/iTerm2/Windows Terminal |
| **Modo UDP** | QUIC o KCP. Reconexión automática, sesiones adjuntables, roaming entre redes. Requiere [tsshd](https://github.com/trzsz/tsshd) en el servidor |
| **Transferencia archivos** | [trzsz](https://trzsz.github.io/) (trz/tsz), Zmodem (rz/sz), drag & drop, upload pre-login (`--upload-file`), instalación remota (`--install-trzsz`) |
| **Recordar contraseñas** | Cifrado AES via `--enc-secret`, soporte de `encPassword`/`encPassphrase`, respuestas para keyboard-interactive, TOTP, OTP command |
| **Password managers externos** | `PasswordCommand`, `PassphraseCommand`, etc. Compatible con gopass, pass, 1Password, Bitwarden, Vault, macOS Keychain |
| **Automated Interaction** | Expect-like: `ExpectPattern`, `ExpectSendPass`, `ExpectSendText`, TOTP, OTP. `CtrlExpect*` para ControlMaster |
| **Port Forwarding** | `-L`/`-R`/`-D` TCP y UDP, X11 forwarding, Agent forwarding, Gateway ports, stdio forward (`-W`) |
| **ProxyJump** | Multihop, también sobre UDP. Relay mode para trzsz en jump servers |
| **Keepalive** | ServerAliveInterval default 30s, configurable via `~/.tssh.conf` `defaultServerAliveInterval` |
| **Wayland integración** | `EnableWaypipe yes` — waypipe automático en background |
| **Clipboard OSC52** | `EnableOSC52 yes` — servidor escribe al portapapeles local |
| **Consola SSH** | Escape `~`: `.` kill, `^Z` suspend, `#` list forwards, `v`/`V` verbosity, `R` rekey, `C` redirect |
| **DNS SRV** | `DnsSrvName myhost.mydomain.com` para resolución via SRV records |
| **Reconnect** | `--reconnect` para reinicio automático tras caída. `-f` modo background. Adjuntar sesión con `--attach` |
| **Herramientas** | `--new-host` (alta GUI), `--list-hosts` (JSON), `--enc-secret` (cifrar), `--install-trzsz`/`--install-tsshd` (remote install) |
| **Temas** | `PromptThemeLayout: tiny/simple/table`, `PromptThemeColors` personalizable, `PromptCursorIcon`/`PromptSelectedIcon` emoji |
| **Autenticación** | Public key, password, keyboard-interactive, SSH agent, PKCS#11, Kerberos/GSSAPI, certificados OpenSSH |
| **i18n** | Español, inglés, chino. `language = english | chinese` en `~/.tssh.conf` |
| **Compatibilidad** | Drop-in de OpenSSH. Soporta `-o`, `-G`, `Include`, `Match`, `ControlMaster`, todos los flags estándar. `#!!` prefix para opciones tssh-only en `~/.ssh/config` |
| **Multiplataforma** | Linux, macOS, Windows (+ Win7), Android/Termux, FreeBSD. Binarios para 386/amd64/arm/arm64/loong64 |

### Navegación

| Tecla | Acción |
|---|---|
| `↑/↓` | Navegar lista de hosts |
| `←` | Menú contextual del host (Editar / Túneles / Eliminar) |
| `→` | Barra de acciones (New / Search / Select / Enter / Quit) |
| `Enter` | Conectar / Confirmar selección |
| `/` | Buscar / filtrar hosts |
| `?` | Ayuda de teclas |
| `Espacio` / `x` | Seleccionar/deseleccionar host (multi-select) |
| `a` / `o` | Seleccionar todos / invertir selección |
| `p` / `w` / `t` | Batch login: paneles / ventanas / tabs |
| `E` | Limpiar filtro de grupo |
| `n` | Nuevo host |
| `S` | Mostrar/ocultar hosts del sistema |
| `q` / `Ctrl+C` | Salir |

### Túneles SSH

- `←` sobre un host → **Tunnels** → menú Manual o Automático
- **Manual**: puerto remoto y local, lanza `ssh -NL` en segundo plano
- **Automático**: escanea puertos abiertos vía `ss`, seleccionas y creas túnel
- `d` sobre un túnel → modo borrado, `Enter` confirma
- Persistencia en `~/.ssh_tunnels`

### Edición inline

`←` → **Edit** abre formulario ncurses. Tab/Enter entre campos, Esc cierra. Guarda en `~/.ssh/config` y `~/.ssh/password`.

### Configuración rápida

```ini
# ~/.tssh.conf
ConfigPath = ~/.ssh/config
ExConfigPath = ~/.ssh/password
PromptThemeLayout = simple
PromptDefaultMode = search
PromptPageSize = 10
DefaultServerAliveInterval = 30
SetTerminalTitle = yes
DragFileUploadCommand = trz -y
DefaultDownloadPath = ~/Downloads
```

### Compilar

```sh
CGO_ENABLED=0 go build -o ./bin/ ./cmd/tssh
```

### Documentación integrada

Ejecuta `tssh --help-config` para ver la referencia completa de opciones de `~/.tssh.conf`.

Para documentación detallada de todas las features (trzsz, zmodem, UDP, expect, scp/sftp, remember passwords, external password managers, Wayland, clipboard, themes, etc.), visita [trzsz.github.io/tssh](https://trzsz.github.io/tssh) o lee [README.en.md](README.en.md).

### TRSS — Fase 0

El diseño de la evolución multipestaña TRSS vive en [TRSS_SDD_v0.2.md](TRSS_SDD_v0.2.md).
La Fase 0 (auditoría, ADRs y prototipo PTY/OpenTUI) está documentada en:

- [Auditoría del código actual](docs/audit/phase-0-audit.md)
- [ADRs de arquitectura](docs/adr/README.md)
- [Prototipo y resultados reproducibles](docs/phase-0/README.md)
- [Contratos UI ↔ supervisor](contracts/README.md)
- [Estado de integración del SDD](docs/integration/status.md)
- [Aceptación Fase 1 P0](docs/phase-1/acceptance.md)

```sh
cd prototype/terminal
bun install
bun run smoke        # tmux + vim + htop + SSH real + resize
```

Supervisor local:

```sh
CGO_ENABLED=0 go build -o ./bin/ ./cmd/trssd
./bin/trssd --ssh-config ~/.ssh/config
```

KeePass opcional:

```sh
./bin/trssd --ssh-config ~/.ssh/config \
  --keepass-db ~/db.kdbx --keepass-key-file ~/db.key
# o con askpass, sin persistir el secreto:
./bin/trssd --ssh-config ~/.ssh/config \
  --keepass-db ~/db.kdbx --keepass-password-command 'pass show keepass'
```

En la UI, `Ctrl+I` recarga el inventario KeePass. Los hosts KDBX-only se
resuelven a `usuario@host:puerto`; los de `ssh_config` conservan su alias.

FRP local (frpc) contra un frps autorizado:

```sh
./bin/trssd --ssh-config ~/.ssh/config \
  --frp-server frps.example.net --frp-port 7000 \
  --frpc-sha256 <sha256-del-binario-frpc>
```

Para levantar un `frps` local gestionado por el supervisor:

```sh
./bin/trssd --ssh-config ~/.ssh/config \
  --frps-listen 127.0.0.1:7000 --frpc /ruta/frpc --frps /ruta/frps
```

En la UI, `Ctrl+F` abre el panel FRP (endpoints, proxies, crear, parar,
iniciar/parar `frps` con `s`, rotar token). El smoke real `make trssd-frp-smoke` cubre TCP, UDP, STCP y XTCP.
La rotación reinicia el `frps` local antes que los `frpc`; para un `frps` remoto se rechaza para no exponer el
token: actualízalo fuera de banda y reinicia con `--frp-token ... --frp-token-force`.

Atajos configurables en `~/.config/trss/keybindings.json` (o
`TRSS_KEYBINDINGS`); los valores ausentes conservan el atajo por defecto y
`null` lo desactiva:

```json
{"palette": "ctrl+space", "frp": "ctrl+shift+f", "favorite": "f6", "quit": "ctrl+q"}
```

La paleta (`Ctrl+K`) incluye «Diagnosticar host» (DNS, TCP, banner y latencia)
y «Exportar diagnóstico saneado», que escribe un
JSON sin secretos en `~/.local/state/trss/diagnostics/`, y «Probar ruta del host»: informa `reachable`,
`refused`, `timeout`, `dns_failure` o `unknown` sin interpretar un timeout como
puerto cerrado.

Metadatos de host en `~/.ssh/config` (siempre con `#!!`):

```sshconfig
Host lab
    HostName 10.0.0.5
    #!! GroupLabels lab produccion
    #!! Favorite Yes
    #!! Color 5878CC
```

Los colores usan RGB cuando la terminal lo soporta y degradan a 256/16
colores; `NO_COLOR=1` o `TRSS_THEME=mono` desactivan el color.

Línea base de rendimiento: `make bench` (ver `docs/performance.md`).

El supervisor permite acotar `--max-sessions` y `--max-pending-bytes`, y la
auditoría retiene `--audit-max-files` archivos rotados.

La paleta incluye «Escanear con Nmap» (optativo, con confirmación y solo en
hosts del inventario) e «Información de red» (ASN por Team Cymru, RDAP y país
orientativo con procedencia y precisión limitada).

La paleta «Transferencias» permite subir/descargar con SCP o SFTP usando el
mismo `ssh_config` y agente que las sesiones. Requiere autenticación no
interactiva (`BatchMode`); no se ejecuta ninguna shell. Trzsz/Zmodem queda
pendiente de compatibilidad remota.

La paleta «Preferencias» ajusta ancho del panel, densidad y posición de las
pestañas; `Ctrl+Shift+←/→` reordena la pestaña activa. Las preferencias se guardan en
`~/.config/trss/preferences.json` (`TRSS_PREFERENCES`).

Desde la lista de hosts, `F2` marca favorito, `F3` cicla el color y `F4` edita
etiquetas. Los cambios se guardan en `~/.config/trss/hosts.json` sin tocar
`~/.ssh/config` (`TRSS_HOST_OVERRIDES` permite otra ruta).

Si `SSH_AUTH_SOCK` apunta a un agente (OpenSSH o KeePassXC), `trssd` lo usa
para autenticar y solo expone el proveedor y el número de identidades; nunca
extrae claves privadas del agente.

El vault KDBX se bloquea tras `--vault-idle-timeout` (15m por defecto) y los
hosts de KeePass dejan de listarse hasta un nuevo `hosts.import` o
`vault.unlockRequest`.

Reconexión automática configurable:

```sh
./bin/trssd --ssh-config ~/.ssh/config --reconnect-max 3 --reconnect-delay 500ms
```

Auditoría saneada (sin secretos, con rotación):

```sh
./bin/trssd --audit-log ~/.local/state/trss/audit.log --audit-max-bytes 10485760
```

UI OpenTUI (en otra terminal):

```sh
cd prototype/terminal
bun install
bun run ui
```

`Ctrl+K` abre la paleta de comandos. `Ctrl+G` activa tmux remoto antes de abrir
un host. La barra de estado muestra la ruta elegida (directa/ProxyJump). Cerrar la pestaña no
destruye la sesión tmux; al reabrir el mismo host se reengancha.

El workspace se guarda en `~/.config/trss/workspaces/<nombre>.json` (sin
secretos) y se restaura como pestañas **pendientes**: Enter reconecta, no se
simula una sesión viva. `Ctrl+P` abre el selector y `n` crea un workspace. Al
cambiar de workspace se reutilizan las sesiones vivas, sin abrir SSH duplicado.
Usa `--workspace <nombre>`, `TRSS_WORKSPACE_NAME` o `TRSS_WORKSPACE_DIR` para
aislar el archivo en pruebas.

Smoke completo (sshd local + trssd + 3 pestañas SSH + resize + tmux + restore +
10 pestañas con overflow):

```sh
make ui-smoke
```
