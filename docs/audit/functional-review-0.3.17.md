# Revisión funcional completa — ssh05 0.3.17

| Campo | Valor |
|---|---|
| Fecha | 22/11/2026 |
| Repositorio | https://github.com/lordpietre/ssh05 |
| Documento base | `TRSS_SDD_v0.2.md` |
| Alcance | Todos los requisitos P0, P1 y P2 |
| Estado global | **Aprobado con limitaciones documentadas** |

## Método

Se revisaron los requisitos del SDD contra el código, las pruebas y los smokes
reales. No se acepta una función por existir solo en documentación: cada fila
apunta a código, test o evidencia de ejecución. Las limitaciones que dependen
del entorno se marcan explícitamente.

Comandos ejecutados:

```sh
CGO_ENABLED=0 go test ./ssh05 ./contracts ./internal/ipc ./internal/supervisor \
  ./internal/audit ./internal/frp ./internal/transfer
make verify
make deb
make trssd-frp-smoke
```

Resultado local: tests, tipos, UI smoke y smoke FRP correctos. Los `.deb`
generados incluyen `ssh05` y `trssd`.

## P0

| ID | Estado | Evidencia / límite |
|---|---|---|
| SES-01 | Cumple | Pestañas independientes, `TabStore`, PTY por sesión; `make ui-smoke` |
| SES-02 | Cumple | Cambio de pestaña sin cerrar procesos; `ui-smoke` |
| SES-03 | Cumple | Diálogo con cerrar/desconectar/desacoplar tmux; `closeChoices` |
| SES-04 | Cumple | PTY, resize, teclado y señales; tests de resize y smoke real |
| SES-05 | Cumple | `connecting`, `connected`, `degraded`, `reconnecting`, `disconnected`, `failed`, `closed`; `TestBackendReconnectExhaustionFails` y `TestBackendReconnectsWithBackoff` |
| INV-01 | Cumple | Inventario canónico sin secretos; `internal/supervisor/inventory.go` |
| AUT-01 | Cumple | Agente OpenSSH, claves y `StrictHostKeyChecking`; `hostkey-smoke` |
| UI-01 | Cumple | UI TypeScript/Bun + OpenTUI; `prototype/terminal` |
| SEG-01 | Cumple | Host key estricta; clave cambiada bloqueada |
| SEG-02 | Cumple | IPC tipado, sin shell, auditoría saneada, rutas SCP restringidas |
| PTY acotado | Cumple | `--max-pending-bytes` y pruebas de buffer |
| Recuperación UI | Cumple | `sessions.list` + `sessions.attach`; `ui-smoke` |

## P1

| ID | Estado | Evidencia / límite |
|---|---|---|
| SES-06 | Cumple | Backoff configurable, límites y cancelación; tests |
| SES-07 | Cumple | tmux opcional, nombre `trss-<alias>`, activación explícita `Ctrl+G` |
| SES-08 | Cumple | `TmuxCreate=false` + `tmux has-session`; si falta, shell y se borra el binding |
| INV-02 | Cumple | `Include`, aliases y opciones vía `ssh_config`; `TestSSHHostsInclude` |
| INV-03 | Cumple | Importación KDBX por `keepassxc-cli` |
| INV-04 | Cumple | Deduplicación, procedencia y conflictos sin sobrescritura |
| INV-05 | Cumple | `#!!` + overrides persistentes `F2/F3/F4`; `TestInventoryHostOverridesPersist` |
| Alta de hosts | Cumple | `hosts.create`, formulario `Ctrl+N`, guarda en `ssh_config` |
| AUT-02 | Cumple | `SSH_AUTH_SOCK` + `ssh-add -l`; solo huellas, sin claves privadas |
| AUT-03 | Cumple | `AuthRef` estable por UUID, sin secreto |
| AUT-04 | Cumple | Sin secretos en perfiles/eventos; auditoría saneada |
| AUT-05 | Cumple | `--vault-idle-timeout` y `vault.unlockRequest` |
| FRP-01 | Cumple | `frpc` y `frps` gestionados; `frp.servers.start/stop` |
| FRP-02 | Cumple | Crear/editar/validar/eliminar TCP, UDP, STCP y XTCP |
| FRP-03 | Cumple | CSPRNG y fallo seguro; `TestManagerFailsClosedWithoutEntropy` |
| FRP-04 | Cumple | AES-GCM, `0600/0700`; `TestManagerPrivatePermissions` |
| FRP-05 | Cumple | Estado, destino, puerto, PID, uptime y errores |
| FRP-06 | Cumple | Rotación reinicia el `frps` gestionado; remoto se rechaza y se recupera con `--frp-token-force` |
| FRP-07 | Cumple con límite | Hash fijado y `verify`; no hay auto-actualización de binarios |
| FRP-08 | Cumple | Cerrar pestaña no detiene el túnel; `TestClosingSessionKeepsFRPTunnel` |
| RUT-01 | Cumple con límite | Directo, `ProxyJump` y FRP según inventario; `RoutePolicy` no se usa para denegar rutas |
| RUT-02 | Cumple | `routes.probe`; timeout no significa cerrado |
| RUT-03 | Cumple | Ruta visible; FRP público se muestra al crear el proxy |
| UI-02 | Cumple | RGB con degradación 256/16 y `NO_COLOR`/mono |
| UI-03 | Cumple | Icono + texto de estado; color no decide el significado |
| UI-04 | Cumple | Búsqueda, paleta y `keybindings.json` |
| OPS-01 | Cumple | Workspaces, pestañas y tmux persistentes |
| OPS-02 | Cumple | Sesiones/túneles separados; grupos de proceso y `Pdeathsig` |
| SEG-03 | Cumple con límite | Socket `0700/0600` y credenciales de par en Linux; el daemon IPC no soporta Windows |
| SEG-04 | Cumple | Auditoría con rotación y retención |
| SEG-05 | Cumple | Fuzz de framing, inyección, host key y reconexión |

## P2

| ID | Estado | Evidencia / límite |
|---|---|---|
| DIA-01 | Cumple | `diagnostics.host`: DNS, TCP, banner, latencia; smoke real |
| DIA-02 | Cumple con entorno | `diagnostics.nmap` exige confirmación/autorización; requiere `nmap` instalado |
| DIA-03 | Cumple con límite | ASN por Team Cymru y RDAP; país orientativo, sin ciudad precisa |
| DAT-01 | Cumple con límite | SCP/SFTP por supervisor y UI. `trzsz`/Zmodem sigue disponible en el cliente `ssh05` interactivo, pero no se expone por la API de transferencias |

## Requisitos no funcionales

| Requisito | Estado | Evidencia / límite |
|---|---|---|
| Fiabilidad entre pestañas | Cumple | Sesiones aisladas; smoke de 10 pestañas y recuperación |
| Seguridad por defecto | Cumple | Host key estricta, IPC privado, auditoría saneada |
| Compatibilidad UTF-8/True Color | Cumple | OpenTUI + degradación de color |
| Diagnóstico estructurado | Cumple | `diagnostics.report`, `diagnostics.host`, Nmap, ASN/RDAP |
| Exportar informe saneado | Cumple | `diagnostics.report` + `src/diagnostics/export.ts` |
| Operaciones cancelables | Cumple | Transferencias, reconexión y timeouts |
| Límites de buffers | Cumple | `--max-pending-bytes`, `--max-sessions` |
| Preferencias persistentes | Cumple | ancho, densidad, pestañas y orden |
| Rendimiento | Cumple | `make bench`, `docs/performance.md` |

## Límites explícitos

1. **OpenTUI no está dentro del `.deb`.** El paquete instala `ssh05` y `trssd`
   y una unidad `trssd.service`; la UI OpenTUI requiere Bun y se ejecuta desde
   `prototype/terminal`. No se empaqueta porque OpenTUI necesita runtime Bun y
   dependencias nativas, y el binario único de Bun no resuelve el módulo nativo
   musl/glibc de forma portable.
2. **trzsz/Zmodem** no se ofrece en `transfers.start`; sigue en el cliente
   interactivo clásico. La API del supervisor usa SCP/SFTP.
3. **GeoIP** se limita al país del registro RDAP/ASN; no se inventa ciudad.
4. **XTCP** de extremo a extremo depende de STUN/NAT; el smoke lo marca como
   condicional. TCP, UDP y STCP son deterministas.
5. **Windows**: `ssh05` compila, pero el supervisor IPC es Unix; los tests de
   integración se ejecutan en Linux.
6. **Docker** no aparece por defecto en el supervisor; `--docker` lo habilita.

## Conclusión

No hay requisitos P0/P1/P2 sin revisar. Las funciones comprometidas están
implementadas y probadas; las excepciones son límites técnicos explícitos, no
código a medias. El paquete `.deb` incluye ahora el cliente y el supervisor.
