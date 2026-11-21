# Aceptación Fase P1 (TRSS 0.2)

| Campo | Valor |
|---|---|
| Fecha | 22/11/2026 |
| Alcance | Todos los requisitos `P1` del SDD 0.2 |
| Fuera de alcance | `P2`: Nmap, GeoIP/ASN/RDAP, SFTP/SCP/trzsz |
| Documento | `TRSS_SDD_v0.2.md` |
| Estado | **Aprobado** |

## Cómo reproducir

```sh
CGO_ENABLED=0 go test -count=1 ./tssh ./contracts ./internal/ipc ./internal/supervisor ./internal/audit ./internal/frp
cd prototype/terminal && bun run typecheck && bun test

make p1-smoke          # host key + UI + FRP real del supervisor
make trssd-frp-smoke   # solo frps/frpc reales a través de trssd
```

`make p1-smoke` requiere `bin/frps`, `bin/frpc`, `python3`, `curl` y `bun`.
Los binarios FRP se obtienen con `make download-frp`.

## Matriz de requisitos P1

| ID | Estado | Evidencia |
|---|---|---|
| SES-06 | Cumplido | `internal/supervisor/backend.go`: backoff, límite y cancelación; tests de reconexión |
| SES-07 | Cumplido | tmux opcional con `Ctrl+G`; nombres `trss-<alias>`; `tmuxStartCommand` en el runner |
| SES-08 | Cumplido | Restauración pendiente; `TmuxCreate=false` + `tmux has-session`; si falta, shell y se borra el binding |
| INV-02 | Cumplido | `tssh.ListHostEntries` respeta `Include`; `TestSSHHostsInclude` |
| INV-03 | Cumplido | `internal/supervisor/keepass.go` importa KDBX con `keepassxc-cli` |
| INV-04 | Cumplido | `Inventory.mergeLocked` conserva procedencia y expone conflictos sin sobrescribir |
| INV-05 | Cumplido | `#!!` + overrides persistentes en `~/.config/trss/hosts.json`; `F2` favorito, `F3` color, `F4` etiquetas |
| AUT-02 | Cumplido | `SSH_AUTH_SOCK` + `ssh-add -l`; solo proveedor/identidades, sin extraer claves |
| AUT-03 | Cumplido | `AuthRef` estable por UUID de KeePass; nunca contiene el secreto |
| AUT-04 | Cumplido | Tokens/secretos cifrados, auditoría saneada, sin credenciales en perfiles ni logs |
| AUT-05 | Cumplido | `--vault-idle-timeout`; `vault.unlockRequest`; hosts KDBX fuera del inventario al bloquear |
| FRP-01 | Cumplido | `internal/frp`: `frpc` y `frps` gestionados; `--frps-listen`, `frp.servers.start/stop` |
| FRP-02 | Cumplido | Crear/editar (`frp.proxies.apply`), validar (`frpc verify`) y eliminar (`frp.proxies.remove`) en 4 tipos |
| FRP-03 | Cumplido | Tokens con `crypto/rand`; `TestManagerFailsClosedWithoutEntropy` |
| FRP-04 | Cumplido | AES-GCM; `TestManagerPrivatePermissions` (key/config/log 0600, dirs 0700) |
| FRP-05 | Cumplido | Estado, destino, puerto, tipo, PID, uptime y errores en UI/IPC; sin estadísticas extra si no hay admin API |
| FRP-06 | Cumplido | Rotación reinicia primero el `frps` gestionado; endpoint remoto se rechaza para no exponer el token |
| FRP-07 | Cumplido | `--frpc-sha256` y `--frps-sha256` antes de arrancar; `frpc verify`/`frps verify` antes de usar |
| FRP-08 | Cumplido | El túnel vive en `internal/frp`, no en la sesión; prueba de cierre de pestaña con proxy online |
| RUT-01 | Cumplido | `routes.plan` combina directo, `ProxyJump` y visitor FRP |
| RUT-02 | Cumplido | `routes.probe`: `reachable`, `refused`, `timeout`, `dns_failure`, `unknown`; timeout no se interpreta como cerrado |
| RUT-03 | Cumplido | La ruta elegida se muestra en la barra de estado; no se abren exposiciones públicas silenciosas |
| UI-02 | Cumplido | Color RGB con degradación a 256/16 y `NO_COLOR`/`TRSS_THEME=mono` |
| UI-03 | Cumplido | Icono + texto de estado; el color nunca decide el significado |
| UI-04 | Cumplido | Búsqueda de hosts, paleta `Ctrl+K` y atajos en `keybindings.json` |
| OPS-01 | Cumplido | Workspaces, pestañas y tmux en `~/.config/trss/workspace.json` |
| OPS-02 | Cumplido | Sesiones y túneles separados; grupo de proceso, `Pdeathsig` en Linux y `Close` sin huérfanos |
| SEG-03 | Cumplido | Socket 0700/0600, credenciales de par, operaciones tipadas IPC |
| SEG-04 | Cumplido | `internal/audit` rota y sanea sin secretos; retención con `--audit-max-files` |
| SEG-05 | Cumplido | Fuzz de framing, pruebas de inyección, host key estricta y reconexión adversarial |

## Extra posterior: preferencias de UI

`~/.config/trss/preferences.json` (`TRSS_PREFERENCES`) guarda ancho del panel,
densidad y posición de pestañas. La paleta abre «Preferencias»; los cambios se
aplican y persisten con permisos `0600`.

## Extra posterior: DIA-01 (P2 adelantado)

`diagnostics.host` resuelve DNS, mide el puerto TCP, clasifica el resultado
(`reachable`, `refused`, `timeout`, `dns_failure`, `unknown`) y lee el banner
SSH con plazo. Un timeout se reporta como incertidumbre, no como puerto
cerrado. La paleta incluye «Diagnosticar host»; `make ui-smoke` lo comprueba
contra un sshd real.

## Extra posterior: informe saneado

`diagnostics.report` devuelve versión, hosts, conflictos, sesiones, vault y
estado FRP sin tokens ni datos de PTY. La paleta (`Ctrl+K`) permite exportarlo
a `~/.local/state/trss/diagnostics/` con permisos `0600`.

## Extra posterior: concurrencia de conexiones

`TestSshLoginConcurrent` (en `tssh/login_concurrent_test.go`) abre tres sesiones
a la vez con la misma configuración; se ejecuta dentro de `make ui-smoke` y
también con `-race` de forma manual. La inicialización sigue serializada, pero
el establecimiento de conexiones ya no lo está.

## Pruebas reales de red

`make trssd-frp-smoke` levanta un `frps` gestionado y un `frpc` reales, crea
proxies TCP, UDP, STCP y XTCP, comprueba HTTP y eco UDP, valida la ruta FRP
STCP, detiene cada proxy y verifica que deja de ser alcanzable.

`make ui-smoke` cubre tres sesiones SSH, resize, cierre independiente, tmux,
10 pestañas con overflow, recuperación tras caída de la UI y workspaces.

`make hostkey-smoke` demuestra que una clave de host cambiada se bloquea.

## Limitaciones documentadas

- La rotación de un `frps` remoto se rechaza a propósito: el token no se
  expone por IPC. La recuperación es actualizar el token fuera de banda y
  reiniciar con `--frp-token ... --frp-token-force`.
- Los proxies FRP no se reinician automáticamente tras reiniciar `trssd`;
  permanecen definidos y se reaplican desde la UI o por IPC.
- No se implementan los requisitos `P2` del SDD.
