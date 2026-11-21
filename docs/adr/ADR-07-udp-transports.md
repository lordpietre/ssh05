# ADR-07: Transportes UDP opcionales

- **Estado:** Aceptado (fuera de P0)
- **Fecha:** 21/11/2026
- **Requisitos afectados:** SES-06/07, RUT-01; no P0

## Contexto

El repositorio actual soporta modo UDP con `tsshd`, QUIC y KCP. El SDD lo cita
como opción de evaluación separada y advierte que no se equipara con tmux ni se
introduce sin validar compatibilidad, requisitos del servidor y modelo de
amenaza.

## Alternativas

1. **Reutilizar `tsshd`/QUIC/KCP** del código existente.
2. **Mosh** como transporte externo.
3. **No incluir UDP** en el núcleo inicial.

## Decisión

UDP/QUIC/KCP **no forma parte del P0**; se evaluará como adaptador separado después de la Fase 4. No forma parte
del P0 ni de la primera UI multipestaña. Si se retoma, se evaluará como
adaptador de transporte con identidad propia, no como sustituto de tmux.

Criterios para retomarlo:

- servidor y cliente con versión compatible y despliegue documentado;
- modelo de amenaza y pruebas de roaming/pérdida;
- comportamiento claro al cerrar pestaña versus conservar sesión;
- no degradar la verificación de host ni la autenticación.

## Consecuencias

- La Fase 1 no depende de UDP.
- El código existente (`tssh/udp.go`) se conserva como referencia, no como
  requisito de arquitectura.
- La documentación debe evitar presentar UDP como persistencia remota.

## Validación pendiente

- Compatibilidad de versiones y binarios.
- Roaming, pérdida de paquetes y reconexión.
- Interacción con tmux y con el supervisor.
- Coste de soporte en Linux/macOS/Windows.

## Evidencia

- `tssh/udp.go`, `tssh/attach.go`.
- SDD §5 y ADR-01.
