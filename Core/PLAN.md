# Piglett Core implementation plan

Piglett is a dynamic resource pool for Blender rendering. Workers can join, leave, crash, or reconnect while a job is running. The scheduler must respond to the current pool, not assume a fixed set of machines.

## Agreed first version

- The host and workers are on the same Wi-Fi network. They communicate directly; no internet service or account is required.
- Joining uses a local link containing the host address and a short-lived pairing code. The host approves and tracks each worker.
- The host packages the selected project folder into a validated ZIP and transfers it to each paired worker. Workers return finished frames to the host.
- Every device completes Blender setup before it can host or connect. A worker must provide an installed Blender executable matching the job's required full major.minor.patch version before joining the render pool.
- Support Cycles first, on CPU and GPUs that the local Blender installation actually reports as usable. Do not assume CUDA, OptiX, HIP, oneAPI, or Metal is available merely from the operating system or GPU vendor.
- Schedule whole frames. The host owns durable job and frame state, the authoritative queue, and output. The first host session runs one active job; job IDs leave room for a later multi-job queue.

## Process boundaries

```text
Vue UI → Electron preload → Electron main → local Go Core
                                             │
                           host Go Core ↔ worker Go Core
                                             │
                                         Blender CLI
```

Electron starts and stops its own Go process, passes the Electron UserData directory, and exposes a narrow local API to the renderer. Go owns discovery, pairing, transfer, scheduling, Blender subprocesses, and its TOML configuration. The renderer does not open network sockets or run Blender.

## Work stages

### 1. Local Core protocol and lifecycle

Give Core explicit host and worker modes. Add a local control channel between Electron main and Go with versioned requests, responses, and state events. Define startup readiness, shutdown, process ownership, reconnect behavior, and error codes before replacing UI sample data. Persist host job/frame state so a host app restart can recover the queue instead of silently losing progress.

### 2. Mandatory setup and Blender capability profile

Every device starts with setup: choose an installed Blender executable, probe it, choose a usable Cycles CPU/GPU device, and confirm the selection. A device is not ready to host or join until this succeeds. Locate the executable, run a background probe, and record its exact version, Cycles availability, usable compute backends, and usable devices. A probe must exercise Blender's APIs rather than relying on a static enum list; Blender 5.2 was observed to render with Cycles even when a simple engine enum query listed only Eevee.

Persist the user's selected Blender path, Cycles backend, device identifiers, and CPU fallback choice in a TOML file under Electron UserData. Keep detected capabilities separate from user choices and refresh them when Blender, its path, or the hardware changes. Reopen setup if a saved choice is no longer available. The host reads a worker's advertised capabilities before admitting it to the pool or assigning a frame.

Project render settings such as frame range, resolution, output format, samples, and denoising come from the chosen `.blend` file. The worker's TOML controls its local compute device. Avoid changing scene settings independently on workers.

### 3. Version gate, pairing, and live membership

The host selects the Blender executable for the job and advertises its required version in the local join link. Before pairing or transferring files, the worker shows that requirement and can select a matching installed Blender executable and render device directly in the Connect flow. The protocol handshake independently verifies the version and capabilities reported by the worker; an edited link cannot bypass the check. Do not automatically install Blender in the first version.

The host listens on an available local interface and publishes a link containing its address and session identity. Workers can discover hosts through a local UDP broadcast query and select a pool, or enter the link manually. A short-lived pairing code authenticates a joining worker. Track each device as setting up, connecting, transferring, ready, busy, reconnecting, offline, or incompatible. A ready worker can join an active job and request work immediately. When a worker leaves, the pool and ETA update without stopping other workers. Provide clear errors for version mismatch, unreachable host, rejected pairing, and unsupported render device. A router with client isolation enabled cannot support direct device-to-device transfer and must be reported as a connection failure.

Use an authenticated transport for control messages and file transfer. The host identifies each worker and rejects unsolicited commands. A session code must expire and be rate limited.

Use heartbeats with a timeout and a short reconnect grace period. A reconnecting worker keeps its identity and can resume a still-valid frame lease or an interrupted transfer. A new worker enters the pool only after version/capability checks and project transfer complete.

### 4. Project preparation and transfer

The host scans the selected folder, presents its `.blend` files, packages the folder with size limits, and confirms the chosen main `.blend` is inside the packaged project, and asks Blender to inspect the frame range and Cycles settings. Transfer the archive with checksums and a resumable or retryable file protocol. Workers extract into session-scoped temporary storage and verify the checksum before accepting assignments. The host collects completed frames, verifies each upload, and writes to a chosen output folder.

### 5. Fault-tolerant whole-frame schedulers

Model jobs, eligible workers, and frame attempts separately. All scheduling modes draw from the same durable pending-frame queue and give each worker one whole frame at a time for its selected render device. Do not preassign a whole job to devices. A device joining mid-render can take the next eligible frame; one leaving cannot strand a long private queue.

Expose three choices in the Host UI. Each choice changes which eligible frame goes to a ready worker, while sharing the same lease and recovery rules:

| UI choice | Algorithm | Use |
| --- | --- | --- |
| Next available (default) | A worker pulls the next eligible frame after finishing its current frame | Mixed devices and variable frame times |
| Even spread | Keep assignment counts close across currently ready devices | Similar devices, without fixed preassignment |
| Adaptive | Use recent successful frame times to choose future assignments | Long jobs with different device speeds |

Each frame attempt has a lease ID, worker ID, deadline, heartbeat/progress, retry count, and final artifact hash. Extend a lease while its worker is healthy and reporting progress. On a confirmed disconnect, expired lease, render failure, or worker crash, return unfinished work to the queue with a bounded retry policy. A brief Wi-Fi interruption should not immediately duplicate a long render. If a lease is reassigned and the old worker later returns, accept only one verified result; late uploads cannot overwrite it. The worker may finish its current frame while the host is temporarily unavailable and spool the output until reconnect, but receives no new work until the host returns.

Record actual frame completion times per worker and per job. Adaptive estimates must warm up from real measurements, respond to changing speed, and avoid giving many speculative frames to one fast-looking device. Recompute progress and ETA on every membership or frame-state change. Keep CPU/GPU capability as eligibility constraints, not as permanent speed rankings. Running frames are not preempted when a new device joins.

Persist leases, accepted artifacts, and retry state so host restart can reconcile with reconnecting workers. Mid-frame Blender checkpointing is outside the first version: a frame on a failed device may need to restart.

### 6. UI integration and verification

Replace the current sample data with Core events. Add a required per-device setup view before Host or Connect. Show the host's required Blender version before worker pairing, and show live pool membership, transfer state, frame assignments, retries, completed frames, and ETA. The Host UI selects the scheduling algorithm for each job.

Verify on at least two computers on one Wi-Fi network, including a worker joining halfway through, a brief disconnect/reconnect, a permanent disconnect during a frame, host restart, interrupted project/result transfer, a late duplicate result, heterogeneous CPU/GPU speeds, Blender version mismatch before pairing, and a saved render device becoming unavailable.

## Initial exclusions

The first version does not use an internet relay, cloud storage, tile rendering, Eevee rendering, automatic Blender installation, or cross-version rendering. These can be added after the frame pipeline works end to end.

## Current implementation

Core now includes the setup TOML and Blender probe, host and worker process modes, fingerprint-pinned HTTPS pairing, project folder packaging and ZIP transfer, local pool discovery, Cycles frame rendering, result uploads, durable frame leases, and the three scheduler choices. Electron starts Core and the UI reads its live state. A local two-process test on one Mac distributed a 40-frame job across both processes and accepted all outputs.

The current host runs one active job at a time. Frame progress is shown as assigned/working/completed with an ETA after measured completions; Blender's internal percentage is not yet parsed. A host restart restores the job ledger and uses the saved listener port when hosting is resumed, while workers retry their connection. Physical cross-device Wi-Fi testing and interrupted large-file transfer testing remain before release.
