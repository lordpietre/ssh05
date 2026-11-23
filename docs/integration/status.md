# Estado de integración del SDD 0.3

| Campo | Valor |
|---|---|
| Fecha | 23/11/2026 |
| Commit base | `b5c6cd6` |
| Documento | `SSH05_SDD_v0.3.md` |
| Fase actual | **P0 + P1 cerrados**; **P2 funcional cerrado** |

## Hecho

| Elemento | Estado | Evidencia |
|---|---|---|
| Auditoría de código y dependencias | Hecha | `docs/audit/phase-0-audit.md` |
| Prototipo PTY/OpenTUI (`vim`, `htop`, `tmux`, SSH, resize) | Hecho | `docs/phase-0/README.md` |
| ADR-01 motor SSH | **Aceptada** | `docs/adr/ADR-01-ssh-engine.md` |
| ADR-02 terminal embebido | Aceptada (prototipo) | `docs/adr/ADR-02-vt-emulator.md` |
| H-01 token FRP con fallback estático | **Resuelto** | `ssh05/frp_proxy.go`, `ssh05/frp_proxy_test.go` |
| H-02 permisos del directorio FRP | **Resuelto** | `ensurePrivateDir`, test |
| H-03 token FRP en reposo | **Resuelto** | AES-GCM + `~/.config/ssh05/frp.key` 0600; TOML remoto 0600 |
| H-04 tipos de proxy FRP | **Resuelto** | 4 tipos validados; runtime TCP/STCP/XTCP/UDP |
| H-05 inyección en copia remota FRP | **Resuelto** | `ssh05/frp_proxy.go`, `ssh05/frp_proxy_test.go` |
| Contratos UI ↔ supervisor | **Definidos** | `contracts/`, `contracts/ts/` |
| IPC local autenticado (transporte) | **Hecho** | `internal/ipc/`, tests |
| Backend `SessionBackend` + runner SSH | **Hecho** | `internal/supervisor/` |
| Servicio IPC de sesiones + daemon | **Hecho** | `internal/supervisor/service.go`, `cmd/ssh05d` |
| Inventario `hosts.list` desde `ssh_config` | **Hecho** | `ssh05/hosts_api.go`, `internal/supervisor/hosts.go` |
| Importación KDBX con KeePassXC CLI | **Hecho** | `internal/supervisor/keepass.go` |
| Deduplicación y provenance ssh_config+KDBX | **Hecho** | `internal/supervisor/inventory.go` |
| Conflictos de inventario sin sobrescritura | **Hecho** | `HostsListResult.conflicts` + tests |
| Importación desde UI (`Ctrl+I`) | **Hecho** | `hosts.import` |
| AUT-02 agente SSH sin extraer claves | **Hecho** | `SSH_AUTH_SOCK` + `ssh-add -l` (solo huellas) |
| AUT-05 bloqueo de vault por inactividad | **Hecho** | `--vault-idle-timeout`, `vault.unlockRequest` |
| Reconexión con backoff y cancelación | **Hecho** | `internal/supervisor/backend.go` + tests |
| Auditoría saneada con rotación | **Hecho** | `internal/audit/`, `--audit-log` |
| Cliente TS/Bun del IPC | **Hecho** | `prototype/terminal/src/ipc/` |
| `TabStore` renderer-independent | **Hecho** | `prototype/terminal/src/tabs/store.ts` |
| UI OpenTUI conectada al supervisor | **Hecho** | `prototype/terminal/src/ssh05.ts` |
| Smoke UI: 3 SSH + resize + cierre independiente | **Hecho** | `make ui-smoke` |
| tmux opcional + reattach al reabrir | **Hecho** | `Ctrl+G`, `SessionsOpenParams.tmuxSession` |
| Workspace persistente (pestañas + tmux) | **Hecho** | `src/workspaces/`, `~/.config/ssh05/workspace.json` |
| Múltiples workspaces con nombre y selector | **Hecho** | `Ctrl+P`, `--workspace`, `SSH05_WORKSPACE_DIR` |
| Paleta de comandos | **Hecho** | `Ctrl+K` en la UI |
| Rutas directas/ProxyJump con motivo | **Hecho** | `routes.plan` + `ProxyJump` + estado en UI |
| FRP supervisor local | **Hecho** | `internal/frp`, endpoints/proxies, procesos |
| FRP-01 frps local gestionado | **Hecho** | `--frps-listen`, `frp.servers.start/stop` |
| FRP-02 crear/editar/validar/eliminar | **Hecho** | `frp.proxies.apply/remove` + `verify` |
| FRP-05 estado, PID y uptime | **Hecho** | `pid`/`uptimeSeconds` en IPC y UI |
| FRP-06 rotación coordinada de token | **Hecho** | reinicia frps gestionado; rechaza remoto |
| FRP-08 cierre de pestaña no corta el túnel | **Hecho** | prueba de sesión + proxy online |
| FRP tokens cifrados, rotación y tipos | **Hecho** | AES-GCM, `frp.tokens.rotate`, 4 tipos |
| FRP hash de binario y sin huérfanos | **Hecho** | `--frpc-sha256`, grupos de proceso, `Pdeathsig` |
| FRP UI (listar/crear/parar/eliminar/rotar) | **Hecho** | `Ctrl+F` |
| Ruta FRP por visitor | **Hecho** | `RouteForHost` + `RouteID` |
| RUT-02/RUT-03 sondeo y ruta visible | **Hecho** | `routes.probe`; timeout ≠ cerrado; paleta |
| UI-04 atajos configurables | **Hecho** | `keybindings.json` (incl. F2/F3/F4) + paleta |
| Pruebas adversariales (framing, inyección, reconexión) | **Hecho** | fuzz tests + tests de inyección/reconnect |
| Restauración como vistas pendientes | **Hecho** | Enter reconecta; nunca simula sesión viva |
| Backend 100 MB con buffers acotados | **Hecho** | tests de `internal/supervisor` |
| Barra de pestañas con overflow y activa visible | **Hecho** | `src/tabs/tabbar.ts` + tests |
| INV-05 grupos, favoritos y colores | **Hecho** | `#!!` + overrides editables (`F2/F3/F4`) |
| UI-02 color RGB con degradación | **Hecho** | `src/theme/color.ts`, 256/16/mono |
| UI-03 estado por símbolo y texto | **Hecho** | `phaseIcon` + `phaseLabel` + estado textual |
| Smoke real de 10 pestañas + contadores | **Hecho** | `make ui-smoke` |
| Recuperación tras caída de UI (`sessions.list`/`attach`) | **Hecho** | reengancha sesiones vivas sin duplicar |
| Aceptación Fase 1 P0 | **Aprobada** | `docs/phase-1/acceptance.md`, `make phase1-smoke` |
| Aceptación P1 completa | **Aprobada** | `docs/phase-1/p1-acceptance.md`, `make p1-smoke` |
| Revisión funcional completa P0/P1/P2 | **Aprobada con límites** | `docs/audit/functional-review-0.3.17.md` |
| SshLogin concurrente con la misma config | **Hecho** | `acquireConfigRead`, `TestSshLoginConcurrent` |
| Informe de diagnóstico saneado | **Hecho** | `diagnostics.report`, `src/diagnostics/export.ts` |
| Retención de auditoría y buffers | **Hecho** | `--audit-max-files`, `--max-pending-bytes` |
| DIA-01 DNS, puerto, banner y latencia | **Hecho** | `diagnostics.host` + paleta y smoke real |
| DIA-02 Nmap optativo y confirmado | **Hecho** | `diagnostics.nmap`, doble confirmación |
| DIA-03 ASN/RDAP y país orientativo | **Hecho** | Team Cymru DNS + rdap.org; IP privadas no se consultan |
| Línea base de rendimiento | **Hecho** | `make bench`, `docs/performance.md` |
| DAT-01 SCP/SFTP tipado | **Hecho** | `transfers.start/list/cancel`, UI y smoke real |
| DAT-01 trzsz/Zmodem en sesiones | **Hecho** | `sessionFilter`, `--trzsz`/`--zmodem`/`--download-path`, `make trzsz-smoke` |
| Preferencias de UI persistentes | **Hecho** | ancho, densidad, posición y reordenado |
| Ayuda de atajos en la UI | **Hecho** | `F1` + comando en la paleta |
| Barra de botones navegable | **Hecho** | `Menú`, acciones y ratón; `Tab`/flechas |
| Identidad ssh05 reunificada | **Hecho** | `ssh05d`, `src/ssh05.ts`, rutas `~/.config/ssh05`, `SSH05_*` con respaldo `TRSS_*` |
| Vendor y soporte en el producto | **Hecho** | `internal/identity/`, `--version` y ayuda; `Dinámicas Electrónicas` |
| Publicación de releases | **Hecho** | `v0.3.18`–`v0.3.27`: `.deb`, rpm, binarios y win7 |
| Paquete autosuficiente | **Hecho** | `.deb` con `ssh05`, `ssh05d`, `ssh05-ui`, Bun y dependencias de producción |
| Supervisor por defecto | **Hecho** | `default.target.wants` global + arranque bajo demanda desde `ssh05-ui` |
| Docker opt-in en el servicio | **Hecho** | unidad sin `*_DISABLE_DOCKER`; `--docker` en ExecStart lista contenedores |
| Navegación completa con flechas | **Hecho** | menús, submenús, listas y formularios; `make ui-smoke` |
| Overlays opacos | **Hecho** | relleno al ancho completo; sin transparencias de fondo |
| Navegación solo con flechas | **Hecho** | `j`/`k` no mueven; se escriben en búsqueda; `TestHostModelVimKeysDoNotNavigate` |
| Pegado en la app | **Hecho** | bracketed paste en búsqueda, formularios y terminal; `ui-smoke` |
| Contraseña en alta de host | **Hecho** | campo en la UI; `hosts.create` escribe `#!! encPassword`; test de ida y vuelta |
| `ssh05` abre la UI por defecto | **Hecho** | sin argumentos ejecuta `ssh05-ui`; `SSH05_NO_UI=1` fuerza el gestor clásico |
| Alta de hosts desde la UI | **Hecho** | `hosts.create`, `Ctrl+N` |
| Servicio de usuario y Docker opt-in | **Hecho** | `ssh05d.service`, `--docker` |
| tmux solo se reengancha si existe | **Hecho** | `TmuxCreate=false`, sonda `has-session`, prueba real |
| FRP real de extremo a extremo | **Hecho** | `scripts/ssh05d-frp-smoke.sh`: TCP/UDP/STCP; XTCP condicional a STUN |
| `SessionBackend` y `TabStore` | Definidos | `contracts/session.go`, `contracts/ts/index.ts` |
| Envelope IPC y métodos del Anexo B | Definidos | `contracts/ipc.go`, `contracts/ts/index.ts` |

## Pendiente inmediato

| Prioridad | Trabajo | Criterio de cierre |
|---|---|---|
| — | GeoIP de ciudad | No incluido; ASN/RDAP y país orientativo ya operativos |

Releases publicados: <https://github.com/lordpietre/ssh05/releases> (`v0.3.18`
identidad, `v0.3.19` trzsz/Zmodem, `v0.3.20` paquete autosuficiente,
`v0.3.21` Docker opt-in, `v0.3.22` flechas, `v0.3.23` overlays opacos, `v0.3.24` solo flechas, `v0.3.25` pegado, `v0.3.26` contraseña enc, `v0.3.27` UI por defecto).

## Invariantes que no se negocian

1. Ningún perfil, evento o log contiene contraseñas, tokens o claves privadas.
2. Ningún cambio de clave host se acepta automáticamente.
3. Cerrar una vista no detiene túneles compartidos.
4. El PTY no se serializa como JSON ilimitado.
5. La UI no ejecuta shell arbitrario por IPC; solo operaciones tipadas.

## Comandos de verificación

```sh
CGO_ENABLED=0 go test -count=1 ./ssh05 ./contracts
cd prototype/terminal && bun run typecheck && bun run typecheck:contracts
# o:
make contracts-check
```
