# ssh05

Cliente SSH de escritorio y terminal, compatible con OpenSSH, con gestor visual de
hosts, supervisor local (`ssh05d`) y frontend OpenTUI. Fork de
[trzsz-ssh](https://github.com/trzsz/trzsz-ssh) orientado a escritorios Linux.

**Versión:** `0.42.4` · **Licencia:** MIT · **Desarrollado por Dinámicas Electrónicas**
Soporte: `dinamicas.electronicas@gmail.com` · [Repositorio](https://github.com/lordpietre/ssh05)

> Identidad actual: binario `ssh05`, supervisor `ssh05d`, UI `ssh05-ui`.
> Los nombres heredados `TRSS_*`, `~/.config/trss`, `~/.local/state/trss` y
> `make trssd` se aceptan como compatibilidad y se migran cuando es posible.

---

## Índice

1. [Qué hace](#qué-hace)
2. [Arquitectura](#arquitectura)
3. [Tecnologías](#tecnologías)
4. [Estructura del repositorio](#estructura-del-repositorio)
5. [Métodos y contratos](#métodos-y-contratos)
6. [Instalación](#instalación)
7. [Verificación de la instalación](#verificación-de-la-instalación)
8. [Uso](#uso)
9. [Configuración y archivos](#configuración-y-archivos)
10. [Seguridad](#seguridad)
11. [Desarrollo y pruebas](#desarrollo-y-pruebas)
12. [Documentación](#documentación)

---

## Qué hace

`ssh05` sustituye a `ssh` en el uso diario y añade una capa de producto:

- **Gestor de hosts** con búsqueda, filtros, favoritos, colores, etiquetas y
  edición inline sobre `~/.ssh/config`.
- **UI multipestaña** (OpenTUI) con terminales embebidas, barra de acciones,
  paleta de comandos, workspaces persistentes y reconexión.
- **Supervisor local** que mantiene sesiones y túneles aunque se cierre la UI.
- **Transferencias** trzsz (`trz`/`tsz`), Zmodem (`rz`/`sz`), SCP y SFTP.
- **Túneles FRP** (TCP/UDP/STCP/XTCP) gestionados desde la UI.
- **Diagnóstico de red**: DNS, TCP, banner, TLS, latencia, ASN/RDAP y
  descubrimiento de la red local con Nmap embebido.
- **Compatibilidad OpenSSH**: flags estándar, `Include`, `Match`, `ProxyJump`,
  `ControlMaster`, `-L/-R/-D`, X11, agente SSH y certificados.
- **Transporte UDP opcional** tipo Mosh con QUIC o KCP, mediante `tsshd` en el
  servidor.

### Capacidades principales

| Área | Detalle |
|---|---|
| TUI clásica | Búsqueda, selección múltiple, paginación, temas `tiny`/`simple`/`table`, iconos y colores |
| OpenTUI | Pestañas con overflow, panel de hosts, terminal embebida, paleta `Ctrl+K`, ayuda `F1` |
| Hosts | Alta, edición, renombrado, borrado, favoritos, colores, etiquetas y Docker optativo |
| Sesiones | PTY, resize, señales, cierre por vista/SSH/tmux, reenganche y reconexión con backoff |
| Persistencia | Workspaces sin secretos; al reiniciar las pestañas quedan pendientes y reconectan con Enter |
| tmux | Creación opcional y `attach` si la sesión existe; nunca se recrea en silencio |
| Secretos | Contraseñas `enc`, `PasswordCommand`, `PassphraseCommand`, TOTP/OTP y gestores externos |
| Agente SSH | Usa `SSH_AUTH_SOCK`; solo expone proveedor y número de identidades |
| KeePass | Importación KDBX read-only mediante KeePassXC CLI, con bloqueo por inactividad |
| FRP | Endpoints y proxies con estado, PID, uptime, token cifrado y rotación coordinada |
| Transferencias | En sesión (trzsz/Zmodem) y fuera de sesión (SCP/SFTP), sin shell intermedia |
| Diagnóstico | Informe saneado, comprobación de host y exportación JSON sin secretos |
| Auditoría | JSON lines con rotación; sin contraseñas, tokens ni bytes de PTY |
| Plataformas | Linux, macOS, Windows/Win7, Android/Termux y FreeBSD; `386`/`amd64`/`arm`/`arm64`/`loong64` |

---

## Arquitectura

```text
┌──────────────────────────────┐        ┌──────────────────────────────────┐
│ ssh05 (CLI / TUI clásica)    │        │ ssh05-ui (Bun + OpenTUI)         │
│ args → config → hosts → SSH  │        │ hosts · pestañas · paleta        │
└──────────────┬───────────────┘        └───────────────┬──────────────────┘
               │                                        │ IPC local tipado
               │                                        │ Unix socket
               │                        ┌───────────────▼──────────────────┐
               │                        │ ssh05d                           │
               │                        │ Service · Backend · SSHRunner    │
               │                        │ Inventory · FRP · Transfers      │
               │                        │ Audit · Diagnostics              │
               │                        └───────────────┬──────────────────┘
               │                                        │
               └──────────────► paquete ssh05 ◄─────────┘
                     SshLogin · config · auth · UDP · trzsz
                                        │
                                        ▼
                              servidor SSH / tsshd / tmux / FRP
```

### Modos de funcionamiento

1. **CLI clásica (`ssh05 destino`)**
   `Ssh05Main` parsea los argumentos, inicializa la configuración, elige el
   host (o abre la TUI), resuelve alias y `ProxyJump`, y conecta con el motor
   SSH interno. No requiere `ssh05d`.

2. **UI por defecto (`ssh05` sin argumentos)**
   Si hay TTY y `ssh05-ui` está instalado, `ssh05` hace `exec` del frontend.
   `SSH05_NO_UI=1` fuerza la TUI clásica. La UI no abre SSH: todo pasa por el
   supervisor.

3. **Sesión gestionada por la UI**
   `hosts.list` → `routes.plan` → `sessions.open`. El supervisor crea la
   sesión, emite `session.status` y transmite la salida PTY por el canal
   binario. La UI envía entrada con `sessions.write` y el tamaño con
   `sessions.resize`. Al cerrar la UI, `sessions.list`/`sessions.attach`
   permiten recuperar sesiones vivas.

4. **Supervisor desacoplado**
   `ssh05d` mantiene sesiones, reconexión, FRP, transferencias y auditoría.
   La UI puede caerse y volver sin matar los procesos gestionados. El socket
   se crea en `$XDG_RUNTIME_DIR/ssh05/control.sock`.

5. **tmux remoto**
   Al abrir con tmux se ejecuta `tmux new-session -A -s <nombre>` si se permite
   crear, o `tmux attach-session -t <nombre>` solo si `has-session` confirma
   que existe. En restauración/reconexión nunca se crea una sesión nueva.

6. **Transferencias dentro de la sesión**
   El runner envuelve el stream PTY con `trzsz-go`. El filtro detecta
   `trz`/`tsz` (y `rz`/`sz` si `--zmodem`) y realiza la transferencia sobre el
   mismo stream; el resto de bytes pasa sin cambios.

7. **Transferencias SCP/SFTP**
   `internal/transfer` ejecuta los binarios `scp`/`sftp` con `-S ssh05`, por lo
   que reutilizan `ssh_config`, el agente SSH, `encPassword` y el resto de la
   autenticación de ssh05. No interpolan una shell; el binario de transporte se
   puede cambiar con `ssh05d --ssh05 <ruta>`.

8. **FRP**
   `internal/frp` es dueño del ciclo de vida de `frpc`/`frps`: valida binarios
   por SHA-256, cifra tokens AES-GCM en reposo, mantiene grupos de proceso y
   expone endpoints/proxies por IPC. La rotación reinicia primero el `frps`
   local; para un `frps` remoto se rechaza y se hace fuera de banda.

---

## Tecnologías

| Capa | Tecnología |
|---|---|
| Lenguaje principal | **Go 1.25** (`go 1.25.0`), compilación con `CGO_ENABLED=0` |
| Motor SSH | `golang.org/x/crypto/ssh`, `github.com/skeema/knownhosts`, `ssh_config` |
| TUI clásica | **Bubble Tea v2**, **Lipgloss v2**, **Bubbles v2**, `go-runewidth` |
| UI moderna | **OpenTUI 0.8.2**, **Bun 1.4.5**, **TypeScript 5.9.3** |
| IPC | Unix domain socket, frames JSON + binarios, `SO_PEERCRED` en Linux |
| Transferencias | `trzsz-go`, `tsshd`, `scp`/`sftp` del sistema, `shellescape` |
| UDP | forks `trzsz/quic-go`, `trzsz/kcp-go`, `trzsz/smux` |
| Túneles | **FRP 0.69.1** (`frpc`/`frps`) |
| Red local | **Nmap 7.99** estático embebido, datos y scripts NSE incluidos |
| Secretos | AES-GCM (`enc`), bcrypt-pbkdf, `pquerna/otp`, `gokrb5` (GSSAPI) |
| Inventario | `~/.ssh/config`, KeePassXC CLI (`keepassxc-cli`), Docker optativo |
| Empaquetado | `.deb` Ubuntu/Debian, RPM, GoReleaser, systemd user unit |
| CI | GitHub Actions: tests multi-OS, snapshot `main`, release por tag |

Dependencias destacadas del módulo: `trzsz/go-arg`, `trzsz/ssh_config`,
`trzsz/promptui` (heredada), `trzsz/trzsz-go`, `trzsz/tsshd`, `creack/pty`,
`google/uuid`, `atotto/clipboard`, `ncruces/zenity`.

---

## Estructura del repositorio

```text
cmd/
  ssh05/            entrypoint fino → ssh05.Ssh05Main
  ssh05d/           daemon supervisor y flags
ssh05/              motor y TUI clásica (79 archivos .go)
  main.go           parseo, herramientas y arranque
  ssh.go            SshLogin, SshClient, SshSession
  config.go         ~/.tssh.conf, alias, defaults
  host_model.go     TUI Bubble Tea de hosts
  login.go          autenticación y diálogos
  forward*.go       -L / -R / -D, X11 y stdio
  udp.go            transporte QUIC/KCP con tsshd
  trzsz.go          filtro de transferencias
  frp_proxy.go      cliente FRP y tokens cifrados
  tmgr*.go          tmux, iTerm2 y Windows Terminal
internal/
  ipc/              transporte, framing, servidor, cliente y seguridad
  supervisor/       backend de sesiones, inventario, servicio IPC, FRP/transfer
  transfer/         SCP/SFTP tipado
  audit/            auditoría JSON saneada con rotación
  frp/              gestor de procesos y persistencia FRP
  identity/         nombres, versión, rutas XDG y migración
  ssh/              bcrypt_pbkdf
  krb5/             GSSAPI
  table/            tablas de la TUI clásica
contracts/
  *.go              tipos Go compartidos (dominio, sesiones, IPC)
  ts/index.ts       espejo TypeScript de los contratos
prototype/terminal/ UI OpenTUI (Bun)
  src/ssh05.ts      aplicación principal
  src/ipc/          cliente y framing
  src/tabs/         TabStore y barra de pestañas
  src/workspaces/   persistencia de workspaces
  src/preferences/  preferencias de UI
  src/config/       keybindings
  src/diagnostics/  exportación saneada
scripts/            smoke tests y build-deb.sh
packaging/          ssh05d.service y avisos de terceros
debian/             metadatos de empaquetado
docs/               ADRs, auditoría, fases, releases y rendimiento
```

---

## Métodos y contratos

### Contrato IPC

- **Transporte:** Unix domain socket (named pipe en Windows, fase posterior).
- **Directorio:** `0700`; socket `0600`.
- **Control:** frame `J` + longitud `uint32` + JSON, máximo **1 MiB**.
- **PTY:** frame `B` + longitud + `sessionId` + bytes, máximo **4 MiB**.
- **Versión:** `protocolVersion = 1`; métodos desconocidos se rechazan.
- **Autorización:** solo métodos declarados en `contracts.Method`; no existe
  `shell.exec`. En Linux se verifica el UID con `SO_PEERCRED`.
- **Correlación:** `requestId`; eventos ordenados por sesión con `seq`.

### Métodos IPC disponibles

| Grupo | Métodos | Función |
|---|---|---|
| Hosts | `hosts.list`, `hosts.import`, `hosts.resolve`, `hosts.overrides.set` | Inventario, importación KDBX, resolución y metadatos |
| Hosts CRUD | `hosts.create`, `hosts.update`, `hosts.delete` | Alta/edición/borrado atómico en `ssh_config` |
| Hosts host key | `hosts.trustKey` | Añade a `known_hosts` solo la huella aprobada por el usuario |
| Sesiones | `sessions.open`, `sessions.write`, `sessions.resize`, `sessions.close` | Ciclo PTY y cierre con intención |
| Sesiones | `sessions.signal`, `sessions.list`, `sessions.attach` | Señales, recuperación tras caer la UI |
| tmux | `sessions.attachTmux`, `sessions.detachTmux` | Reenganche y desenganche remoto |
| Rutas | `routes.plan`, `routes.connect`, `routes.probe` | Directa, ProxyJump o FRP; sondeo con latencia |
| FRP | `frp.instances.list`, `frp.proxies.apply`, `frp.proxies.stop`, `frp.proxies.remove` | Estado y ciclo de vida de proxies |
| FRP | `frp.tokens.rotate`, `frp.servers.start`, `frp.servers.stop` | Rotación y `frps` gestionado |
| Transferencias | `transfers.start`, `transfers.list`, `transfers.cancel` | SCP/SFTP tipado |
| Vault | `vault.status`, `vault.unlockRequest` | Estado KDBX y desbloqueo |
| Diagnóstico | `diagnostics.report`, `diagnostics.host`, `diagnostics.nmap`, `diagnostics.network` | Informe saneado, DNS/TCP/banner/TLS, red local con Nmap, ASN/RDAP |
| Workspaces | `workspaces.save`, `workspaces.restore` | Persistencia y plan de restauración |

### Interfaces Go internas

```go
// contracts/session.go (resumen)
type SessionBackend interface {
    Open(ctx context.Context, profileID string, routeID string, options SessionOptions) (SessionID, error)
    Write(ctx context.Context, id SessionID, data []byte) error
    Resize(ctx context.Context, id SessionID, cols, rows int) error
    RequestClose(ctx context.Context, id SessionID, intent CloseIntent) error
    Attach(ctx context.Context, id SessionID, cursor *uint64) error
    Signal(ctx context.Context, id SessionID, signal string) error
    OnData(id SessionID, cb DataHandler) (unsubscribe func())
    OnStatus(id SessionID, cb StatusHandler) (unsubscribe func())
    Capabilities(ctx context.Context) (Capabilities, error)
}

// internal/supervisor/runner.go
type Runner interface {
    Start(ctx context.Context, profileID string, cols, rows int,
        options contracts.SessionOptions) (RunSession, error)
}

// internal/supervisor/service.go (resumen)
type HostInventory interface {
    List(ctx context.Context) ([]contracts.HostProfile, error)
    Resolve(ctx context.Context, id string) (contracts.HostProfile, bool)
    Conflicts() []contracts.HostConflict
    VaultStatus() contracts.VaultStatusResult
    Import(ctx context.Context, source string, path string) (contracts.HostsImportResult, error)
    SetOverride(ctx context.Context, patch contracts.HostOverridePatch) (contracts.HostProfile, error)
    CreateHost(ctx context.Context, params contracts.HostCreateParams) (contracts.HostProfile, error)
    UpdateHost(ctx context.Context, params contracts.HostUpdateParams) (contracts.HostProfile, error)
    DeleteHost(ctx context.Context, params contracts.HostDeleteParams) error
}
```

`SSHRunner` implementa `Runner` sobre el paquete `ssh05`; `Backend`
implementa `SessionBackend` y aplica límites de sesiones, colas acotadas de
PTY, reconexión con backoff y eventos de estado.

### Herramientas de línea de comandos

| Flag | Uso |
|---|---|
| `--new-host` | Alta guiada de host |
| `--list-hosts` | Inventario en JSON |
| `--enc-secret` | Cifrar un secreto para la configuración |
| `--install-trzsz` / `--install-tsshd` | Instalación remota de herramientas |
| `--upload-file` / `--download-path` | Transferencia y destino |
| `--udp` / `--kcp` / `--attach` | Transporte UDP o reenganche |
| `--reconnect` | Reinicio automático tras caída |
| `--zmodem` | Habilitar `rz`/`sz` |
| `--help-config` | Referencia de `~/.tssh.conf` |

`ssh05 --help` muestra la lista completa de flags compatibles con OpenSSH.

---

## Instalación

### Paquete `.deb` (Ubuntu / Debian)

Descarga el paquete de tu arquitectura desde
<https://github.com/lordpietre/ssh05/releases/latest>:

```sh
VERSION=0.42.4
wget "https://github.com/lordpietre/ssh05/releases/download/v${VERSION}/ssh05_${VERSION}_ubuntu_amd64.deb"
wget "https://github.com/lordpietre/ssh05/releases/download/v${VERSION}/ssh05_${VERSION}_ubuntu_amd64.deb.sha256"
sha256sum -c "ssh05_${VERSION}_ubuntu_amd64.deb.sha256"
sudo apt install "./ssh05_${VERSION}_ubuntu_amd64.deb"
```

El paquete instala:

- `/usr/bin/ssh05` — cliente y TUI clásica.
- `/usr/bin/ssh05d` — supervisor local.
- `/usr/bin/ssh05-ui` — frontend OpenTUI.
- `/usr/lib/ssh05/bun` — runtime Bun incluido.
- `/usr/lib/ssh05/prototype/terminal/` — UI y dependencias de producción.
- `/usr/lib/systemd/user/ssh05d.service` — unidad de usuario.
- `/etc/systemd/user/default.target.wants/ssh05d.service` — arranque al iniciar
  sesión sin ejecutar `systemctl`.

Dependencias recomendadas: `tmux` (sesiones persistentes) y `zenity`
(selectores de archivo). Para FRP hacen falta `frpc`/`frps` (o
`make download-frp`); para KeePass, `keepassxc-cli`. Nmap va embebido; `--nmap`
permite usar un binario externo.

`ssh05-ui` arranca `ssh05d` automáticamente si el socket todavía no existe, así
que la primera ejecución tras instalar funciona sin pasos manuales.

### Actualizar y `apt upgrade`

`apt upgrade` solo actualiza paquetes que vienen de un repositorio APT. Si
instalaste ssh05 desde un `.deb` local, el paquete figura en `dpkg` pero no
tiene repositorio, así que `apt upgrade` no mostrará ninguna actualización de
ssh05. Para actualizar usa el paquete descargado:

```sh
sudo apt install --reinstall ./ssh05_0.42.4_ubuntu_amd64.deb
```

Si quieres que `apt upgrade` gestione ssh05 automáticamente, hace falta
publicar los `.deb` en un repositorio APT y añadirlo con `sources.list.d`;
eso no se configura desde el propio paquete.

### Compilar desde el código

```sh
CGO_ENABLED=0 go build -o ./bin/ ./cmd/ssh05
CGO_ENABLED=0 go build -o ./bin/ ./cmd/ssh05d
```

UI:

```sh
cd prototype/terminal
bun install
bun run typecheck
bun run ui
```

Paquete local:

```sh
make deb
sudo apt install ./dist/ssh05_0.42.4_ubuntu_amd64.deb
```

---

## Verificación de la instalación

Comprobaciones rápidas:

```sh
dpkg -l ssh05                 # paquete instalado y versión
dpkg -V ssh05                 # integridad de archivos (sin salida = correcto)
ssh05 --version               # cliente
ssh05d --version              # supervisor
systemctl --user is-enabled ssh05d
systemctl --user is-active ssh05d
ls -l "${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/ssh05/control.sock"
ssh05 --list-hosts            # inventario JSON desde ~/.ssh/config
```

Resultado esperado:

- `ssh05` y `ssh05d` muestran la misma versión (`0.42.4`).
- `ssh05d` está `enabled` y `active`; el socket existe con permisos `srw-------`.
- `ssh05 --list-hosts` devuelve un array JSON (aunque esté vacío).
- `ssh05-ui` en una terminal real dibuja la interfaz y conecta con el socket.

Si el supervisor no está activo:

```sh
systemctl --user daemon-reload
systemctl --user enable --now ssh05d
systemctl --user status ssh05d --no-pager
```

Si `ssh05` no encuentra la UI, comprueba `command -v ssh05-ui` y que
`/usr/lib/ssh05/bun` sea ejecutable. `SSH05_NO_UI=1 ssh05` permite usar la TUI
clásica como diagnóstico.

---

## Uso

```sh
ssh05                      # UI OpenTUI (si está instalada)
ssh05 usuario@host         # conexión directa con la CLI clásica
ssh05 host -p 2222         # flags OpenSSH habituales
SSH05_NO_UI=1 ssh05        # forzar la TUI clásica
ssh05 --workspace laboratorio
```

### Primera conexión y host keys

Cuando un host no está en `known_hosts`, la UI muestra un diálogo con el tipo
de clave y la huella SHA-256. Al pulsar **Aceptar y conectar**, la UI llama a
`hosts.trustKey` con la huella exacta que se mostró; el supervisor vuelve a
sondear el servidor, comprueba que coincide y solo entonces escribe la clave en
`known_hosts`. Después reintenta la conexión.

- El supervisor **nunca** depende de `/dev/tty`: si no hay una decisión
  explícita, devuelve el código IPC `host_key_unknown` con la huella.
- Con `StrictHostKeyChecking yes` no se ofrece confiar: el diálogo indica que
  hay que ajustar `ssh_config` o añadir la clave manualmente.
- Si la clave cambió, se devuelve `host_key_changed` y **nunca** se
  sobrescribe `known_hosts` automáticamente.
- La CLI clásica mantiene el prompt interactivo habitual cuando hay terminal.

### Ratón y ventana de sesión

La UI está pensada para alternar teclado y ratón sin perder el contexto:

- **Hosts:** un clic selecciona; doble clic conecta; clic derecho abre
  `Conectar · Editar · Eliminar`; la rueda desplaza la lista.
- **Pestañas:** un clic activa la pestaña; clic derecho o central abre el menú
  de cierre; la rueda cambia de pestaña; `‹N` y `N›` permiten recorrer las
  ocultas.
- **Barra superior:** cada botón se ejecuta con un clic. Los botones siguen
  funcionando con `Tab`, flechas y `Enter`.
- **Menús y paneles:** las filas se activan con un clic. Los formularios
  permiten clicar directamente en el campo. Los paneles de Transferencias, FRP,
  Preferencias, Nmap, Ayuda y Diagnóstico muestran botones `[ Acción ]`
  clicables.
- **Rueda:** desplaza la lista de hosts, el menú superior y los selectores
  largos.
- **Ventana SSH:** cada sesión se muestra dentro de un marco con cabecera de
  estado, host, ruta y tmux. La `×` de la esquina superior derecha cierra la
  conexión (`close-view`); con clic derecho en la `×` se abre el menú para
  elegir entre cerrar la conexión y la pestaña o desacoplar tmux.

En menús y formularios, `↑↓←→`, `Home`, `End`, `PageUp`, `PageDown`, `Enter`
y `Esc` siguen funcionando exactamente igual que antes.

### Escaneo de red local con Nmap

La paleta `Ctrl+K` o el menú `Menú` incluyen **Escanear red local (Nmap)**.
No escanea los puertos de un host: descubre hosts de la red privada principal
con `nmap -sn`.

El binario Nmap 7.99 y sus datos van embebidos en ssh05 para Linux `amd64` y
`arm64`; no hace falta instalar `nmap`. En otras plataformas se puede indicar
`ssh05d --nmap <ruta>`.

Opciones del panel:

| Tecla | Opción |
|---|---|
| `v` | Servicios y versiones (`-sV`) |
| `s` | Scripts NSE (`-sC`) |
| `o` | Sistema operativo (`-O`), requiere root |
| `t` | Trazado de ruta, requiere root |
| `p` | Todos los puertos (`-p-`) |
| `r` | Usar `sudo -n` para SO/trazado (sin pedir contraseña) |
| `f` | Activar todo |
| `Enter` | Escanear |
| `Esc` | Cancelar el escaneo |

La UI muestra IP, hostname, MAC, fabricante, tipo de máquina, SO, puertos,
servicios, versiones, scripts, uptime, distancia y traza. `D` abre el informe
completo y `A` añade el host seleccionado al inventario.

Sin root se sigue haciendo descubrimiento, servicios y scripts; la MAC y el
fabricante se completan desde la tabla ARP local. Para SO y trazado se puede
activar `sudo -n`, que falla de forma controlada si sudo pide contraseña.

El escaneo usa una conexión IPC separada, así que no bloquea la apertura de
sesiones ni el resto de la UI. Se puede cancelar con `Esc`.

### Enfoque inicial y navegación

Al cerrar la bienvenida con **Empezar**, la barra de menús queda enfocada:

- `←/→` navega por los botones del menú superior.
- `Enter` abre el submenú del botón seleccionado.
- `↓` o `Esc` baja a la lista de hosts.
- `Tab` desde hosts vuelve a la barra de menús.

Al cerrar cualquier menú, panel o diálogo, el foco vuelve a hosts para poder
escribir, buscar y usar atajos sin pasos extra.

### Atajos de la UI OpenTUI

| Tecla | Acción |
|---|---|
| `↑` / `↓` | Mover por la lista de hosts |
| `←` | Enfocar la barra superior de acciones |
| `→` | Abrir el menú del host (Conectar, Editar, Eliminar) |
| `Enter` | Conectar / confirmar |
| `/` | Búsqueda y comandos (`/connect`, `/conn`, `/new`, `/hosts`, `/frp`, `/transfers`, `/help`) |
| `Ctrl+N` | Nuevo host |
| `Ctrl+K` | Paleta de comandos |
| `Ctrl+P` | Selector de workspaces (`n` crea uno) |
| `Ctrl+G` | Activar/desactivar tmux remoto antes de conectar |
| `Ctrl+I` | Importar/recargar KeePass |
| `Ctrl+F` | Panel FRP |
| `Ctrl+Shift+←/→` | Reordenar la pestaña activa |
| `F1` | Ayuda con todos los atajos |
| `F2` / `F3` / `F4` | Favorito / color / etiquetas |
| `Tab` | Enfocar la barra de acciones |
| `Esc` | Cerrar overlay o volver |
| `F10` / `Ctrl+Q` | Salir |

La barra incluye `Menú`, `Hosts`, `Nuevo`, `WS`, `Rutas`, `Diag`, `Transferir`,
`FRP`, `Prefs`, `Ayuda` y `Salir`; se navega con flechas, `Enter`, `Tab` y
ratón.

### TUI clásica

| Tecla | Acción |
|---|---|
| `↑` / `↓` | Navegar hosts |
| `←` / `→` | Menú contextual del host |
| `Enter` | Conectar |
| `/` | Buscar |
| `?` | Ayuda |
| `Espacio` | Seleccionar host |
| `a` / `o` | Seleccionar todos / invertir |
| `p` / `w` / `t` | Batch: paneles / ventanas / pestañas |
| `n` | Nuevo host |
| `e` | Limpiar filtro |
| `S` | Mostrar hosts del sistema |
| `q` / `Ctrl+C` | Salir |

### Transferencias

```sh
ssh05 host --upload-file ./archivo          # subir antes/durante la conexión
ssh05 host --download-path ~/Descargas      # destino de descargas
ssh05 host --install-trzsz                  # instalar trz/tsz remoto
```

Dentro de una sesión, `trz`/`tsz` se detectan automáticamente. Zmodem requiere
`--zmodem` y `lrzsz` en el servidor. Las descargas usan `--download-path` o
`~/Downloads`; si no existe, se abre el selector del escritorio.

El panel **Transferencias** de la UI usa SCP/SFTP con `-S ssh05`, así que
funciona con claves, agente SSH y contraseñas `#!! encPassword` igual que una
sesión normal. Si una transferencia falla, se puede seleccionar y usar
**Reintentar** (`r`) sin reiniciar la app. El binario de transporte se puede
cambiar con `ssh05d --ssh05 <ruta>`.

### Túneles y FRP

La TUI clásica permite túneles manuales o automáticos desde el menú del host y
los persiste en `~/.tssh_tunnels`. La UI usa `Ctrl+F` para endpoints y proxies
FRP, con estado, PID, uptime y rotación de token.

---

## Configuración y archivos

### `~/.tssh.conf`

```ini
ConfigPath = ~/.ssh/config
ExConfigPath = ~/.ssh/password
PromptThemeLayout = simple          # tiny | simple | table
PromptDefaultMode = search
PromptPageSize = 10
DefaultServerAliveInterval = 30
SetTerminalTitle = yes
DragFileUploadCommand = trz -y
DefaultDownloadPath = ~/Downloads
Language = english                  # english | chinese
```

### `~/.ssh/config`

Las opciones propias se marcan con `#!!` para que OpenSSH las ignore:

```sshconfig
Host lab
    HostName 10.0.0.5
    User operador
    #!! GroupLabels laboratorio produccion
    #!! Favorite Yes
    #!! Color 5878CC
    #!! encPassword <secreto-cifrado>
```

### Archivos de estado

| Ruta | Contenido |
|---|---|
| `$XDG_RUNTIME_DIR/ssh05/control.sock` | Socket IPC (`0600`) |
| `~/.config/ssh05/hosts.json` | Overrides de favoritos, color y etiquetas |
| `~/.config/ssh05/keybindings.json` | Atajos configurables (`null` desactiva) |
| `~/.config/ssh05/preferences.json` | Ancho, densidad y posición de pestañas |
| `~/.config/ssh05/workspaces/<nombre>.json` | Pestañas y estado no secreto |
| `~/.config/ssh05/frp.json` / `frp.key` | Configuración FRP y clave AES-GCM (`0600`) |
| `~/.local/state/ssh05/audit.log` | Auditoría saneada con rotación |
| `~/.local/state/ssh05/diagnostics/` | Informes exportados sin secretos |
| `~/.tssh_tunnels` | Túneles de la TUI clásica |

### Variables de entorno

| Variable | Efecto |
|---|---|
| `SSH05_NO_UI=1` | Fuerza la TUI clásica |
| `SSH05_SOCKET` | Ruta alternativa del socket |
| `SSH05_SUPERVISOR_BIN` | Binario que arranca la UI si falta el socket |
| `SSH05_WORKSPACE_NAME` / `SSH05_WORKSPACE_DIR` / `SSH05_WORKSPACE` | Workspace activo, directorio o archivo |
| `SSH05_KEYBINDINGS` | Ruta alternativa de keybindings |
| `SSH05_PREFERENCES` | Ruta alternativa de preferencias |
| `SSH05_HOST_OVERRIDES` | Ruta alternativa de `hosts.json` |
| `SSH05_THEME` / `SSH05_COLOR_LEVEL` | Tema y nivel de color |
| `SSH05_NO_WELCOME=1` | Omite la pantalla de bienvenida |
| `SSH05_UI_DEBUG` | Log de depuración de la UI |
| `SSH05_DISABLE_DOCKER=1` | Oculta contenedores Docker |

Los nombres `TRSS_*` siguen aceptándose como respaldo.

---

## Seguridad

- **Sin secretos en claro:** los perfiles, eventos, logs y workspaces no
  contienen contraseñas, tokens ni claves privadas.
- **`enc`:** los secretos de configuración se cifran con AES-GCM y una clave
  local; `#!! encPassword` nunca se escribe en claro.
- **FRP:** tokens cifrados en reposo, `frp.key` con `0600`, binarios validados
  por SHA-256 y directorios forzados a `0700`.
- **IPC:** socket privado, `SO_PEERCRED` en Linux, frames acotados y lista
  blanca de métodos; no hay ejecución de shell arbitraria.
- **Host keys:** una clave nueva se acepta solo tras mostrar su huella y
  recibir una decisión explícita; una clave cambiada nunca se acepta ni se
  sobrescribe.
- **Supervisor no interactivo:** `ssh05d` no lee contraseñas, passphrases ni
  respuestas de `/dev/tty`; usa solo secretos configurados, comandos o el
  agente SSH. Así una sesión nunca queda bloqueada esperando una terminal.
- **Nmap:** el binario y sus datos se extraen con permisos `0700` y SHA-256
  verificado. `sudo -n` solo se usa si el usuario activa la opción de SO/traza;
  no se pide ni se guarda ninguna contraseña.
- **Agente SSH:** solo se consultan identidades y huellas; nunca se extraen
  claves privadas.
- **Auditoría:** eventos explícitos y saneados, con rotación; sin payload PTY.
- **KeePass:** importación read-only y bloqueo del vault por inactividad
  (`--vault-idle-timeout`, 15 min por defecto).

---

## Desarrollo y pruebas

```sh
make test                 # tests Go
make contracts-check      # contratos Go + typecheck TS
make bench                # benchmarks IPC, backend y FRP
make phase0               # typecheck + smoke del prototipo
make ui-check             # typecheck + tests de la UI
make ui-smoke             # 3 sesiones, tmux, restore y overflow
make hostkey-smoke        # host key estricta
make trzsz-smoke          # trzsz real contra sshd de pruebas
make frp-smoke            # FRP real (requiere binarios)
make p1-smoke             # host key + UI + FRP
make verify               # suite completa
```

Comandos útiles:

```sh
go test -v -count=1 ./ssh05
go test -v -count=1 ./internal/ipc ./internal/supervisor
CGO_ENABLED=0 go test -count=1 ./contracts ./internal/ipc ./internal/supervisor
```

Notas de desarrollo:

- Go 1.25 y `CGO_ENABLED=0` son obligatorios.
- Los tests deben ejecutarse desde la raíz del repositorio.
- `TestTableExample` en `internal/table` es una falla basal conocida en
  entornos con color.
- El frontend usa Bun 1.4.5 y `@opentui/core` 0.8.2 fijados.

---

## Documentación

- [SDD v0.3](SSH05_SDD_v0.3.md) — diseño y alcance.
- [ADRs](docs/adr/README.md) — decisiones de arquitectura.
- [Auditoría Fase 0](docs/audit/phase-0-audit.md).
- [Revisión funcional P0/P1/P2](docs/audit/functional-review-0.3.17.md).
- [Estado de integración](docs/integration/status.md).
- [Contratos UI ↔ supervisor](contracts/README.md).
- [Prototipo y resultados](docs/phase-0/README.md).
- [Rendimiento](docs/performance.md).
- [Releases](docs/releases/).

---

## Licencia y soporte

MIT. Ver [LICENSE](LICENSE).

**Dinámicas Electrónicas** · `dinamicas.electronicas@gmail.com`
