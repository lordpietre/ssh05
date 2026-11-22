# Auditoría de Fase 0 — de `trzsz-ssh` a ssh05

| Campo | Valor |
|---|---|
| Fecha | 21/11/2026 |
| Repositorio | `lordpietre/trzsz-ssh` |
| Commit auditado | `6e98b8d` |
| Referencia | `SSH05_SDD_v0.3.md` |
| Alcance | Inventario, reutilización, huecos frente al SDD, seguridad y línea base de compilación |
| Resultado | Fase 0 ejecutada; prototipo PTY/OpenTUI reproducible en `prototype/terminal` |

> Snapshot histórico del commit auditado. El estado actual de integración vive en `docs/integration/status.md`.
>
> Esta auditoría describe el código **tal como está** en el commit indicado. No
> presupone que las funcionalidades existentes cumplan los criterios de
> aceptación del SDD: solo identifica puntos de partida y riesgos.

## 1. Método

1. Lectura de entrada, configuración, autenticación, hosts, FRP, túneles y
   utilidades de terminal.
2. Contraste requisito por requisito con el capítulo 2 del SDD.
3. Compilación y pruebas con `CGO_ENABLED=0`, Go 1.25.0.
4. Prototipo aislado con Bun 1.4.5 + `@opentui/core` 0.8.2 para validar el
   supuesto crítico de Fase 0: alojar terminales VT interactivas y propagar
   resize hasta un PTY y un servidor SSH real.

## 2. Inventario

| Área | Archivos principales | Líneas aprox. | ¿Reutilizable? |
|---|---|---:|---|
| Entrada CLI | `cmd/ssh05/main.go`, `ssh05/main.go`, `ssh05/args.go` | 700 | Aprovechable con adaptación: semántica OpenSSH, no la TUI |
| Login SSH | `ssh05/ssh.go`, `ssh05/login.go`, `ssh05/auth_method.go` | 1.800 | **Sí**: motor `golang.org/x/crypto/ssh`, PTY, señales, ProxyJump |
| Inventario | `ssh05/config.go`, `ssh05/effective_config.go`, `ssh05/prompt.go` | 2.000 | **Sí**: parser `~/.ssh/config`, `ssh -G`, hosts |
| Known hosts | `ssh05/known_hosts.go` | 240 | **Sí**: verificación y aviso de clave cambiada |
| Auth extra | `ssh05/agent.go`, `ssh05/keys.go`, `ssh05/otp.go`, `ssh05/krb5.go` | 1.400 | **Sí**: agent, claves, TOTP/OTP, GSSAPI |
| TUI actual | `ssh05/host_model.go`, `ssh05/theme.go`, `ssh05/lang.go` | 5.100 | No para OpenTUI; sí ideas de UX y datos |
| FRP | `ssh05/frp_proxy.go`, `ssh05/frp_download.go` | 1.800 | Aprovechable con adaptación: flujo completo, carencias corregidas después |
| Túneles SSH | `ssh05/forward*.go`, `ssh05/tunnel*` dentro de `host_model.go` | 1.500 | **Sí** como base del supervisor |
| Terminal/tmux | `ssh05/tmgr*.go`, `ssh05/tmuxcc.go`, `ssh05/attach.go` | 1.100 | Aprovechable con adaptación: tmux local y attach de tsshd |
| UDP/QUIC/KCP | `ssh05/udp.go` (tsshd) | 900 | Evaluar en Fase 4, no P0 |
| Transferencia | `ssh05/trzsz.go`, `ssh05/tools_upload.go` | 900 | **Sí** como adaptador P2 |
| Tablas internas | `internal/table`, `internal/krb5`, `internal/ssh` | 2.700 | Sí, con revisión de licencia |

Total: 78 archivos Go, ~26.500 líneas; 14 archivos de prueba.

## 3. Contraste con el SDD

| Requisito | Estado real | Evidencia / hueco |
|---|---|---|
| SES-01…04 multipestañas y PTY | **No cumple** | La TUI elige un host y lanza el login; no hay `TabStore`, ni emulador VT, ni sesiones concurrentes dentro de la interfaz. El batch abre ventanas/pestañas en tmux/iTerm2/WT, no en ssh05. |
| SES-05 estados | Brecha en commit auditado | Hay estados de UI para escaneo/túnel, no una máquina `connecting → connected → degraded…` por sesión. |
| SES-06 reconexión | Brecha en commit auditado | `--reconnect` relanza el proceso; no hay política por sesión ni cancelación desde UI. |
| SES-07/08 tmux | Brecha en commit auditado | `attach.go` reengancha sesiones tsshd; `tmgr_tmux.go` abre tmux local. No hay `TmuxBinding` por pestaña ni restauración de workspace. |
| INV-01…05 inventario | Brecha en commit auditado | Hosts desde `~/.ssh/config` y edición inline; no KDBX, deduplicación ni provenance. Grupos/etiquetas existen de forma limitada. |
| AUT-01…05 auth | Brecha en commit auditado | Agent, claves, password, keyboard-interactive, TOTP/OTP. No KeePassXC agent ni KDBX. Las contraseñas se guardan cifradas (`encPassword`), no en claro. No hay bloqueo por inactividad. |
| FRP-01…08 | Brechas y riesgos en commit auditado | FRP existe (`frp_proxy.go`), pero solo proxies TCP, token con fallback estático, directorio `0755`, token en JSON plano y sin rotación/verificación de actualización. Ver §5. |
| RUT-01…03 rutas | Brecha en commit auditado | ProxyJump multihop funciona; no hay `RoutePolicy` ni resolver que elija y explique candidatos. |
| UI-01…04 OpenTUI | **No cumple** | Bubbletea v2, no OpenTUI; sin pestañas, paleta de comandos ni atajos configurables. |
| OPS-01/02 workspaces | **No cumple** | Sin workspaces ni supervisor persistente; `onExitFuncs` limpia procesos del proceso actual. |
| DIA-01…03 diagnóstico | No cumple | Solo DNS personalizado/SRV; sin Nmap, GeoIP, ASN ni RDAP. |
| DAT-01 transferencias | Brecha en commit auditado | trzsz/Zmodem, upload/download; falta SFTP/SCP como motor desacoplado. |
| SEG-01 host key | Cumple el bloqueo de cambios | `known_hosts.go` avisa y bloquea cambios; `accept-new`/`no` conserva la semántica OpenSSH de aceptar claves nuevas. |
| SEG-02 inyección/secretos | Resuelto (H-05) | `frpCopyToRemote` usa argumentos citados; los comandos remotos restantes se revisaron en las fases posteriores. |
| SEG-03/04/05 | No cumple | Sin IPC autenticado, auditoría estructurada ni pruebas adversariales. |

## 4. Activos reutilizables para el supervisor

1. **Motor SSH**: `SshLogin` devuelve una interfaz `SshClient` con sesiones, PTY,
   `WindowChange`, canales y forwarding (`ssh05/ssh.go:208`).
2. **Autenticación**: agent, claves, certificados, keyboard-interactive, OTP,
   GSSAPI y `ProxyJump` ya resueltos (`ssh05/auth_method.go`, `ssh05/login.go`).
3. **Identidad**: `known_hosts.go` usa `skeema/knownhosts`, bloquea cambios de
   clave y conserva `HostKeyAlgorithms`.
4. **Configuración**: parser de `~/.ssh/config`, `Include`, `Match` y `ssh -G`
   (`effective_config.go`), base del inventario.
5. **FRP**: generación de TOML, arranque/parada y persistencia; sirve como
   referencia, pero debe endurecerse antes de reutilizarse.
6. **Transferencia**: filtro trzsz/Zmodem y utilidades de subida/descarga.

## 5. Hallazgos de seguridad y deuda prioritaria

| ID | Hallazgo | Ubicación | Impacto | Acción propuesta |
|---|---|---|---|---|
| H-01 | `generateToken()` devolvía un token fijo si falla `crypto/rand` | `ssh05/frp_proxy.go:179-185` | Viola FRP-03; token predecible | **Resuelto:** ahora devuelve `(string, error)`, falla cerrado ante error/short read y hay tests en `ssh05/frp_proxy_test.go` |
| H-02 | Directorio `~/.ssh05` creado con `0755` | `ssh05/frp_proxy.go` | Viola FRP-04; otros usuarios pueden listar | **Resuelto:** `ensurePrivateDir` crea/endurece a `0700`; test incluido |
| H-03 | Token FRP persistido en `~/.tssh_frp` (JSON) y TOML | `frpSaveConfig`, `writeFrpcConfig` | Secreto en claro fuera de un proveedor | **Resuelto:** `Token` no se serializa; se cifra AES-GCM con `~/.tssh/frp.key` (`0600`) y los TOML remotos se copian con `0600`. Riesgo residual: un atacante con la misma UID puede leer la clave, igual que un keyring desbloqueado |
| H-04 | Solo proxies `type = "tcp"` | `ssh05/frp_proxy.go` | No cubre UDP/STCP/XTCP (FRP-02) | **Resuelto:** UI/config admiten los 4 tipos; `frpc verify` los acepta y los túneles reales TCP/STCP/XTCP/UDP funcionan |
| H-05 | `frpCopyToRemote` construye `mkdir -p … && cat > …` sin citar rutas | `ssh05/frp_proxy.go` | Inyección de comandos si la ruta es controlable | **Resuelto:** `sh -c` con argumentos citados, expansión de `~/` en el script y tests de inyección/rutas con espacios |
| H-06 | Claves nuevas se aceptan automáticamente con `accept-new`/`no` | `ssh05/known_hosts.go:199-216` | Comportamiento OpenSSH esperado, pero el SDD pide flujo explícito | No cambiar el default; añadir UX explícita en ssh05 |
| H-07 | Sin bloqueo de secretos por inactividad | `ssh05/config.go` (cifrado AES-GCM) | AUT-05 pendiente | Diseñar vault y temporalidad |
| H-08 | Prueba `TestTableExample` falla por color en el entorno | `internal/table/table_test.go` | Línea base ruidosa | Arreglar aparte; no relacionado con Fase 0 |

## 6. Licencias y atribución

- Repositorio y código propio: MIT (cabeceras en `ssh05/*.go`).
- `internal/table`: MIT, Copyright Charmbracelet.
- `internal/krb5/gssapi.go`: Copyright 2022-2025 `wencaiwulue`; revisar términos.
- `internal/ssh/bcrypt_pbkdf.go`: Copyright The Go Authors (BSD).
- Dependencias Go: MIT/BSD/Apache-2.0; `go.sum` fija versiones.
- OpenTUI: MIT; Binarios nativos por plataforma (`@opentui/core-linux-*`).
- FRP: Apache-2.0 (solo ejecutables oficiales).
- KeePassXC/KDBX y datos GeoIP requieren revisión específica antes de distribuir.

## 7. Línea base de compilación y pruebas

```text
Go 1.25.0, CGO_ENABLED=0
go build -o /tmp/opencode/ssh05 ./cmd/ssh05   -> OK
go test -count=1 ./ssh05                     -> ok (1.0 s)
go test -count=1 ./internal/table           -> FAIL preexistente (TestTableExample, color)
```

El fallo de `internal/table` no lo introduce la Fase 0; se documenta para no
confundirlo con una regresión. `go test ./ssh05` pasa.

## 8. Conclusión

El repositorio actual es una base útil para el **supervisor** (SSH, auth,
known_hosts, config, FRP y transferencia), pero **no** para la interfaz ni el
modelo de sesiones del SDD. La Fase 0 confirma que OpenTUI + Bun PTY puede
alojar `vim`, `htop`, `tmux` y una sesión SSH real con resize. Los riesgos que
bloquean fases posteriores son los de FRP (H-01…H-05), la ausencia de IPC y la
falta de pestañas/supervisor. No se recomienda iniciar la Fase 1 sin resolver
H-01 y H-05 y sin cerrar ADR-01/ADR-03.

Estado de la integración posterior a esta auditoría:

- **H-01 resuelto**: token CSPRNG con fallo cerrado y tests.
- **H-02 resuelto**: directorio de configuración FRP en `0700`.
- **H-03 resuelto**: token FRP cifrado en reposo con clave local `0600`; TOML remoto `0600`; riesgo residual documentado.
- **H-04 resuelto**: 4 tipos validados por `frpc verify` y túneles reales TCP/STCP/XTCP/UDP.
- **H-05 resuelto**: copia remota FRP con argumentos citados y tests de inyección.
- **ADR-01 cerrada**: motor SSH Go aceptado, con contrato `contracts.SessionBackend`.
- **Contratos definidos**: `contracts/` (Go) y `contracts/ts/` (TypeScript).
- Pendiente: H-02/H-03/H-04, ADR-03 (IPC implementado) y el supervisor de la Fase 1.
