---
name: piglett-project-map
description: Navigate Piglett's Electron GUI and Go Core workspaces, including their ownership and current integration boundary. Use before cross-component Piglett work.
---

# Piglett project map

Piglett is a distributed Blender rendering farm. The repo has a working Core protocol and worker lifecycle.

## Ownership

- `app/src/renderer/src/App.vue`: Settings with saved Blender versions, Host, Connect, and live pool dashboards. Do not put filesystem, process, or network authority in renderer code.
- `app/src/renderer/src/assets/main.css`: Ausk-inspired workspace palette, Inter typography, and compact sizing.
- `app/src/preload/index.js`: narrow API exposed to the renderer.
- `app/src/main/index.js`: Electron lifecycle, file pickers, and IPC registration.
- `app/src/main/core/binary.js`: locates the Go executable in development and packaged apps.
- `app/src/main/core/client.js`: owns the Core process and proxies its localhost control API to the renderer.
- `Core/internal/farm/`: TOML setup, a UserData Blender installation registry, Blender probing, project folder packaging, UDP pool discovery, pinned TLS pairing, worker transfer/rendering, and the durable frame scheduler. Read [the Core plan](../../Core/PLAN.md) before changing the resource pool.
- `tooling/`: root checks and Go build commands.

Electron starts Core with its UserData directory. Renderer requests pass through preload and main to Core's token-protected localhost API. The Go host owns the job ledger and network listener; workers discover nearby pools or enter a fingerprinted HTTPS link, pair with a code, fetch the packaged project ZIP, render frames, and upload results. Keep network and Blender logic in Go.

## Commands

Root `package.json` is the command index. See [local development](../piglett-local-development/SKILL.md) for compilation and packaging details. Search the owning subtree first; avoid generated `app/out`, `app/dist`, `app/node_modules`, and `app/resources/native` output.
