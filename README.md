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

On first launch, open Settings, add a Blender application, and choose a detected Cycles CPU or GPU device. Settings saves multiple Blender versions under UserData while one version and render device remain active. To host, select a project folder and its main `.blend` from the detected files, then choose an output folder and scheduler, then share the local link and pairing code. Other devices complete their own Blender setup, discover the pool on the same Wi-Fi or enter its link, and enter the pairing code. If a pool needs a different Blender version, Connect offers any matching saved version or lets the worker add the required installation and choose its render device before pairing.

See [AGENTS.md](AGENTS.md) and `skills/` for the component map and development conventions.
