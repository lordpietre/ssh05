# ADR-04: Gestor de secretos y compatibilidad KDBX

- **Estado:** Aceptado
- **Fecha:** 21/11/2026
- **Requisitos afectados:** AUT-02…05, INV-03, SEG-02/03

## Contexto

El SDD exige importar perfiles y credenciales desde KeePass/KeePassXC sin
persistir secretos en los perfiles, y usar el agente SSH de KeePassXC cuando se
autentique con clave. El código actual guarda contraseñas cifradas con AES-GCM
en `~/.ssh/password` (`encPassword`) y no lee KDBX.

## Alternativas

1. **KeePassXC como proveedor**: agente SSH + CLI/API autorizada.
   - Ventaja: no se parsea KDBX ni se extrae la clave privada; el desbloqueo y
     la memoria quedan en KeePassXC.
   - Riesgo: depende de que el usuario tenga KeePassXC y de su versión/API.
2. **Parser KDBX directo** (por ejemplo, una biblioteca Go de KDBX).
   - Ventaja: importación sin depender de KeePassXC.
   - Riesgo: criptografía, versiones KDBX, memoria de secretos, superficie de
     ataque y auditoría de la biblioteca.
3. **Solo secretos cifrados propios** (modelo actual).
   - Ventaja: ya existe.
   - Riesgo: no cumple el flujo KeePass del SDD; no hay bloqueo por inactividad.

## Decisión

Adoptar una estrategia por capas:

1. **KeePassXC SSH Agent** para autenticación por clave, sin extraer la clave
   privada.
2. **KeePassXC CLI/API** para localizar entradas y obtener contraseñas bajo
   demanda, con autorización y tiempo de exposición limitados.
3. **Importación KDBX directa solo si** una evaluación de biblioteca, formato y
   amenazas lo aprueba; nunca como primer paso.

Los perfiles guardan únicamente `CredentialReference`
(`provider`, `entryId`, `fingerprint` no secreto). No se registran valores en
logs, argv ni variables de entorno.

## Consecuencias

- El vault no forma parte del modelo de perfil; es un proveedor externo.
- La UI pide desbloqueo cuando corresponda y muestra el estado bloqueado.
- Hay que definir caducidad de secretos en memoria y bloqueo por inactividad.
- La compatibilidad KDBX queda fuera de la Fase 1 si KeePassXC no está presente.

## Implementación actual

- `internal/supervisor/keepass.go` importa KDBX vía `keepassxc-cli`.
- Soporta key file y `--keepass-password-command` (askpass); el secreto no se
  escribe en disco ni en argv.
- Solo se importan metadatos no secretos: URL, usuario, host, puerto, notas.
- `HostProfile.AuthRef` apunta a `keepassxc`/UUID; no contiene contraseña.
- El agente SSH de KeePassXC se usa a través de `SSH_AUTH_SOCK` del cliente SSH.

## Validación pendiente

- Importar host, puerto, usuario y URL `ssh://` sin iniciar conexiones.
- Cero secretos en perfiles, logs y snapshots.
- Entrada eliminada: desvincular o conservar como perfil manual, sin borrar
  trabajo del usuario.
- Conflicto KDBX/`ssh_config` sin sobrescritura silenciosa.
- Bloqueo de KeePassXC no sorteado por reconexión automática.

## Evidencia

- `docs/audit/phase-0-audit.md` §3 (AUT/INV).
- `ssh05/config.go` (cifrado AES-GCM de `encPassword`).
- SDD §7 y §19.3.
