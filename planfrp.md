# Plan: Mejora del sistema FRP Proxy en ssh05

## Problemas identificados (revisión 2026-07-05)

### P1 — Bug: escaneo R2L envía resultados a `m.tunnel` en vez de `m.frp`
`tunnelScanRemote()` devuelve `tunnelScanResultMsg`, pero el Update handler de FRP
espera `frpScanResultMsg`. Los resultados del scan remoto se pierden en el estado
equivocado y nunca llegan a la UI de FRP.

### P2 — Sin fallback si el scan de puertos falla o está vacío
Si `ss`/`netstat` no están en el sistema, o no hay puertos escuchando, el usuario
se queda en una pantalla de error sin posibilidad de introducir datos manualmente.

### P3 — Puertos fijos sin detección de conflicto
- `bindPort = 7000` hardcodeado — si está ocupado, `frps` falla al bindear
- `webServer.port = 7400` hardcodeado — mismo problema
- No hay intento de buscar puerto libre alternativo

### P4 — Validación de entrada insuficiente
- Nombre del proxy: no se valida que no contenga `"` (rompe TOML)
- Puerto expuesto: no se valida contra duplicados
- Errores de save/load de configuración se tragan silenciosamente
- ServicePort de entrada de usuario se ignora silenciosamente

### P5 — Token no persistido en `frpProxyEntry`
El token de autenticación se genera durante el setup pero no se guarda en los
entries persistidos. En operaciones de reload/restart el token se pierde.

---

## Plan de implementación

### S1 — Arreglar bug de scan remoto (R2L)
**Archivo:** `ssh05/frp_proxy.go`
**Cambio:** Crear `frpScanRemotePorts(alias)` que hace SSH y parsea puertos,
retornando `frpScanResultMsg` directamente. Reemplazar `tunnelScanRemote()` en
el flujo R2L de `host_model.go:590`.

### S2 — Fallback manual cuando scan falla o vacío
**Archivo:** `ssh05/frp_proxy.go`
**Cambio:** En el handler de `frpScanResultMsg`, si `err != nil` o `len(ports) == 0`,
en vez de mostrar error, ir a vista de entrada manual (`manual_setup`) con campos
para:
  - Service port (numérico, obligatorio)
  - Proxy name (texto, default `"proxy-<port>"`)
  - Exposed port (numérico, default = service port)

### S3 — Detección de puerto libre
**Archivo:** `ssh05/frp_proxy.go`
**Cambio:** Crear `findFreePort(startPort)` que prueba conectividad TCP a
`127.0.0.1:port` desde `startPort` hacia arriba hasta encontrar uno libre.
Usarlo en `frpSetupCmd` en vez del hardcode `kFrpDefaultPort=7000`.
Aplicar también para `kFrpAdminPort=7400`.

### S4 — Validación de entrada
**Archivo:** `ssh05/frp_proxy.go`
**Cambio:**
- `handleFrpConfirm`: validar que `proxyName` no contenga `"`
- Validar que `exposedPort` sea número > 0
- Validar que `exposedPort` no esté ya en uso por otro proxy del mismo alias
- Mostrar errores de validación en la UI (`m.frp.setupErr`)
- `frpSaveConfig`: retornar error visible al usuario
- `frpLoadConfig`: retornar error visible al usuario

### S5 — Persistir token en entries
**Archivo:** `ssh05/frp_proxy.go`
**Cambio:** Guardar `entry.Token = m.frp.token` al crear el proxy en
`handleFrpConfirm`. El token se genera en `frpSetupCmd` y se almacena
en `m.frp` para ser usado después.

### S6 — Feedback de errores al usuario
**Archivo:** `ssh05/frp_proxy.go`
**Cambio:**
- `frpSaveConfig`: retornar `error`, mostrar en UI si falla
- `frpLoadConfig`: retornar `error`, mostrar en UI si falla
- Los handlers de `frpStartResultMsg`/`frpStopResultMsg` deben mostrar error

---

## Testing

1. `CGO_ENABLED=0 go build -o ./bin/ ./cmd/ssh05`
2. `CGO_ENABLED=0 go test -v -count=1 ./ssh05`
3. Probar R2L con host real
4. Probar L2R con host real
5. Probar fallback manual (simular `ss` no disponible)
6. Probar puerto 7000 ocupado → debe saltar a 7001
