# Piglett

Piglett is an early scaffold for a distributed Blender rendering farm. Electron/Vue provides the GUI; Go Core will own native binaries and connections.

## Setup

Requires Node.js, npm, and Go 1.26 or newer.

```bash
cd app && npm install
cd ..
npm run check
npm run dev
```

`npm run dev` compiles Core and starts Electron. `npm run build` builds the native binary and Electron code. `npm run build:mac`, `build:win`, and `build:linux` create platform packages. Builds target the current operating system and architecture.

See [AGENTS.md](AGENTS.md) and `skills/` for the component map and development conventions.
