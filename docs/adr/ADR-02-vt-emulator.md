# ADR-02: Terminal embebido y PTY

- **Estado:** Aceptado (prototipo)
- **Fecha:** 21/11/2026
- **Requisitos afectados:** SES-02/04, UI-01/02, UIX-02/05
- **Versiones probadas:** Bun 1.4.5, `@opentui/core` 0.8.2, Linux x86_64

## Contexto

El SDD §17 propone `EmbeddedTerminalRenderable` de OpenTUI y
`Bun.spawn({ terminal })` para el prototipo A. La duda era si un renderable de
OpenTUI puede alojar una terminal VT real sin romper aplicaciones interactivas,
y si el resize llega al PTY y al servidor SSH.

## Decisión

Adoptar **OpenTUI 0.8.2 + PTY nativo de Bun** para el prototipo y para la
primera UI de ssh05, manteniendo `SessionBackend` como contrato estable para
poder sustituir el backend sin cambiar `TabStore`.

El prototipo vive en `prototype/terminal` y demuestra:

- interpretación ANSI, alt-screen, cursor y Unicode;
- entrada de teclado y pegado básico hacia el PTY;
- resize del panel → `EmbeddedTerminalRenderable.onResize` → `Terminal.resize`
  → `SIGWINCH` en el proceso hijo;
- convivencia con `vim`, `htop`, `tmux` y una sesión SSH real.

## Detalles de integración obligatorios

1. El renderable debe usar `width: "100%"`, `flexGrow: 1` y `flexShrink: 1`.
   Si se fija `width`/`height` numéricos, el layout no encoge y `onResize` no
   se dispara (observado en el prototipo).
2. En Bun 1.4.5 el handler interno de OpenTUI puede perder el resize mientras
   hay un frame nativo en vuelo. El prototipo escucha `SIGWINCH`, lee
   `process.stdout.columns/rows` y llama a `renderer.resize(cols, rows)`, que es
   la API documentada para resize externo.
3. El tamaño inicial del PTY debe tomarse de `renderer.width` y
   `renderer.height - 2` (barra de pestañas + estado), no de 80×24.
4. El foco se obtiene con `terminal.focus()`, no con
   `renderer.focusRenderable()`: este último no registra los handlers de teclado
   del renderable.
5. Los datos con origen `input` y `response` deben reenviarse al PTY; separar
   solo para telemetría.

## Consecuencias

- La UI puede avanzar sin escribir un emulador VT propio.
- La dependencia de OpenTUI queda acotada a la capa de vista; el supervisor Go
  no la conoce.
- El binario nativo de OpenTUI añade matriz de plataformas y empaquetado.

## Validación pendiente (no cerrar la ADR todavía)

- Mouse reporting del programa remoto frente a selección local.
- Copiar/seleccionar texto, bracketed paste y pegado multilínea.
- Scrollback acotado y 100 MB de salida sin bloquear la UI.
- Unicode de doble ancho, emojis y `NO_COLOR`.
- `tmux` remoto, `mc`, `less`, `fzf` y aplicaciones con mouse.
- Linux aarch64/musl, macOS y Windows; binarios por plataforma.
- Estrés de resize y reconexión.

## Evidencia

- `prototype/terminal/src/main.ts`.
- `prototype/terminal/scripts/smoke.sh` (todas las comprobaciones pasan).
- `docs/phase-0/README.md` con la salida reproducible.
