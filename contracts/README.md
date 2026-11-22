# Contratos ssh05 (UI ↔ supervisor)

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
7. `SessionOptions.tmuxSession` es opcional y no secreto. Si se usa, la sesión
   remota puede sobrevivir al cierre de la vista; no se promete lo mismo sin
   tmux.
8. `Workspace` solo contiene vistas y referencias no secretas; restaurar un
   workspace no crea conexiones vivas por sí solo. `TabRecord.sessionId` permite
   reutilizar una sesión viva al cambiar de workspace.
9. `sessions.list` devuelve solo sesiones vivas; `sessions.attach` permite a una
   UI nueva reengancharse sin abrir una sesión SSH duplicada.

## Transporte previsto

- Unix domain socket local autenticado (`$XDG_RUNTIME_DIR/ssh05/control.sock`,
  `0600`, directorio `0700`), con verificación del par por credenciales del
  sistema.
- JSON-RPC 2.0 para control y eventos; canal binario separado para PTY.
- Handshake con `protocolVersion`, rechazo explícito de versiones incompatibles
  y límites de cola por sesión.

## Verificación

```sh
CGO_ENABLED=0 go test -count=1 ./contracts ./internal/ipc ./internal/supervisor
cd prototype/terminal && bun run typecheck:contracts && bun test
# o, desde la raíz:
make contracts-check
make ui-check
```

## Estado

- `SessionBackend` y `TabStore` definidos.
- Envelope IPC y métodos del Anexo B definidos.
- Implementación real del supervisor/IPC: **pendiente** (ADR-03).
