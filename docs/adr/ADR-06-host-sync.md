# ADR-06: Sincronización del inventario de hosts

- **Estado:** Propuesto
- **Fecha:** 21/11/2026
- **Requisitos afectados:** INV-01…05, RUT-01

## Contexto

El inventario actual se deriva de `~/.ssh/config` y de ediciones inline. El SDD
exige procedencia por valor, deduplicación, resolución de conflictos y que una
fuente externa no sobrescriba cambios locales. También exige que un host
importado figure como descubierto/no verificado hasta comprobar identidad.

## Alternativas

1. **Perfil canónico con provenance por campo** (recomendada).
2. **Copiar la configuración SSH** a una base propia.
   - Riesgo: duplica la verdad y pierde cambios externos.
3. **Leer siempre de `ssh_config`** sin base propia.
   - Riesgo: no permite ediciones locales, etiquetas ni conflictos.

## Decisión propuesta

Base canónica `HostProfile` con:

- `id` estable (UUID propio, nunca el alias);
- `sourceRefs[]` por fuente (`manual`, `ssh_config`, `keepass`) y por campo
  cuando sea necesario;
- reglas de fusión: manual gana; cambios externos se muestran como conflicto,
  no se aplican en silencio;
- `verifiedIdentity` separado de `source` (un host importado no es un host
  verificado);
- `deleted` como tombstone para no resucitar entradas eliminadas.

La importación es de solo lectura hasta que el usuario acepta; la sincronización
posterior muestra diferencias y permite separar entradas fusionadas.

## Consecuencias

- El modelo de datos crece; hay que versionar migraciones.
- La UI debe explicar de dónde viene cada valor y qué cambió.
- El resolver de rutas consume el perfil canónico, no `ssh_config` en crudo.

## Validación pendiente

- Importar `Include`, alias y `Match` sin perder opciones aplicables.
- Fusionar `ssh_config` + KDBX + manual conservando procedencia.
- Edición local no sobrescrita por una fuente externa.
- Entrada eliminada en la fuente: desvincular/ conservar, sin borrar trabajo.
- Deduplicación por usuario+puerto+host+ruta, no solo por `hostname`.

## Evidencia

- `tssh/config.go`, `tssh/effective_config.go`.
- SDD §6, INV-01…05.
- Auditoría §3.
