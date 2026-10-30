---
name: piglett-project-map
description: Navigate Piglett's Electron GUI and Go Core workspaces, including their ownership and current integration boundary. Use before cross-component Piglett work.
---

# Piglett project map

Piglett is a distributed Blender rendering farm. The current repo is an early scaffold; a render protocol and worker lifecycle have not been implemented yet.

## Ownership

- `app/src/renderer/src/App.vue`: Vue workspace UI with Host and Connect preview flows. Do not put filesystem, process, or network authority in renderer code.
- `app/src/renderer/src/assets/main.css`: Ausk-inspired workspace palette, Inter typography, and compact sizing.
- `app/src/preload/index.js`: narrow API exposed to the renderer.
- `app/src/main/index.js`: Electron lifecycle and IPC registration.
- `app/src/main/core/binary.js`: locates the Go executable in development and packaged apps.
- `Core/`: Go executable and future render, worker, and connection logic.
- `tooling/`: root checks and Go build commands.

The current integration is a `core:binary-status` IPC check through preload. It proves the binary path is wired but does not start a worker or connect to a daemon. Define those contracts when rendering features are added.

The Host and Connect screens use sample session data. A selected ZIP is not extracted, the typed .blend path is not checked against archive contents, and the sample code does not connect to a host.

## Commands

Root `package.json` is the command index. See [local development](../piglett-local-development/SKILL.md) for compilation and packaging details. Search the owning subtree first; avoid generated `app/out`, `app/dist`, `app/node_modules`, and `app/resources/native` output.
