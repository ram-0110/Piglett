<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'

const page = ref('loading')
const state = ref(null)
const error = ref('')
const busy = ref(false)
const blenderPath = ref('')
const capabilities = ref(null)
const deviceIndex = ref(0)
const projectPath = ref('')
const blendFiles = ref([])
const blendPath = ref('')
const outputPath = ref('')
const algorithm = ref('next')
const joinLink = ref('')
const joinCode = ref('')
const requiredVersion = ref('')
const poolBlenderPath = ref('')
const poolCapabilities = ref(null)
const poolDeviceIndex = ref(0)
const poolVersionError = ref('')
const pools = ref([])
const discovering = ref(false)
const copied = ref('')
const darkTheme = ref(localStorage.getItem('piglett-theme') === 'dark')
const setupReturnPage = ref('home')
let timer

document.documentElement.dataset.theme = darkTheme.value ? 'dark' : 'light'

const projectName = computed(() => projectPath.value.split(/[\\/]/).pop() || '')
const job = computed(() => state.value?.job)
const workers = computed(() => state.value?.workers || [])
const frames = computed(() => job.value?.frames || [])
const working = computed(() => frames.value.filter((frame) => frame.state === 'working'))
const completed = computed(() => frames.value.filter((frame) => frame.state === 'completed'))
const remaining = computed(() => frames.value.filter((frame) => frame.state !== 'completed').length)
const selectedDevice = computed(() => capabilities.value?.devices?.[deviceIndex.value])
const selectedPoolDevice = computed(() => poolCapabilities.value?.devices?.[poolDeviceIndex.value])
const canHost = computed(
  () => projectPath.value && outputPath.value && blendFiles.value.includes(blendPath.value)
)
const workerState = computed(() => state.value?.worker)
const activeSessionPage = computed(() =>
  state.value?.mode === 'host'
    ? 'host-dashboard'
    : state.value?.mode === 'worker'
      ? 'connect-dashboard'
      : ''
)

function openDeviceSetup() {
  setupReturnPage.value = page.value
  page.value = 'device-setup'
  void selectBlender(state.value?.config?.blenderPath || state.value?.blenders?.[0]?.path)
}

function goBack() {
  if (page.value === 'device-setup') {
    if (state.value?.setupReady) page.value = setupReturnPage.value
    return
  }
  page.value = 'home'
}

function toggleTheme() {
  darkTheme.value = !darkTheme.value
  document.documentElement.dataset.theme = darkTheme.value ? 'dark' : 'light'
  localStorage.setItem('piglett-theme', darkTheme.value ? 'dark' : 'light')
}

function nameFor(workerId) {
  return workers.value.find((worker) => worker.id === workerId)?.name || 'Offline worker'
}

function formatDuration(seconds) {
  if (seconds == null) return 'Calculating'
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.ceil(seconds / 60)}m`
  return `${Math.floor(seconds / 3600)}h ${Math.ceil((seconds % 3600) / 60)}m`
}

async function refresh() {
  try {
    state.value = await window.api.coreRequest('status')
    if (page.value === 'loading') {
      if (!state.value.setupReady) {
        page.value = 'device-setup'
        void selectBlender(state.value?.blenders?.[0]?.path)
      } else if (state.value.mode === 'host') page.value = 'host-dashboard'
      else if (state.value.mode === 'worker') page.value = 'connect-dashboard'
      else page.value = 'home'
    }
  } catch (cause) {
    error.value = cause.message
  }
}

async function run(action, body, nextPage) {
  busy.value = true
  error.value = ''
  try {
    state.value = await window.api.coreRequest(action, body)
    if (nextPage) page.value = nextPage
    return true
  } catch (cause) {
    error.value = cause.message
    return false
  } finally {
    busy.value = false
  }
}

async function selectBlender(path) {
  if (!path) return
  blenderPath.value = path
  capabilities.value = null
  busy.value = true
  error.value = ''
  try {
    capabilities.value = await window.api.coreRequest('probe', { path })
    const saved = state.value?.config
    const index = capabilities.value.devices.findIndex(
      (device) =>
        path === saved?.blenderPath &&
        device.backend === saved.backend &&
        device.id === saved.deviceId
    )
    deviceIndex.value = index < 0 ? 0 : index
  } catch (cause) {
    error.value = cause.message
  } finally {
    busy.value = false
  }
}

async function addBlender() {
  const path = await window.api.chooseFile('blender')
  if (!path) return
  busy.value = true
  error.value = ''
  try {
    const result = await window.api.coreRequest('blenders/add', { path })
    blenderPath.value = result.installation.path
    capabilities.value = result.capabilities
    deviceIndex.value = 0
    state.value = await window.api.coreRequest('status')
  } catch (cause) {
    error.value = cause.message
  } finally {
    busy.value = false
  }
}

async function saveSetup() {
  if (!selectedDevice.value) return
  const device = selectedDevice.value
  await run(
    'setup',
    {
      blenderPath: blenderPath.value,
      backend: device.backend,
      deviceId: device.id
    },
    state.value?.setupReady ? setupReturnPage.value : 'home'
  )
}

async function choosePath(kind) {
  const path = await window.api.chooseFile(kind)
  if (kind === 'project' && path) {
    projectPath.value = path
    blendPath.value = ''
    blendFiles.value = []
    busy.value = true
    error.value = ''
    try {
      const result = await window.api.coreRequest('project-files', { path })
      blendFiles.value = result.files
      if (result.files.length === 1) blendPath.value = result.files[0]
    } catch (cause) {
      error.value = cause.message
    } finally {
      busy.value = false
    }
  }
  if (kind === 'output' && path) outputPath.value = path
}

async function host() {
  await run(
    'host',
    {
      projectPath: projectPath.value,
      blendPath: blendPath.value.trim(),
      outputPath: outputPath.value,
      algorithm: algorithm.value
    },
    'host-dashboard'
  )
}

async function inspectHost() {
  requiredVersion.value = ''
  poolBlenderPath.value = ''
  poolCapabilities.value = null
  poolVersionError.value = ''
  busy.value = true
  error.value = ''
  try {
    const info = await window.api.coreRequest('inspect', { link: joinLink.value.trim() })
    requiredVersion.value = info.version
  } catch (cause) {
    error.value = cause.message
  } finally {
    busy.value = false
  }
}

async function choosePoolBlender() {
  const path = await window.api.chooseFile('blender')
  if (!path) return
  await selectPoolBlender(path, true)
}

async function selectPoolBlender(path, add = false) {
  poolBlenderPath.value = path
  poolCapabilities.value = null
  poolVersionError.value = ''
  busy.value = true
  error.value = ''
  try {
    const result = add
      ? await window.api.coreRequest('blenders/add', { path })
      : { capabilities: await window.api.coreRequest('probe', { path }) }
    const probed = result.capabilities
    if (add) {
      poolBlenderPath.value = result.installation.path
      state.value = await window.api.coreRequest('status')
    }
    if (probed.version !== requiredVersion.value) {
      poolVersionError.value = `This is Blender ${probed.version}. Choose Blender ${requiredVersion.value} for this pool.`
      return
    }
    poolCapabilities.value = probed
    poolDeviceIndex.value = 0
  } catch (cause) {
    poolVersionError.value = cause.message
  } finally {
    busy.value = false
  }
}

async function savePoolSetup() {
  const device = selectedPoolDevice.value
  if (!device || poolCapabilities.value.version !== requiredVersion.value) return
  await run('setup', {
    blenderPath: poolBlenderPath.value,
    backend: device.backend,
    deviceId: device.id
  })
}

function clearHostInspection() {
  requiredVersion.value = ''
  poolCapabilities.value = null
  poolBlenderPath.value = ''
  poolVersionError.value = ''
}

async function discoverPools() {
  discovering.value = true
  error.value = ''
  try {
    const result = await window.api.coreRequest('discover')
    pools.value = result.pools
    if (result.pools.length === 1 && !joinLink.value.trim()) await selectPool(result.pools[0])
  } catch (cause) {
    error.value = cause.message
  } finally {
    discovering.value = false
  }
}

async function selectPool(pool) {
  joinLink.value = pool.link
  await inspectHost()
}

function openConnect() {
  page.value = 'connect-setup'
  void discoverPools()
}

async function join() {
  await run(
    'join',
    { link: joinLink.value.trim(), code: joinCode.value.trim() },
    'connect-dashboard'
  )
}

async function copy(value, label) {
  try {
    await navigator.clipboard.writeText(value)
    copied.value = label
    window.setTimeout(() => {
      copied.value = ''
    }, 1800)
  } catch (cause) {
    error.value = cause.message
  }
}

async function stop() {
  if (await run('stop', {}, 'home')) await refresh()
}

onMounted(() => {
  void refresh()
  timer = window.setInterval(refresh, 2000)
})
onUnmounted(() => window.clearInterval(timer))
</script>

<template>
  <div class="shell">
    <div class="main-frame">
      <header class="toolbar">
        <div class="toolbar-location">
          <button
            v-if="page !== 'home' && page !== 'loading' && state?.setupReady"
            class="toolbar-back"
            type="button"
            aria-label="Back"
            @click="goBack"
          >
            <svg viewBox="0 0 20 20" fill="none" aria-hidden="true">
              <path d="m11.5 4.5-5.5 5.5 5.5 5.5M6 10h10" />
            </svg>
          </button>
          <button
            class="toolbar-home"
            type="button"
            @click="page = state?.setupReady ? 'home' : 'device-setup'"
          >
            Workspace</button
          ><span v-if="page !== 'home'" class="slash">/</span
          ><strong v-if="page !== 'home'">{{
            page === 'device-setup'
              ? 'Settings'
              : page === 'host-dashboard'
                ? 'Host overview'
                : page === 'connect-dashboard'
                  ? 'Worker overview'
                  : page === 'host-setup'
                    ? 'Host a render'
                    : page === 'connect-setup'
                      ? 'Connect to host'
                      : 'New workspace'
          }}</strong>
        </div>
        <div class="toolbar-actions">
          <button
            v-if="state?.setupReady && page !== 'device-setup'"
            class="toolbar-action"
            type="button"
            :disabled="!!activeSessionPage"
            @click="openDeviceSetup"
          >
            Settings
          </button>
          <button
            class="theme-button"
            type="button"
            :aria-label="darkTheme ? 'Switch to light theme' : 'Switch to dark theme'"
            :aria-pressed="darkTheme"
            @click="toggleTheme"
          >
            {{ darkTheme ? '☀' : '☾' }} <span>{{ darkTheme ? 'Light' : 'Dark' }}</span>
          </button>
        </div>
      </header>
      <main class="content" :class="{ wide: page.includes('dashboard') }">
        <p v-if="error" class="error-banner" role="alert">{{ error }}</p>

        <section v-if="page === 'loading'" class="narrow"><h1>Starting Piglett Core…</h1></section>

        <section v-else-if="page === 'device-setup'" class="narrow">
          <header class="page-header">
            <p class="eyebrow">SETTINGS</p>
            <h1>Blender settings</h1>
            <p>
              Add Blender installations and choose the Cycles device this computer will use for
              rendering.
            </p>
          </header>
          <div class="panel form-panel">
            <div class="step-heading">
              <span>1</span>
              <div>
                <h2>Render device</h2>
                <p>Available devices come from the selected Blender installation.</p>
              </div>
            </div>
            <label class="field-label" for="render-device">CYCLES DEVICE</label>
            <select
              id="render-device"
              v-model.number="deviceIndex"
              class="text-input"
              :disabled="!capabilities?.devices?.length"
            >
              <option v-if="!capabilities?.devices?.length" value="">
                Add a Blender installation first
              </option>
              <option
                v-for="(device, index) in capabilities?.devices || []"
                :key="device.backend + device.id"
                :value="index"
              >
                {{ device.backend }} · {{ device.name }}
              </option>
            </select>
            <p v-if="capabilities" class="field-hint">Using Blender {{ capabilities.version }}</p>
            <div class="divider"></div>
            <div class="settings-section-heading">
              <div class="step-heading">
                <span>2</span>
                <div>
                  <h2>Blender versions</h2>
                  <p>
                    Add an installed Blender application. Its version is detected automatically.
                  </p>
                </div>
              </div>
              <button
                class="secondary-button add-blender-button"
                type="button"
                :disabled="busy"
                @click="addBlender"
              >
                <span aria-hidden="true">+</span> Add Blender
              </button>
            </div>
            <div v-if="state?.blenders?.length" class="installation-list">
              <button
                v-for="installation in state.blenders"
                :key="installation.path"
                class="installation-option"
                :class="{ selected: blenderPath === installation.path }"
                type="button"
                :disabled="busy"
                @click="selectBlender(installation.path)"
              >
                <span class="installation-icon">B</span>
                <span class="installation-details"
                  ><strong>Blender {{ installation.version }}</strong
                  ><small :title="installation.path">{{ installation.path }}</small></span
                >
                <span
                  v-if="state?.config?.blenderPath === installation.path"
                  class="installation-active"
                  >Active</span
                >
                <span v-if="blenderPath === installation.path" class="installation-check">✓</span>
              </button>
            </div>
            <p v-else class="field-hint installation-empty">No Blender versions added yet.</p>
          </div>
          <div class="form-actions">
            <span>{{
              state?.setupReady
                ? `Current: Blender ${state.config.version} · ${state.config.backend}`
                : 'Choose a Blender version and render device to continue.'
            }}</span>
            <button
              class="primary-button"
              type="button"
              :disabled="busy || !selectedDevice"
              @click="saveSetup"
            >
              Save settings
            </button>
          </div>
        </section>

        <section v-else-if="page === 'home'" class="home-page narrow">
          <header class="page-header">
            <p class="eyebrow">NEW WORKSPACE</p>
            <h1>Start rendering together</h1>
            <p>
              Host a Blender project or connect this device to a render session on your local
              network.
            </p>
          </header>
          <div class="action-grid">
            <button
              class="action-card"
              type="button"
              :disabled="!!activeSessionPage"
              @click="page = 'host-setup'"
            >
              <span class="action-icon">+</span><strong>Host</strong
              ><small>Prepare a project and invite devices to render.</small></button
            ><button
              class="action-card"
              type="button"
              :disabled="!!activeSessionPage"
              @click="openConnect"
            >
              <svg class="action-icon" viewBox="0 0 20 20" fill="none" aria-hidden="true">
                <path d="M4 10h12m-5-5 5 5-5 5" /></svg
              ><strong>Connect</strong><small>Find a nearby pool or use a host link.</small>
            </button>
          </div>
          <section v-if="activeSessionPage" class="panel guide">
            <p class="eyebrow">ACTIVE SESSION</p>
            <div class="active-session-row">
              <div>
                <strong>{{ state?.mode === 'host' ? job?.name : workerState?.jobName }}</strong>
                <p>
                  {{
                    state?.mode === 'host' ? 'Hosting a render pool' : 'Connected to a render pool'
                  }}
                </p>
              </div>
              <button class="secondary-button" type="button" @click="page = activeSessionPage">
                Open session
              </button>
            </div>
          </section>
          <section v-if="state?.mode === 'host-paused'" class="panel guide">
            <p class="eyebrow">RECOVERED JOB</p>
            <p>{{ job?.name }} is saved. Resume hosting to reconnect workers.</p>
            <button
              class="primary-button"
              type="button"
              :disabled="busy"
              @click="run('resume', {}, 'host-dashboard')"
            >
              Resume hosting
            </button>
          </section>
        </section>

        <section v-else-if="page === 'host-setup'" class="narrow">
          <header class="page-header">
            <p class="eyebrow">HOST A RENDER</p>
            <h1>Prepare your Blender project</h1>
            <p>
              Choose the folder containing your Blender project. Select its main .blend file below.
            </p>
          </header>
          <div class="panel form-panel">
            <div class="step-heading">
              <span>1</span>
              <div>
                <h2>Project folder</h2>
                <p>Include the .blend file and its external assets.</p>
              </div>
            </div>
            <button class="upload-zone" type="button" @click="choosePath('project')">
              <span class="upload-icon">↑</span
              ><strong>{{ projectName || 'Choose a project folder' }}</strong
              ><small>{{ projectPath || 'Select a folder from this computer' }}</small>
            </button>
            <div class="divider"></div>
            <div class="step-heading">
              <span>2</span>
              <div>
                <h2>Main Blender file</h2>
                <p>Choose the scene whose render settings and frame range will be used.</p>
              </div>
            </div>
            <div
              v-if="blendFiles.length"
              class="blend-list"
              role="radiogroup"
              aria-label="Main Blender file"
            >
              <label
                v-for="file in blendFiles"
                :key="file"
                class="blend-option"
                :class="{ selected: blendPath === file }"
              >
                <input v-model="blendPath" type="radio" name="main-blend" :value="file" />
                <span class="blend-glyph">B</span>
                <span class="blend-file"
                  ><strong>{{ file.split('/').pop() }}</strong
                  ><small>{{ file }}</small></span
                >
                <span class="blend-check">{{ blendPath === file ? '✓' : '' }}</span>
              </label>
            </div>
            <p v-else class="field-hint">Choose a project folder to see its .blend files.</p>
            <label class="field-label" for="output-path">OUTPUT FOLDER</label
            ><button
              id="output-path"
              class="text-input path-button"
              type="button"
              @click="choosePath('output')"
            >
              {{ outputPath || 'Choose output folder' }}</button
            ><label class="field-label" for="algorithm">SCHEDULER</label
            ><select id="algorithm" v-model="algorithm" class="text-input">
              <option value="next">Next available</option>
              <option value="even">Even spread</option>
              <option value="adaptive">Adaptive</option>
            </select>
          </div>
          <div class="form-actions">
            <span>Blender {{ state?.config?.version }} required on all workers</span
            ><button
              class="primary-button"
              type="button"
              :disabled="busy || !canHost"
              @click="host"
            >
              Start hosting
            </button>
          </div>
        </section>

        <section v-else-if="page === 'connect-setup'" class="narrow">
          <header class="page-header">
            <p class="eyebrow">CONNECT TO A HOST</p>
            <h1>Join a render pool</h1>
            <p>
              Find a pool on the same Wi-Fi network, or paste a host link. Enter its pairing code to
              join.
            </p>
          </header>
          <div class="panel form-panel">
            <div class="step-heading">
              <span>1</span>
              <div>
                <h2>Nearby pools</h2>
                <p>Select a host found on your local network.</p>
              </div>
            </div>
            <div class="discovery-heading">
              <span>{{ discovering ? 'Searching…' : `${pools.length} pools found` }}</span
              ><button
                class="secondary-button"
                type="button"
                :disabled="discovering"
                @click="discoverPools"
              >
                Refresh
              </button>
            </div>
            <div v-if="pools.length" class="pool-list">
              <button
                v-for="pool in pools"
                :key="pool.link"
                class="pool-option"
                :class="{ selected: joinLink === pool.link }"
                type="button"
                @click="selectPool(pool)"
              >
                <span class="online-dot"></span
                ><span
                  ><strong>{{ pool.name }}</strong
                  ><small
                    >Blender {{ pool.version }} · {{ new URL(pool.link).hostname }}</small
                  ></span
                ><span>{{ joinLink === pool.link ? '✓' : '›' }}</span>
              </button>
            </div>
            <p v-else class="field-hint">
              {{
                discovering
                  ? 'Looking for hosts…'
                  : 'No nearby pools found. You can still enter a link below.'
              }}
            </p>
            <label class="field-label" for="join-link">OR ENTER A HOST LINK</label
            ><input
              id="join-link"
              v-model="joinLink"
              class="text-input"
              placeholder="https://192.168.1.5:..."
              spellcheck="false"
              @input="clearHostInspection"
            /><button
              class="secondary-button check-button"
              type="button"
              :disabled="busy || !joinLink.trim()"
              @click="inspectHost"
            >
              Check required version
            </button>
            <div
              v-if="requiredVersion"
              class="version-panel"
              :class="{ 'needs-version': requiredVersion !== state?.config?.version }"
            >
              <strong>Blender {{ requiredVersion }} required</strong>
              <p v-if="requiredVersion === state?.config?.version">
                This device is ready with Blender {{ state?.config?.version }}.
              </p>
              <template v-else>
                <p>
                  This device currently uses Blender {{ state?.config?.version }}. Choose an
                  installed Blender {{ requiredVersion }} executable for this pool.
                </p>
                <div
                  v-if="state?.blenders?.some((item) => item.version === requiredVersion)"
                  class="matching-installations"
                >
                  <button
                    v-for="installation in state.blenders.filter(
                      (item) => item.version === requiredVersion
                    )"
                    :key="installation.path"
                    class="installation-option"
                    type="button"
                    :disabled="busy"
                    @click="selectPoolBlender(installation.path)"
                  >
                    <span class="installation-icon">B</span>
                    <span class="installation-details"
                      ><strong>Blender {{ installation.version }}</strong
                      ><small>{{ installation.path }}</small></span
                    >
                    <span v-if="poolBlenderPath === installation.path" class="installation-check"
                      >✓</span
                    >
                  </button>
                </div>
                <button
                  class="secondary-button"
                  type="button"
                  :disabled="busy"
                  @click="choosePoolBlender"
                >
                  + Add Blender {{ requiredVersion }}
                </button>
                <p v-if="poolVersionError" class="version-error" role="alert">
                  {{ poolVersionError }}
                </p>
                <div v-if="poolCapabilities" class="pool-device-setup">
                  <label class="field-label" for="pool-render-device">RENDER DEVICE</label>
                  <select
                    id="pool-render-device"
                    v-model.number="poolDeviceIndex"
                    class="text-input"
                  >
                    <option
                      v-for="(device, index) in poolCapabilities.devices"
                      :key="device.backend + device.id"
                      :value="index"
                    >
                      {{ device.backend }} · {{ device.name }}
                    </option>
                  </select>
                  <button
                    class="primary-button"
                    type="button"
                    :disabled="busy || !selectedPoolDevice"
                    @click="savePoolSetup"
                  >
                    Use this Blender version
                  </button>
                </div>
              </template>
            </div>
            <div class="divider"></div>
            <div class="step-heading">
              <span>2</span>
              <div>
                <h2>Pairing code</h2>
                <p>Enter the code shown on the host.</p>
              </div>
            </div>
            <label class="field-label" for="join-code">PAIRING CODE</label
            ><input
              id="join-code"
              v-model="joinCode"
              class="text-input code-input"
              placeholder="123-456"
              autocomplete="off"
              spellcheck="false"
            />
          </div>
          <div class="form-actions">
            <span>Project files transfer after pairing</span
            ><button
              class="primary-button"
              type="button"
              :disabled="
                busy ||
                !joinLink.trim() ||
                !joinCode.trim() ||
                requiredVersion !== state?.config?.version
              "
              @click="join"
            >
              Connect
            </button>
          </div>
        </section>

        <section v-else-if="page === 'host-dashboard'" class="dashboard">
          <header class="dashboard-header">
            <div>
              <p class="eyebrow">HOST WORKSPACE</p>
              <h1>{{ job?.name }}</h1>
              <p>
                {{ job?.blendPath }} · Blender {{ job?.version }} · {{ job?.algorithm }} scheduling
              </p>
            </div>
            <button class="secondary-button" type="button" @click="stop">Stop hosting</button>
          </header>
          <div class="panel share-panel">
            <div>
              <p class="eyebrow">INVITE DEVICES</p>
              <strong>{{ state?.code }}</strong>
              <p>{{ state?.link }}</p>
            </div>
            <div class="share-actions">
              <button class="secondary-button" type="button" @click="copy(state?.link, 'link')">
                {{ copied === 'link' ? 'Copied' : 'Copy link' }}</button
              ><button class="secondary-button" type="button" @click="copy(state?.code, 'code')">
                {{ copied === 'code' ? 'Copied' : 'Copy code' }}
              </button>
            </div>
          </div>
          <div class="stats-grid">
            <div class="panel stat">
              <span>Connected devices</span
              ><strong>{{ workers.filter((w) => w.status !== 'offline').length }}</strong
              ><small>{{ workers.length }} joined total</small>
            </div>
            <div class="panel stat">
              <span>Working</span><strong>{{ working.length }}</strong
              ><small>Frames in progress</small>
            </div>
            <div class="panel stat">
              <span>Completed</span
              ><strong
                >{{ completed.length }} <em>/ {{ frames.length }}</em></strong
              ><small>{{ remaining }} frames remaining</small>
            </div>
            <div class="panel stat">
              <span>Estimated time left</span
              ><strong class="host-name">{{ formatDuration(state?.progress?.etaSeconds) }}</strong
              ><small>{{ state?.progress?.failed || 0 }} failed after retries</small>
            </div>
          </div>
          <section class="panel data-panel">
            <div class="panel-heading">
              <div>
                <h2>Connected devices</h2>
                <p>Live membership and assigned frames</p>
              </div>
              <span class="badge">{{ workers.length }} devices</span>
            </div>
            <div v-for="worker in workers" :key="worker.id" class="device-row">
              <span class="avatar">{{ worker.name?.[0] }}</span>
              <div class="device-name">
                <strong>{{ worker.name }}</strong
                ><small>{{ worker.backend }} · Blender {{ worker.version }}</small>
              </div>
              <div class="assignment">
                {{
                  working
                    .filter((f) => f.workerId === worker.id)
                    .map((f) => f.number)
                    .join(', ') || 'No frame'
                }}<small>{{ worker.status }}</small>
              </div>
              <div class="device-progress">
                <small>{{ worker.completed }} done</small>
              </div>
            </div>
            <p v-if="!workers.length" class="empty-row">Waiting for devices to join…</p>
          </section>
          <div class="tables-grid">
            <section class="panel data-panel">
              <div class="panel-heading">
                <div>
                  <h2>Working</h2>
                  <p>Current frame leases</p>
                </div>
                <span class="badge">{{ working.length }}</span>
              </div>
              <div v-for="frame in working" :key="frame.number" class="frame-row">
                <b>{{ frame.number }}</b
                ><span>{{ nameFor(frame.workerId) }}</span
                ><small>Attempt {{ frame.attempts }}</small>
              </div>
              <p v-if="!working.length" class="empty-row">No frames working</p>
            </section>
            <section class="panel data-panel">
              <div class="panel-heading">
                <div>
                  <h2>Completed</h2>
                  <p>Most recent finished frames</p>
                </div>
                <span class="badge">{{ completed.length }}</span>
              </div>
              <div
                v-for="frame in completed.slice(-8).reverse()"
                :key="frame.number"
                class="frame-row"
              >
                <b>{{ frame.number }}</b
                ><span>{{ nameFor(frame.workerId) }}</span
                ><small>Done</small>
              </div>
              <p v-if="!completed.length" class="empty-row">No frames completed yet</p>
            </section>
          </div>
        </section>

        <section v-else class="dashboard">
          <header class="dashboard-header">
            <div>
              <p class="eyebrow">CONNECTED WORKSPACE</p>
              <h1>{{ workerState?.jobName || 'Worker overview' }}</h1>
              <p>Host {{ workerState?.host }} · Blender {{ state?.config?.version }}</p>
            </div>
            <button class="secondary-button" type="button" @click="stop">Disconnect</button>
          </header>
          <div class="connection-banner">
            <span class="online-dot"></span>
            <div>
              <strong>{{ workerState?.status || 'Connecting' }}</strong>
              <p>
                {{
                  workerState?.error ||
                  'This device requests another frame when its current render finishes.'
                }}
              </p>
            </div>
          </div>
          <div class="stats-grid worker-stats">
            <div class="panel stat">
              <span>Current frames</span><strong>{{ workerState?.frames?.length || 0 }}</strong
              ><small>{{ workerState?.frames?.join(', ') || 'Waiting for work' }}</small>
            </div>
            <div class="panel stat">
              <span>Completed here</span><strong>{{ workerState?.completed || 0 }}</strong
              ><small>This session</small>
            </div>
            <div class="panel stat">
              <span>Render device</span
              ><strong class="host-name">{{ state?.config?.backend }}</strong
              ><small>Frames assigned automatically</small>
            </div>
          </div>
          <div class="tables-grid">
            <section class="panel data-panel">
              <div class="panel-heading">
                <div>
                  <h2>Working</h2>
                  <p>Current assignments</p>
                </div>
              </div>
              <div v-for="frame in workerState?.frames || []" :key="frame" class="frame-row">
                <b>{{ frame }}</b
                ><span>Frame {{ frame }}</span
                ><small>Rendering</small>
              </div>
              <p v-if="!workerState?.frames?.length" class="empty-row">Waiting for a frame</p>
            </section>
            <section class="panel data-panel">
              <div class="panel-heading">
                <div>
                  <h2>Completed</h2>
                  <p>Accepted by the host</p>
                </div>
              </div>
              <p class="empty-row">{{ workerState?.completed || 0 }} frames completed</p>
            </section>
          </div>
        </section>
      </main>
    </div>
  </div>
</template>
