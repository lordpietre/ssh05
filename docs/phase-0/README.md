# Fase 0 — Prototipo PTY / OpenTUI

Este directorio contiene el prototipo que valida la decisión más arriesgada del
SDD 0.2: que una TUI de OpenTUI pueda alojar una terminal VT real y reenviar
teclado y resize a un PTY y a una sesión SSH.

## Estado

| Elemento | Estado |
|---|---|
| Auditoría del código actual | `docs/audit/phase-0-audit.md` |
| ADR-01…ADR-08 | `docs/adr/` |
| Prototipo ejecutable | `prototype/terminal` |
| Smoke test automatizado | `prototype/terminal/scripts/smoke.sh` |
| SSH real | Sí, con un `sshd` desechable en `127.0.0.1:2222` |

## Requisitos

- Bun 1.4.5 (probado) y `@opentui/core` 0.8.2 (fijado en `bun.lock`).
- `tmux`, `vim` y `htop` para el smoke test.
- Opcional: `sshd` y `ssh-keygen` para el caso SSH. El script levanta un servidor
  local como el usuario actual, sin `sudo`.

## Ejecución manual

```sh
cd prototype/terminal
bun install
bun start                                  # $SHELL
bun run src/main.ts -- vim /tmp/prueba.txt
bun run src/main.ts -- htop
bun run src/main.ts -- ssh usuario@host
```

Atajos del prototipo: `F10` cierra (también `Ctrl+Shift+Q` cuando el emulador lo
distingue). `Escape` y `/` se envían al programa remoto; no se interceptan.

## Smoke test

```sh
cd prototype/terminal
bun run smoke
```

o, desde la raíz, `make phase0` (requiere `bun` en `PATH`).

Resultado del 21/11/2026 (Bun 1.4.5, Linux x86_64):

```text
PASS: TUI started (ssh05-smoke-size)
PASS: Unicode output renders in the embedded terminal
PASS: initial PTY size is 120x34
PASS: resize propagates to the child PTY (100x28)
PASS: TUI started (ssh05-smoke-vim)
PASS: vim receives input and renders in the embedded terminal
PASS: vim survives an alt-screen resize
PASS: TUI started (ssh05-smoke-htop)
PASS: htop renders (alt-screen + live updates)
PASS: htop status line is visible
PASS: TUI started (ssh05-smoke-keys)
PASS: keyboard input reaches the child PTY
PASS: Escape and / do not close the TUI
PASS: F10 closes the TUI
PASS: TUI started (ssh05-smoke-tmux)
PASS: tmux runs inside the embedded terminal
PASS: TUI started (ssh05-smoke-ssh)
PASS: SSH shell prompt is visible
PASS: commands run in the real SSH PTY
PASS: SSH PTY starts at 120x34
PASS: SSH window-change reaches the remote PTY
== all checks passed ==
```

Para omitir el caso SSH: `SSH05_PROTO_SKIP_SSH=1 bun run smoke`.

## Qué demuestra

- `EmbeddedTerminalRenderable` interpreta ANSI, alt-screen y Unicode.
- El foco y el teclado llegan al PTY; `vim`, `htop` y `tmux` funcionan.
- `Escape` y `/` se entregan al hijo; `F10` cierra la TUI sin cerrar por error
  una sesión con `Escape`.
- El tamaño inicial del PTY coincide con el layout (120×34 en una ventana de
  120×36).
- Un resize de la ventana llega al hijo (`SIGWINCH`) y a un PTY SSH real
  (100×28).
- La integración no necesita un emulador VT propio.

## Qué no demuestra todavía

- Mouse reporting del programa remoto, selección/copia, bracketed paste.
- Scrollback, 100 MB de salida y backpressure.
- `tmux` **remoto** (el test usa tmux local).
- Linux aarch64/musl, macOS y Windows.
- Empaquetado de los binarios nativos de OpenTUI.

Estos puntos están en la validación pendiente de
[ADR-02](../adr/ADR-02-vt-emulator.md) y no deben presentarse como resueltos.

## Notas de integración observadas

1. **Layout:** el renderable necesita `width: "100%"`, `flexGrow: 1`,
   `flexShrink: 1`. Con dimensiones fijas, el layout no cambia y no se dispara
   `onTerminalResize`.
2. **Resize en Bun 1.4.5:** además del handler interno de OpenTUI, el prototipo
   escucha `SIGWINCH` y llama a `renderer.resize(cols, rows)`. Es la API
   documentada para resize externo y evita perder el evento durante un frame.
3. **Foco:** usar `terminal.focus()`, no `renderer.focusRenderable()`.
4. **Tamaño inicial:** `renderer.width` × (`renderer.height` − 2), reservando
   barra de pestañas y estado.
5. **Datos:** reenviar al PTY tanto `source === "input"` como `"response"`.

## Fixture SSH

`scripts/ssh-fixture.sh start|stop` crea claves, `authorized_keys` y un
`sshd_config` en `${TMPDIR:-/tmp}/ssh05-sshd-fixture`, escucha en el puerto 2222 y
se detiene al terminar el smoke test. No modifica `~/.ssh` ni requiere permisos
de root.
