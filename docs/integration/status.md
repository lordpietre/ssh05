# Estado de integración del SDD 0.2

| Campo | Valor |
|---|---|
| Fecha | 21/11/2026 |
| Commit base | `6e98b8d` |
| Documento | `TRSS_SDD_v0.2.md` |
| Fase actual | Fase 1 P0 aprobada; Fase 2 completada; Fase 3/4 en curso |

## Hecho

| Elemento | Estado | Evidencia |
|---|---|---|
| Auditoría de código y dependencias | Hecha | `docs/audit/phase-0-audit.md` |
| Prototipo PTY/OpenTUI (`vim`, `htop`, `tmux`, SSH, resize) | Hecho | `docs/phase-0/README.md` |
| ADR-01 motor SSH | **Aceptada** | `docs/adr/ADR-01-ssh-engine.md` |
| ADR-02 terminal embebido | Aceptada (prototipo) | `docs/adr/ADR-02-vt-emulator.md` |
| H-01 token FRP con fallback estático | **Resuelto** | `tssh/frp_proxy.go`, `tssh/frp_proxy_test.go` |
| H-02 permisos del directorio FRP | **Resuelto** | `ensurePrivateDir`, test |
| H-03 token FRP en reposo | **Resuelto** | AES-GCM + `~/.tssh/frp.key` 0600; TOML remoto 0600 |
| H-04 tipos de proxy FRP | **Resuelto** | 4 tipos validados; runtime TCP/STCP/XTCP/UDP |
| H-05 inyección en copia remota FRP | **Resuelto** | `tssh/frp_proxy.go`, `tssh/frp_proxy_test.go` |
| Contratos UI ↔ supervisor | **Definidos** | `contracts/`, `contracts/ts/` |
| IPC local autenticado (transporte) | **Hecho** | `internal/ipc/`, tests |
| Backend `SessionBackend` + runner SSH | **Hecho** | `internal/supervisor/` |
| Servicio IPC de sesiones + daemon | **Hecho** | `internal/supervisor/service.go`, `cmd/trssd` |
| Inventario `hosts.list` desde `ssh_config` | **Hecho** | `tssh/hosts_api.go`, `internal/supervisor/hosts.go` |
| Importación KDBX con KeePassXC CLI | **Hecho** | `internal/supervisor/keepass.go` |
| Deduplicación y provenance ssh_config+KDBX | **Hecho** | `internal/supervisor/inventory.go` |
| Conflictos de inventario sin sobrescritura | **Hecho** | `HostsListResult.conflicts` + tests |
| Importación desde UI (`Ctrl+I`) | **Hecho** | `hosts.import` |
| AUT-05 bloqueo de vault por inactividad | **Hecho** | `--vault-idle-timeout`, `vault.unlockRequest` |
| Reconexión con backoff y cancelación | **Hecho** | `internal/supervisor/backend.go` + tests |
| Auditoría saneada con rotación | **Hecho** | `internal/audit/`, `--audit-log` |
| Cliente TS/Bun del IPC | **Hecho** | `prototype/terminal/src/ipc/` |
| `TabStore` renderer-independent | **Hecho** | `prototype/terminal/src/tabs/store.ts` |
| UI OpenTUI conectada al supervisor | **Hecho** | `prototype/terminal/src/trss.ts` |
| Smoke UI: 3 SSH + resize + cierre independiente | **Hecho** | `make ui-smoke` |
| tmux opcional + reattach al reabrir | **Hecho** | `Ctrl+G`, `SessionsOpenParams.tmuxSession` |
| Workspace persistente (pestañas + tmux) | **Hecho** | `src/workspaces/`, `~/.config/trss/workspace.json` |
| Múltiples workspaces con nombre y selector | **Hecho** | `Ctrl+P`, `--workspace`, `TRSS_WORKSPACE_DIR` |
| Paleta de comandos | **Hecho** | `Ctrl+K` en la UI |
| Rutas directas/ProxyJump con motivo | **Hecho** | `routes.plan` + `ProxyJump` + estado en UI |
| FRP supervisor local | **Hecho** | `internal/frp`, endpoints/proxies, procesos |
| FRP-01 frps local gestionado | **Hecho** | `--frps-listen`, `frp.servers.start/stop` |
| FRP-02 crear/editar/validar/eliminar | **Hecho** | `frp.proxies.apply/remove` + `verify` |
| FRP tokens cifrados, rotación y tipos | **Hecho** | AES-GCM, `frp.tokens.rotate`, 4 tipos |
| FRP hash de binario y sin huérfanos | **Hecho** | `--frpc-sha256`, grupos de proceso |
| FRP UI (listar/crear/parar/eliminar/rotar) | **Hecho** | `Ctrl+F` |
| Ruta FRP por visitor | **Hecho** | `RouteForHost` + `RouteID` |
| RUT-02/RUT-03 sondeo y ruta visible | **Hecho** | `routes.probe`; timeout ≠ cerrado; paleta |
| UI-04 atajos configurables | **Hecho** | `keybindings.json` + paleta |
| Pruebas adversariales (framing, inyección, reconexión) | **Hecho** | fuzz tests + tests de inyección/reconnect |
| Restauración como vistas pendientes | **Hecho** | Enter reconecta; nunca simula sesión viva |
| Backend 100 MB con buffers acotados | **Hecho** | tests de `internal/supervisor` |
| Barra de pestañas con overflow y activa visible | **Hecho** | `src/tabs/tabbar.ts` + tests |
| INV-05 grupos, favoritos y colores | **Hecho** | `#!! GroupLabels`, `#!! Favorite`, `#!! Color` |
| UI-02 color RGB con degradación | **Hecho** | `src/theme/color.ts`, 256/16/mono |
| UI-03 estado por símbolo y texto | **Hecho** | `phaseIcon` + `phaseLabel` + estado textual |
| Smoke real de 10 pestañas + contadores | **Hecho** | `make ui-smoke` |
| Recuperación tras caída de UI (`sessions.list`/`attach`) | **Hecho** | reengancha sesiones vivas sin duplicar |
| Aceptación Fase 1 P0 | **Aprobada** | `docs/phase-1/acceptance.md`, `make phase1-smoke` |
| `SessionBackend` y `TabStore` | Definidos | `contracts/session.go`, `contracts/ts/index.ts` |
| Envelope IPC y métodos del Anexo B | Definidos | `contracts/ipc.go`, `contracts/ts/index.ts` |

## Pendiente inmediato

| Prioridad | Trabajo | Criterio de cierre |
|---|---|---|
| 2 | FRP frps local y actualización verificada | FRP-01/07 |
| 3 | Config SSH por conexión | Eliminar serialización de `SshLogin` |
| 3 | Resolver de rutas FRP | RUT-01/03 para FRP |

## Invariantes que no se negocian

1. Ningún perfil, evento o log contiene contraseñas, tokens o claves privadas.
2. Ningún cambio de clave host se acepta automáticamente.
3. Cerrar una vista no detiene túneles compartidos.
4. El PTY no se serializa como JSON ilimitado.
5. La UI no ejecuta shell arbitrario por IPC; solo operaciones tipadas.

## Comandos de verificación

```sh
CGO_ENABLED=0 go test -count=1 ./tssh ./contracts
cd prototype/terminal && bun run typecheck && bun run typecheck:contracts
# o:
make contracts-check
```
