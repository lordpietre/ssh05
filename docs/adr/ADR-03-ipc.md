# ADR-03: IPC local entre UI y supervisor

- **Estado:** Aceptado (transporte)
- **Fecha:** 21/11/2026
- **Requisitos afectados:** SES-01…06, OPS-02, SEG-03/04

## Contexto

La UI (Bun/OpenTUI) y el supervisor (Go) son procesos distintos. El SDD exige
comandos tipados, eventos por sesión, backpressure, cancelación y que una caída
del renderer no mate sesiones ni túneles.

## Alternativas

1. **JSON-RPC 2.0 sobre Unix domain socket** + canal binario separado para PTY.
   - Ventaja: depurable con `socat`/`jq`, versionado explícito, fácil en Go y TS.
   - Riesgo: JSON no es ideal para alto volumen; se mitiga separando el flujo
     binario.
2. **gRPC/protobuf sobre UDS**.
   - Ventaja: esquema fuerte y streaming.
   - Riesgo: toolchain adicional, peor depuración manual, más superficie.
3. **WebSocket local**.
   - Ventaja: ecosistema.
   - Riesgo: autenticación y permisos de socket menos directos.

## Decisión

Se acepta **JSON-RPC 2.0** para control y un **canal binario con encuadre de
longitud** para bytes de PTY, sobre un Unix domain socket en
`$XDG_RUNTIME_DIR/trss/control.sock` (modo `0600`, directorio `0700`).

Reglas:

- Handshake con `protocolVersion` y `requestId`; rechazo explícito de versiones
  incompatibles.
- Verificación del par con `SO_PEERCRED`/credenciales del sistema cuando exista.
- Un `sessionId` por flujo; orden garantizado por sesión.
- Colas acotadas y `flow-control` por sesión; nunca serializar PTY como JSON.
- Cancelación por `requestId` y cierre ordenado.
- La UI no envía shell interpolado: solo operaciones tipadas.
- En Windows, named pipes con el mismo contrato (fase posterior).

## Implementación actual

`internal/ipc` implementa el transporte:

| Elemento | Estado |
|---|---|
| Socket Unix con dir `0700` y socket `0600` | Hecho |
| Detección de socket vivo (`ErrAlreadyRunning`) y limpieza de huérfano | Hecho |
| Frames JSON (1 MiB) y binarios (4 MiB), con `sessionId` | Hecho |
| `Server`/`Client` con correlación por `requestId` | Hecho |
| Versión de protocolo y `Method.Valid()` | Hecho |
| `SO_PEERCRED` en Linux; permisos de socket en otros Unix | Hecho |
| Panic del handler sin filtrar el valor | Hecho |
| Cancelación por contexto y cierre de llamadas pendientes | Hecho |
| `CallTyped[P,R]` para métodos del contrato | Hecho |
| Servicio `sessions.*`, `hosts.list`, `routes.*` | Hecho (`internal/supervisor/service.go`) |
| Daemon `trssd` | Hecho (`cmd/trssd`) |
| Cliente TypeScript/Bun (`IpcClient`) | Hecho (`prototype/terminal/src/ipc`) |
| Prueba cruzada Bun ↔ Go | Hecho (opt-in `trssd.integration.test.ts`) |
| Conectar la UI OpenTUI | Pendiente |

Tests: `internal/ipc/ipc_test.go` y `internal/ipc/frame_test.go` cubren round
trip, versión, método desconocido, panics, permisos, segundo servidor,
JSON malformado, cancelación y framing binario.

Comando:

```sh
CGO_ENABLED=0 go test -count=1 ./internal/ipc
```

## Consecuencias

- El contrato se puede versionar y probar con un cliente CLI de diagnóstico.
- Hay que implementar framing binario, backpressure y reconexión de UI.
- La autenticación del socket es imprescindible: no se acepta “confiar en que es
  local”.

## Validación pendiente

- Cliente no autorizado rechazado en un escenario multi-usuario real.
- Renderer caído: supervisor y túneles sobreviven.
- 100 MB de salida con colas acotadas y UI responsiva.
- Cancelación de `open`, `resize`, `close` y `frp.apply` contra el supervisor.
- Compatibilidad de protocolo entre versiones.
- Cliente TypeScript/Bun y named pipes en Windows.

## Evidencia

- SDD Anexo B y §17.5.
- `internal/ipc/` (transporte implementado y probado).
- `internal/supervisor/` (backend, runner SSH, servicio de sesiones).
- `cmd/trssd` (daemon).
- `contracts/ipc.go` y `contracts/ts/index.ts` (envelopes y métodos).
