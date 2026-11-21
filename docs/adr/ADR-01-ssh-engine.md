# ADR-01: Motor SSH del supervisor

- **Estado:** Aceptado
- **Fecha:** 21/11/2026
- **Decisores:** responsable del proyecto + implementación
- **Requisitos afectados:** SES-01…06, AUT-01…05, SEG-01/02

## Contexto

El SDD exige que el supervisor abra sesiones SSH reales con PTY, señales,
resize, ProxyJump, agent, verificación estricta de host y cancelación. El
repositorio actual ya implementa un cliente SSH completo sobre
`golang.org/x/crypto/ssh` con una interfaz estable (`SshClient`, `SshSession`)
en `tssh/ssh.go`. La alternativa es delegar en el binario `ssh` del sistema.

La decisión importa porque el supervisor necesita multiplexar sesiones, aislar
fallos y no depender de la versión de OpenSSH instalada en cada equipo.

## Alternativas

1. **Reutilizar el cliente Go existente** (`tssh/ssh.go`, `login.go`,
   `auth_method.go`, `known_hosts.go`).
   - Ventaja: agent, ProxyJump, teclado interactivo, OTP, certificados,
     known_hosts y UDP ya resueltos; sin parseo de salida de un proceso.
   - Riesgo: biblioteca `x/crypto/ssh` no cubre algoritmos/extensiones nuevas
     al ritmo de OpenSSH; hay que auditar el fork y mantener compatibilidad.
2. **Ejecutar OpenSSH externo** (`ssh -tt` por sesión).
   - Ventaja: máxima compatibilidad y menor mantenimiento de protocolo.
   - Riesgo: PTY, señales, resize, control de canales, autenticación y
     diagnóstico dependen de rasgos del binario; harder IPC binario y control
     fino; el argv puede filtrar secretos si no se diseña con cuidado.
3. **Ambos**: Go por defecto, OpenSSH como fallback explícito por perfil.

## Decisión

Se acepta el **cliente Go existente** como motor por defecto del supervisor,
extraído detrás del contrato `SessionBackend` (`contracts/`). OpenSSH externo
queda como adaptador de respaldo explícito para perfiles que requieran
algoritmos no soportados o comportamiento específico del sistema. La UI nunca
invoca `ssh` directamente; solo el supervisor selecciona el adaptador y
registra la razón.

El motor elegido debe implementar `contracts.SessionBackend` y exponer sus
capacidades reales. No se permite que la UI infiera capacidades por el índice
de pestaña o por el nombre del host.

### Capacidades verificadas en el código

| Capacidad | Implementación actual |
|---|---|
| PTY remoto | `SshSession.RequestPty` (`tssh/ssh.go:134`) |
| Resize | `SshSession.WindowChange` (`tssh/ssh.go:108`) |
| Shell/ejecución | `SshSession.Shell`, `Run`, `Start` (`tssh/ssh.go:98-112`) |
| ProxyJump | `tssh/login.go` (multihop) |
| Agent / claves | `tssh/agent.go`, `tssh/auth_method.go` |
| Host key estricta | `tssh/known_hosts.go` |
| UDP/KCP/QUIC | `tssh/udp.go` (adaptador separado, no P0) |

**Condición conocida:** `SshSession` no incluye señales en su interfaz base;
el runner SSH usa la interfaz opcional `SshSignalSender`. El transporte
UDP/tsshd no la implementa, por lo que las señales quedan limitadas al motor
SSH clásico. El supervisor deberá añadir cierre de árbol de procesos antes de
cerrar SES-04.

Además, `SshLogin` inicializa configuración global; hasta que exista config por
conexión, `configMu` serializa init + connect para evitar carreras entre
sesiones. El runner SSH y el servicio IPC ya están implementados en
`internal/supervisor`.

## Criterio de cierre

ADR-01 se considera cerrada porque hay una decisión explícita, una ruta de
reutilización concreta y un contrato destino (`contracts.SessionBackend`). No
implica que la matriz P0 esté aprobada: las pruebas pendientes siguen siendo
condición de salida de la Fase 1.

## Consecuencias

- Se conserva la lógica de `known_hosts` y `ProxyJump`, reduciendo el riesgo de
  una reimplementación incompleta.
- El supervisor debe exponer capacidades reales (`supportsRemoteTmux`,
  `supportsBinaryStreaming`, etc.) para no prometer de más.
- La coexistencia de adaptadores exige pruebas de paridad y una política de
  selección visible en el inspector.

## Validación ejecutada

- `CGO_ENABLED=0 go build ./cmd/tssh` OK con Go 1.25.0.
- `CGO_ENABLED=0 go test -count=1 ./tssh` OK.
- Interfaz `SshClient`/`SshSession` compilada y usada por el login actual.
- El prototipo de Fase 0 abrió una sesión SSH real con PTY y resize.

## Validación pendiente

- Tres sesiones concurrentes con `vim`, `htop` y `tmux`.
- Cambio de clave host: bloqueo y decisión explícita, sin aceptación silenciosa.
- `ProxyJump` multihop, agent, keyboard-interactive y OTP en la matriz P0.
- Cierre de árbol de procesos.
- Resize y señales en Linux x86_64/aarch64; después macOS/Windows.
- Comparación con OpenSSH externo en algoritmos modernos y casos límite.

## Evidencia

- `tssh/ssh.go:208` (`SshLogin`, `SshClient`, `SshSession`).
- `tssh/login.go` (resolución de destino, ProxyJump, PTY, keepalive).
- `tssh/known_hosts.go` (host key callback).
- `tssh/agent.go`, `tssh/auth_method.go`, `tssh/otp.go`.
- `contracts/session.go` y `contracts/ts/index.ts` (`SessionBackend`).
- `contracts/README.md` (reglas de compatibilidad y transporte).
