# Contratos TRSS (UI ↔ supervisor)

Este directorio es la fuente de verdad de los contratos compartidos entre la UI
y el supervisor. No contiene implementación: describe vocabulario, envelopes y
capacidades.

| Archivo | Lado | Contenido |
|---|---|---|
| `domain.go` / `ts/index.ts` | Go / TS | Hosts, rutas, workspaces, FRP |
| `session.go` / `ts/index.ts` | Go / TS | `SessionBackend`, fases, intents, capacidades |
| `ipc.go` / `ts/index.ts` | Go / TS | `RPCRequest`, `RPCResponse`, eventos y métodos |
| `internal/ipc/` | Go | Transporte local: socket, frames, servidor y cliente |
| `contracts_test.go` | Go | Validación de fases, métodos, JSON y ausencia de secretos |

## Reglas de compatibilidad

1. `ProtocolVersion` empieza en `1`. Cualquier cambio incompatible del envelope
   o de la semántica de un método requiere una versión nueva y una ADR.
2. Los métodos son operaciones tipadas. **No existe** `shell.exec` ni una API
   genérica de ejecución arbitraria.
3. Los bytes del PTY viajan por un canal binario con encuadre de longitud, no
   como eventos JSON ilimitados.
4. Los perfiles y eventos no contienen contraseñas, tokens ni claves privadas;
   solo referencias (`CredentialReference`).
5. `close-view` no implica terminar el proceso si el backend declara
   `supportsDetach`; la UI debe consultar `capabilities()` antes de prometerlo.
6. El orden por sesión es obligatorio; `SessionStatus.seq` es monótono.

## Transporte previsto

- Unix domain socket local autenticado (`$XDG_RUNTIME_DIR/trss/control.sock`,
  `0600`, directorio `0700`), con verificación del par por credenciales del
  sistema.
- JSON-RPC 2.0 para control y eventos; canal binario separado para PTY.
- Handshake con `protocolVersion`, rechazo explícito de versiones incompatibles
  y límites de cola por sesión.

## Verificación

```sh
CGO_ENABLED=0 go test -count=1 ./contracts ./internal/ipc
cd prototype/terminal && bun run typecheck:contracts
# o, desde la raíz:
make contracts-check
```

## Estado

- `SessionBackend` y `TabStore` definidos.
- Envelope IPC y métodos del Anexo B definidos.
- Implementación real del supervisor/IPC: **pendiente** (ADR-03).
