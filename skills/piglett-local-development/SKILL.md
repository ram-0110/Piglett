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

The Go executable currently supports `--version` only. Do not assume a network protocol or daemon mode exists yet. When adding one, keep the implementation in Go and expose only needed operations through Electron main and preload.
