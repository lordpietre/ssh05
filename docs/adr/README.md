# Architecture Decision Records — TRSS

Registro de decisiones de arquitectura de la Fase 0. El estado de cada ADR es
explícito: **Aceptado (prototipo)** significa que hay evidencia reproducible,
pero no que la decisión esté cerrada para producción.

| ADR | Decisión | Estado | Evidencia |
|---|---|---|---|
| [ADR-01](ADR-01-ssh-engine.md) | Motor SSH del supervisor | Aceptado | Auditoría + `tssh/ssh.go` + contrato |
| [ADR-02](ADR-02-vt-emulator.md) | Terminal embebido OpenTUI + PTY Bun | Aceptado (prototipo) | `prototype/terminal` + smoke test |
| [ADR-03](ADR-03-ipc.md) | IPC local tipado | Aceptado (transporte) | `internal/ipc` + tests |
| [ADR-04](ADR-04-kdbx.md) | Gestor de secretos KDBX | Propuesto | Auditoría; sin implementación |
| [ADR-05](ADR-05-frp-ownership.md) | Propiedad de procesos FRP | Propuesto | Auditoría de `frp_proxy.go` |
| [ADR-06](ADR-06-host-sync.md) | Sincronización de inventario | Propuesto | Auditoría de `config.go` |
| [ADR-07](ADR-07-udp-transports.md) | Transportes UDP opcionales | Diferido | `tssh/udp.go` (tsshd) |
| [ADR-08](ADR-08-tunnel-autostart.md) | Autoarranque de túneles | Propuesto | SDD §8/§19 |

## Criterio de cierre de la Fase 0

La Fase 0 se considera ejecutada cuando:

1. Existe la auditoría del código y dependencias reales.
2. El prototipo demuestra `vim`, `htop`, `tmux` y resize.
3. Las ADR-01…ADR-04 tienen una decisión registrada y validación pendiente
   explícita.

La Fase 1 no debe comenzar hasta resolver los hallazgos H-01 y H-05 de la
auditoría y cerrar ADR-03 con pruebas del núcleo P0. H-01/H-05 están resueltos;
ADR-01 está cerrada y ADR-03 tiene el transporte aceptado. Falta conectar el
supervisor y el cliente UI.
