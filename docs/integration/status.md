# Estado de integración del SDD 0.2

| Campo | Valor |
|---|---|
| Fecha | 21/11/2026 |
| Commit base | `6e98b8d` |
| Documento | `TRSS_SDD_v0.2.md` |
| Fase actual | Fase 0 cerrada + inicio de Fase 1 (contratos) |

## Hecho

| Elemento | Estado | Evidencia |
|---|---|---|
| Auditoría de código y dependencias | Hecha | `docs/audit/phase-0-audit.md` |
| Prototipo PTY/OpenTUI (`vim`, `htop`, `tmux`, SSH, resize) | Hecho | `docs/phase-0/README.md` |
| ADR-01 motor SSH | **Aceptada** | `docs/adr/ADR-01-ssh-engine.md` |
| ADR-02 terminal embebido | Aceptada (prototipo) | `docs/adr/ADR-02-vt-emulator.md` |
| H-01 token FRP con fallback estático | **Resuelto** | `tssh/frp_proxy.go`, `tssh/frp_proxy_test.go` |
| H-05 inyección en copia remota FRP | **Resuelto** | `tssh/frp_proxy.go`, `tssh/frp_proxy_test.go` |
| Contratos UI ↔ supervisor | **Definidos** | `contracts/`, `contracts/ts/` |
| IPC local autenticado (transporte) | **Hecho** | `internal/ipc/`, tests |
| Backend `SessionBackend` + runner SSH | **Hecho** | `internal/supervisor/` |
| Servicio IPC de sesiones + daemon | **Hecho** | `internal/supervisor/service.go`, `cmd/trssd` |
| Inventario `hosts.list` desde `ssh_config` | **Hecho** | `tssh/hosts_api.go`, `internal/supervisor/hosts.go` |
| `SessionBackend` y `TabStore` | Definidos | `contracts/session.go`, `contracts/ts/index.ts` |
| Envelope IPC y métodos del Anexo B | Definidos | `contracts/ipc.go`, `contracts/ts/index.ts` |

## Pendiente inmediato

| Prioridad | Trabajo | Criterio de cierre |
|---|---|---|
| 1 | Cliente TypeScript/Bun del IPC + `TabStore` | UI caída no mata sesiones; 10 pestañas (UIX-01) |
| 2 | Integrar `trssd` con la UI OpenTUI | 3 sesiones reales, resize y cierre independiente desde la UI |
| 2 | Persistencia de sesiones/túneles al cerrar UI | FRP-08, OPS-02 |
| 3 | Inventario con provenance y conflictos | Importar `ssh_config` sin sobrescribir cambios locales |
| 3 | FRP: permisos `0700`, token fuera de JSON plano, tipos UDP/STCP/XTCP | FRP-03/04 y pruebas de permisos |

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
