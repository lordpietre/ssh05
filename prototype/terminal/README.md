# ssh05 Phase 0 prototype

Minimal OpenTUI + Bun PTY proof. See `../../docs/phase-0/README.md` for the
full context, evidence and pending validations.

```sh
bun install
bun start                                  # prototipo de una sola terminal
bun run ui                                 # ssh05 UI contra ssh05d
bun test                                   # framing, cliente IPC, TabStore
bun run typecheck
bun run ui:smoke                           # requiere bin/ssh05d
```

Pinned in `bun.lock`: `@opentui/core@0.8.2`, Bun 1.4.5, TypeScript 5.9.3.
