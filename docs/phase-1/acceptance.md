# Fase 1 — Aceptación P0

| Campo | Valor |
|---|---|
| Fase | 1 — núcleo multipestaña P0 |
| Estado | Aprobada por la suite de aceptación |
| Referencia | `SSH05_SDD_v0.3.md` §16.1 y capítulos 2, 4, 11 |
| Fecha | 21/11/2026 |

## Requisitos y evidencia

| Requisito | Resultado | Prueba |
|---|---|---|
| SES-01 varias pestañas SSH | Abre 3+ sesiones independientes | `make ui-smoke` |
| SES-02 cambio de pestaña | Alterna sin cerrar las demás | `make ui-smoke` |
| SES-03 cierre independiente | Diálogo cerrar vista/desconectar/desacoplar tmux | `make ui-smoke` |
| SES-04 PTY, teclado, resize | `vim`, `htop`, SSH y resize 120→100 | `make ui-smoke`, `make phase0` |
| SES-05 estados | `connecting/connected/reconnecting/failed/closed` con `seq` | `go test ./internal/supervisor` |
| INV-01 inventario canónico | `hosts.list` sin secretos | `go test ./internal/supervisor` |
| AUT-01 agent/claves OpenSSH | Runner SSH real con key auth | `make ui-smoke` |
| UI-01 OpenTUI + teclado | UI completa, tabs, paleta, workspaces | `make ui-smoke` |
| SEG-01 host key estricta | Clave correcta conecta; cambiada bloquea | `make hostkey-smoke` |
| SEG-02 sin inyección/filtración | IPC tipado, comandos citados, auditoría saneada | `go test ./ssh05 ./internal/ipc` |
| PTY 100 MB acotado | Buffer pendiente ≤1 MiB | `go test ./internal/supervisor` |
| Recuperación tras caída UI | `sessions.list`/`attach` sin duplicar | `make ui-smoke` |

## Comandos

```sh
CGO_ENABLED=0 go test -count=1 ./ssh05 ./contracts ./internal/ipc ./internal/supervisor ./internal/audit
make hostkey-smoke
make ui-smoke
```

## Fuera de Fase 1 (P1/P2)

- KeePass/KDBX: Fase 2.
- FRP vertical: Fase 3.
- Diagnóstico, Nmap, GeoIP, SFTP/SCP: Fase 5/P2.

## Criterio de cierre

La Fase 1 se considera aprobada cuando `make hostkey-smoke` y `make ui-smoke`
pasan, y la suite Go anterior está en verde. No se declara terminada ninguna
capacidad P1/P2 en este documento.
