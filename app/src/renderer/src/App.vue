<script setup>
import { computed, ref } from 'vue'

const page = ref('home')
const zipFile = ref(null)
const blendPath = ref('')
const joinCode = ref('')
const copied = ref(false)
const demoCode = 'PIG-482-731'
const archiveName = computed(() => zipFile.value?.name || '')
const projectName = computed(() => archiveName.value.replace(/\.zip$/i, '') || 'Untitled project')
const canHost = computed(() => zipFile.value && /\.blend$/i.test(blendPath.value.trim()))
const devices = [
  { name: 'This Mac', type: 'Host · Apple Silicon', frame: 124, progress: 68 },
  { name: 'Studio PC', type: 'Worker · Windows', frame: 125, progress: 42 },
  { name: 'Render Node 02', type: 'Worker · Linux', frame: 126, progress: 15 }
]
const working = [
  { frame: 124, device: 'This Mac', progress: '68%', time: '01:42' },
  { frame: 125, device: 'Studio PC', progress: '42%', time: '02:18' },
  { frame: 126, device: 'Render Node 02', progress: '15%', time: '00:37' }
]
const completed = [
  { frame: 121, device: 'Studio PC', time: '04:12' },
  { frame: 122, device: 'This Mac', time: '03:48' },
  { frame: 123, device: 'Render Node 02', time: '04:01' }
]

function selectZip(event) {
  zipFile.value = event.target.files?.[0] || null
  blendPath.value = ''
}

async function copyCode() {
  try {
    await navigator.clipboard.writeText(demoCode)
    copied.value = true
    window.setTimeout(() => {
      copied.value = false
    }, 1800)
  } catch {
    copied.value = false
  }
}
</script>

<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand"><span class="brand-mark">p</span><span>Piglett</span></div>
      <p class="side-label">WORKSPACE</p>
      <button
        class="side-link"
        :class="{ active: page === 'home' }"
        type="button"
        @click="page = 'home'"
      >
        <span class="side-glyph">⌂</span> New workspace
      </button>
      <p class="side-label sessions-label">SESSIONS</p>
      <button
        v-if="page === 'host-dashboard'"
        class="side-link active"
        type="button"
        @click="page = 'host-dashboard'"
      >
        <span class="online-dot"></span> {{ projectName }}
      </button>
      <button
        v-else-if="page === 'connect-dashboard'"
        class="side-link active"
        type="button"
        @click="page = 'connect-dashboard'"
      >
        <span class="online-dot"></span> Connected session
      </button>
      <p v-else class="side-empty">No active sessions</p>
      <div class="side-footer"><span class="online-dot"></span> Local network · Preview</div>
    </aside>

    <div class="main-frame">
      <header class="toolbar">
        <div>
          <button class="toolbar-home" type="button" @click="page = 'home'">Workspace</button
          ><span class="slash">/</span
          ><strong>{{
            page === 'home'
              ? 'New workspace'
              : page === 'host-setup'
                ? 'Host a render'
                : page === 'connect-setup'
                  ? 'Connect to host'
                  : page === 'host-dashboard'
                    ? 'Host overview'
                    : 'Worker overview'
          }}</strong>
        </div>
        <span class="preview-pill">● &nbsp; UI preview</span>
      </header>

      <main class="content" :class="{ wide: page.includes('dashboard') }">
        <section v-if="page === 'home'" class="home-page narrow">
          <header class="page-header">
            <p class="eyebrow">NEW WORKSPACE</p>
            <h1>Start rendering together</h1>
            <p>
              Host a Blender project or connect this device to a render session on your local
              network.
            </p>
          </header>
          <div class="action-grid">
            <button class="action-card" type="button" @click="page = 'host-setup'">
              <span class="action-icon">+</span><strong>Host</strong>
              <small>Prepare a project and invite devices to render.</small>
            </button>
            <button class="action-card" type="button" @click="page = 'connect-setup'">
              <span class="action-icon">→</span><strong>Connect</strong>
              <small>Join a host using its session code or link.</small>
            </button>
          </div>
          <section class="panel guide">
            <p class="eyebrow">HOW IT WORKS</p>
            <div class="guide-row">
              <span>01</span>
              <div>
                <strong>Prepare your project</strong>
                <p>Choose a ZIP archive and identify its main .blend file.</p>
              </div>
            </div>
            <div class="guide-row">
              <span>02</span>
              <div>
                <strong>Connect devices</strong>
                <p>Share a local session code with other computers.</p>
              </div>
            </div>
            <div class="guide-row">
              <span>03</span>
              <div>
                <strong>Watch frames render</strong>
                <p>See connected devices, working frames, and completed frames.</p>
              </div>
            </div>
          </section>
        </section>

        <section v-else-if="page === 'host-setup'" class="narrow">
          <button class="back-link" type="button" @click="page = 'home'">← New workspace</button>
          <header class="page-header">
            <p class="eyebrow">HOST A RENDER</p>
            <h1>Prepare your Blender project</h1>
            <p>
              Add a ZIP archive, then name the main .blend file inside it. Archive extraction and
              file discovery will be connected to Go later.
            </p>
          </header>
          <div class="panel form-panel">
            <div class="step-heading">
              <span>1</span>
              <div>
                <h2>Project archive</h2>
                <p>Choose a .zip file containing your Blender project and assets.</p>
              </div>
            </div>
            <label class="upload-zone" for="zip-input"
              ><span class="upload-icon">↑</span
              ><strong>{{ archiveName || 'Choose a ZIP archive' }}</strong
              ><small>{{
                archiveName
                  ? 'Click to choose a different file'
                  : 'Select a file from this computer'
              }}</small></label
            >
            <input
              id="zip-input"
              class="visually-hidden"
              type="file"
              accept=".zip,application/zip"
              @change="selectZip"
            />
            <div class="divider"></div>
            <div class="step-heading">
              <span>2</span>
              <div>
                <h2>Main Blender file</h2>
                <p>Enter the path to the .blend file inside the archive.</p>
              </div>
            </div>
            <label class="field-label" for="blend-path">FILE PATH IN ARCHIVE</label>
            <input
              id="blend-path"
              v-model="blendPath"
              class="text-input"
              type="text"
              placeholder="scenes/main.blend"
              spellcheck="false"
            />
            <p class="field-hint">
              Later, this field will list the .blend files found after extraction.
            </p>
          </div>
          <div class="form-actions">
            <span>Preview only · no files are extracted</span
            ><button
              class="primary-button"
              type="button"
              :disabled="!canHost"
              @click="page = 'host-dashboard'"
            >
              Preview host workspace →
            </button>
          </div>
        </section>

        <section v-else-if="page === 'connect-setup'" class="narrow">
          <button class="back-link" type="button" @click="page = 'home'">← New workspace</button>
          <header class="page-header">
            <p class="eyebrow">CONNECT TO A HOST</p>
            <h1>Join a render session</h1>
            <p>
              Enter the code or local link shared by the host. Network discovery and connection will
              be added with the Go backend.
            </p>
          </header>
          <div class="panel form-panel">
            <div class="step-heading">
              <span>1</span>
              <div>
                <h2>Host code or link</h2>
                <p>Both formats will work when local networking is ready.</p>
              </div>
            </div>
            <label class="field-label" for="join-code">SESSION CODE OR LINK</label>
            <input
              id="join-code"
              v-model="joinCode"
              class="text-input code-input"
              type="text"
              placeholder="PIG-482-731"
              autocomplete="off"
              spellcheck="false"
            />
            <p class="field-hint">Enter any value to explore the worker workspace preview.</p>
          </div>
          <div class="form-actions">
            <span>Preview only · no connection is made</span
            ><button
              class="primary-button"
              type="button"
              :disabled="!joinCode.trim()"
              @click="page = 'connect-dashboard'"
            >
              Preview connection →
            </button>
          </div>
        </section>

        <section v-else-if="page === 'host-dashboard'" class="dashboard">
          <header class="dashboard-header">
            <div>
              <p class="eyebrow">HOST WORKSPACE <span class="sample-tag">SAMPLE DATA</span></p>
              <h1>{{ projectName }}</h1>
              <p>{{ blendPath }} · {{ archiveName }}</p>
            </div>
            <button class="secondary-button" type="button" @click="page = 'host-setup'">
              Edit project
            </button>
          </header>
          <div class="panel share-panel">
            <div>
              <p class="eyebrow">INVITE DEVICES</p>
              <strong>{{ demoCode }}</strong>
              <p>Preview code. Local connections will be available when the backend is ready.</p>
            </div>
            <button class="secondary-button" type="button" @click="copyCode">
              {{ copied ? 'Copied' : 'Copy code' }}
            </button>
          </div>
          <div class="stats-grid">
            <div class="panel stat">
              <span>Connected devices</span><strong>3</strong><small>1 host · 2 workers</small>
            </div>
            <div class="panel stat">
              <span>Working</span><strong>3</strong><small>Frames 124–126</small>
            </div>
            <div class="panel stat">
              <span>Completed</span><strong>123 <em>/ 240</em></strong
              ><small>117 frames remaining</small>
            </div>
            <div class="panel stat">
              <span>Estimated time left</span><strong>2h 18m</strong
              ><small>Based on sample pace</small>
            </div>
          </div>
          <section class="panel data-panel">
            <div class="panel-heading">
              <div>
                <h2>Connected devices</h2>
                <p>Frame assignment and progress for each machine</p>
              </div>
              <span class="badge">3 devices</span>
            </div>
            <div v-for="device in devices" :key="device.name" class="device-row">
              <span class="avatar">{{ device.name[0] }}</span>
              <div class="device-name">
                <strong>{{ device.name }}</strong
                ><small>{{ device.type }}</small>
              </div>
              <div class="assignment">Frame {{ device.frame }}<small>Rendering</small></div>
              <div class="device-progress">
                <div class="progress-track">
                  <span :style="{ width: device.progress + '%' }"></span>
                </div>
                <small>{{ device.progress }}%</small>
              </div>
            </div>
          </section>
          <div class="tables-grid">
            <section class="panel data-panel">
              <div class="panel-heading">
                <div>
                  <h2>Working</h2>
                  <p>Frames currently assigned</p>
                </div>
                <span class="badge">3</span>
              </div>
              <div v-for="item in working" :key="item.frame" class="frame-row">
                <b>{{ item.frame }}</b
                ><span>{{ item.device }}</span
                ><small>{{ item.progress }} · {{ item.time }}</small>
              </div>
            </section>
            <section class="panel data-panel">
              <div class="panel-heading">
                <div>
                  <h2>Completed</h2>
                  <p>Most recent finished frames</p>
                </div>
                <span class="badge">123</span>
              </div>
              <div v-for="item in completed" :key="item.frame" class="frame-row">
                <b>{{ item.frame }}</b
                ><span>{{ item.device }}</span
                ><small>{{ item.time }}</small>
              </div>
            </section>
          </div>
        </section>

        <section v-else class="dashboard">
          <header class="dashboard-header">
            <div>
              <p class="eyebrow">CONNECTED WORKSPACE <span class="sample-tag">SAMPLE DATA</span></p>
              <h1>Worker overview</h1>
              <p>Connected to Studio host · {{ joinCode }}</p>
            </div>
            <button class="secondary-button" type="button" @click="page = 'connect-setup'">
              Change host
            </button>
          </header>
          <div class="connection-banner">
            <span class="online-dot"></span>
            <div>
              <strong>Ready to receive work</strong>
              <p>Connection activity shown here is a preview. No host is connected yet.</p>
            </div>
          </div>
          <div class="stats-grid worker-stats">
            <div class="panel stat">
              <span>Current frame</span><strong>125</strong><small>Working · 42% complete</small>
            </div>
            <div class="panel stat">
              <span>Completed here</span><strong>18</strong><small>This session</small>
            </div>
            <div class="panel stat">
              <span>Host</span><strong class="host-name">Studio host</strong
              ><small>Local network preview</small>
            </div>
          </div>
          <div class="tables-grid">
            <section class="panel data-panel">
              <div class="panel-heading">
                <div>
                  <h2>Working</h2>
                  <p>Current assignment</p>
                </div>
                <span class="badge">1</span>
              </div>
              <div class="worker-current">
                <b>125</b>
                <div>
                  <strong>Frame 125</strong>
                  <p>Rendering on this device</p>
                  <div class="progress-track"><span style="width: 42%"></span></div>
                </div>
                <strong>42%</strong>
              </div>
            </section>
            <section class="panel data-panel">
              <div class="panel-heading">
                <div>
                  <h2>Completed</h2>
                  <p>Recent work from this device</p>
                </div>
                <span class="badge">18</span>
              </div>
              <div v-for="frame in [122, 117, 113]" :key="frame" class="frame-row">
                <b>{{ frame }}</b
                ><span>Frame {{ frame }}</span
                ><small>Done</small>
              </div>
            </section>
          </div>
        </section>
      </main>
    </div>
  </div>
</template>
