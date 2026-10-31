---
name: piglett-local-development
description: Work on Piglett's root npm commands, Go Core compilation, Electron binary discovery, and desktop packaging. Use for setup, local builds, or binary path problems.
---

# Piglett local development

Run commands from the repository root:

- `npm run check`: verify prerequisites and source layout.
- `npm run dev`: compile Core, then start Electron/Vue development.
- `npm run build:core`: compile just the Go executable.
- `npm run build` or `npm run build:app`: compile Core and build Electron code.
- `npm run build:unpack`, `build:mac`, `build:win`, or `build:linux`: build and package through the app workspace.
- `npm run lint` and `npm run test:core`: focused checks.

Install Electron dependencies with `cd app && npm install`; `app/package-lock.json` is the dependency lockfile. The root package is a command index, not a dependency workspace.

`tooling/build-core.mjs` runs `go build` from `Core/` and places the executable at `app/resources/native/<platform>/<arch>/piglett-core[.exe]`. Electron resolves that location relative to its built main process during development; packaged apps use `process.resourcesPath/native`. `app/electron-builder.yml` copies the native tree through `extraResources`. Keep both paths aligned. The binary is generated and ignored by Git.

The Go executable starts a local control server with `--data-dir <Electron UserData path>` and prints one JSON readiness line containing its ephemeral localhost port and token. Electron owns this process through `app/src/main/core/client.js`. Core stores `core/renderer.toml` for the active render choice, `core/blenders.json` for saved Blender installations, the active job ledger, TLS certificate, and session data inside UserData. Do not write those into the source tree.

The host and worker connect over a pinned local HTTPS link and a short-lived pairing code. Use two installed app instances on the same Wi-Fi network for real transfer tests. The host also joins its own pool as a worker. Core's scheduler tests cover lease expiry, stale results, and the version gate.
