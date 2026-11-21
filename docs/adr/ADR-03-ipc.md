# ADR-03: IPC local entre UI y supervisor

- **Estado:** Propuesto
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

## Decisión propuesta

Usar **JSON-RPC 2.0** para control y un **canal binario con encuadre de longitud**
para bytes de PTY, sobre un Unix domain socket en
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

## Consecuencias

- El contrato se puede versionar y probar con un cliente CLI de diagnóstico.
- Hay que implementar framing binario, backpressure y reconexión de UI.
- La autenticación del socket es imprescindible: no se acepta “confiar en que es
  local”.

## Validación pendiente

- Cliente no autorizado rechazado.
- Renderer caído: supervisor y túneles sobreviven.
- 100 MB de salida con colas acotadas y UI responsiva.
- Cancelación de `open`, `resize`, `close` y `frp.apply`.
- Compatibilidad de protocolo entre versiones.

## Evidencia

- SDD Anexo B y §17.5.
- `docs/audit/phase-0-audit.md` (no existe IPC en el código actual).
