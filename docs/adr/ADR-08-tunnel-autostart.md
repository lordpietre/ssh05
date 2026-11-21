# ADR-08: Autoarranque de túneles

- **Estado:** Aceptado
- **Fecha:** 21/11/2026
- **Requisitos afectados:** FRP-06/07/08, RUT-01/03, SEG-03

## Contexto

El SDD exige que iniciar un túnel o exponer un puerto sea una acción explícita,
con resumen de impacto, y que una ruta FRP ausente muestre el requisito sin abrir
una alternativa pública silenciosa. Un workspace puede recordar túneles, pero
eso no equivale a arrancarlos sin consentimiento.

## Alternativas

1. **Sin autoarranque** (todo manual).
2. **Autoarranque por workspace con consentimiento previo**.
3. **Autoarranque global**.

## Decisión

Por defecto **no autoarrancar** túneles. Un workspace puede declarar túneles
previstos y el usuario autoriza, por workspace y por túnel:

- iniciar al restaurar el workspace;
- mantener vivo el túnel al cerrar pestañas;
- detenerlo al cerrar el workspace (opcional).

Nunca se autoexponen puertos TCP públicos. Para STCP/XTCP se explica el
visitante autorizado. Toda acción que publique un puerto muestra interfaz,
puerto resultante y confirmación informada.

## Consecuencias

- La restauración de workspace es predecible y no sorprende con exposiciones.
- La UI necesita un paso de confirmación y un registro saneado.
- El resolver de rutas puede pedir “iniciar túnel” como acción, no hacerlo solo.

## Validación pendiente

- Workspace con túnel detenido: no se abre SSH hasta confirmar.
- Cierre de pestaña: el túnel sigue según política.
- Exposición TCP pública: confirmación obligatoria.
- Fallo de `frpc`: diagnóstico específico, sin ruta pública alternativa.
- Rotación de token y recuperación visibles.

## Evidencia

- SDD §8.3, §9, §19.1.
- `docs/adr/ADR-05-frp-ownership.md`.
