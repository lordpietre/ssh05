# ADR-05: Propiedad de procesos FRP

- **Estado:** Propuesto
- **Fecha:** 21/11/2026
- **Requisitos afectados:** FRP-01…08, OPS-02, SEG-03

## Contexto

El SDD separa el ciclo de vida de un túnel del de una pestaña. Cerrar una vista
no debe matar `frpc`/`frps` si atienden otras rutas. El código actual arranca
procesos hijos desde la TUI (`frpStartLocal`) y los limpia al salir
(`frpCleanupLocalProcesses`), lo que acopla túnel y proceso de UI.

## Alternativas

1. **Hijos del supervisor**, con grupos de proceso y reconciliación al
   reiniciar.
   - Ventaja: control total, logs, parada explícita y aislamiento.
   - Riesgo: si el supervisor cae, hay que detectar y adoptar/matar huérfanos.
2. **Servicio del sistema** (systemd/launchd/Windows Service).
   - Ventaja: persistencia real y arranque al boot.
   - Riesgo: instalación privilegiada, permisos y portabilidad.
3. **Procesos hijos de la UI** (modelo actual).
   - Ventaja: simple.
   - Riesgo: contradice FRP-08 y OPS-02.

## Decisión propuesta

El **supervisor es el propietario por defecto** de `frpc`/`frps`:

- Los procesos se lanzan en un grupo/sesión propio y se registran con PID,
  arranque, configuración y hash de la misma.
- Al reiniciar, el supervisor reconcilia: adopta procesos propios vivos o los
  marca como huérfanos; nunca asume que siguen sanos.
- Cerrar una pestaña o la UI **no** detiene túneles; detenerlos requiere una
  acción explícita `frp.proxies.stop`.
- Se ofrece un modo “servicio externo” por endpoint, en el que TRSS solo
  supervisa y no escribe la configuración sin confirmación.
- Los secretos se referencian, no se copian al argv.

## Consecuencias

- Hay que implementar un supervisor con ownership y reconciliación antes de
  prometer FRP-08.
- La UI muestra estado, destino, puerto y errores saneados, no tokens.
- El cierre abrupto del supervisor deja un caso de recuperación documentado.

## Validación pendiente

- Cerrar pestaña y UI: el túnel sigue activo.
- Matar `frpc`: estado `error/degraded` visible y reintento acotado.
- Reinicio del supervisor: sin duplicar procesos ni matar túneles ajenos.
- Permisos de configs `0600`, directorios `0700`; token fuera de logs.
- Rotación de token no presentada como atómica si no lo es.

## Evidencia

- `tssh/frp_proxy.go` (`frpStartLocal`, `frpStopProcess`,
  `frpCleanupLocalProcesses`).
- SDD §8.2 y FRP-08.
- Auditoría H-01…H-05.
