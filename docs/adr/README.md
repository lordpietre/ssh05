# Architecture Decision Records — ssh05

Registro de decisiones de arquitectura de la Fase 0. El estado de cada ADR es
explícito: **Aceptado (prototipo)** significa que hay evidencia reproducible,
pero no que la decisión esté cerrada para producción.

| ADR | Decisión | Estado | Evidencia |
|---|---|---|---|
| [ADR-01](ADR-01-ssh-engine.md) | Motor SSH del supervisor | Aceptado | Auditoría + `ssh05/ssh.go` + contrato |
| [ADR-02](ADR-02-vt-emulator.md) | Terminal embebido OpenTUI + PTY Bun | Aceptado (prototipo) | `prototype/terminal` + smoke test |
| [ADR-03](ADR-03-ipc.md) | IPC local tipado | Aceptado | `internal/ipc` + supervisor + UI |
| [ADR-04](ADR-04-kdbx.md) | Gestor de secretos KDBX | Aceptado | Import KeePassXC CLI implementado |
| [ADR-05](ADR-05-frp-ownership.md) | Propiedad de procesos FRP | Aceptado | Decisión; implementación Fase 3 |
| [ADR-06](ADR-06-host-sync.md) | Sincronización de inventario | Aceptado | Decisión; implementación Fase 2 |
| [ADR-07](ADR-07-udp-transports.md) | Transportes UDP opcionales | Aceptado (fuera de P0) | `ssh05/udp.go` (tsshd) |
| [ADR-08](ADR-08-tunnel-autostart.md) | Autoarranque de túneles | Aceptado | Decisión; implementación Fase 3 |

## Criterio de cierre de la Fase 0

La Fase 0 se considera ejecutada cuando:

1. Existe la auditoría del código y dependencias reales.
2. El prototipo demuestra `vim`, `htop`, `tmux` y resize.
3. Las ADR-01…ADR-08 tienen una decisión aceptada; sus validaciones se
   ejecutan en la fase de implementación correspondiente.

La Fase 0 y las decisiones de arquitectura están cerradas: ADR-01…ADR-08 tienen
una decisión aceptada. H-01/H-02/H-03/H-04/H-05 están resueltos. La UI usa IPC y
el supervisor; el trabajo restante es de implementación de fases, no de
decisiones abiertas.
