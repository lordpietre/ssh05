# TRSS Phase 0 prototype

Minimal OpenTUI + Bun PTY proof. See `../../docs/phase-0/README.md` for the
full context, evidence and pending validations.

```sh
bun install
bun start
bun run src/main.ts -- vim /tmp/trss.txt
bun run smoke        # tmux + vim + htop + SSH + resize
bun run typecheck
```

Pinned in `bun.lock`: `@opentui/core@0.8.2`, Bun 1.4.5, TypeScript 5.9.3.
