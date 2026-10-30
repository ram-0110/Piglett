# Piglett workspace

Piglett is a distributed Blender rendering farm. `app/` owns the Electron/Vue GUI and privileged desktop coordination. `Core/` owns Go binaries and, as the project grows, worker and network behavior. Renderer code must cross the preload and main process boundary to request privileged work.

Read the relevant local skill before changing a component:

- [Project map](skills/piglett-project-map/SKILL.md) for ownership and entry points.
- [Local development](skills/piglett-local-development/SKILL.md) for root commands, Go compilation, and packaging.

These skills describe the current scaffold. Update them when entry points, binary layout, or commands change.
