import { spawn } from 'node:child_process'
import { app } from 'electron'
import { resolveCoreBinaryPath } from './binary.js'

const actions = new Set([
  'status',
  'probe',
  'blenders/add',
  'setup',
  'project-files',
  'host',
  'resume',
  'join',
  'inspect',
  'discover',
  'stop'
])
let child = null
let connection = null
let starting = null

export function startCore() {
  if (starting) return starting
  starting = new Promise((resolve, reject) => {
    let binary
    try {
      binary = resolveCoreBinaryPath()
    } catch (error) {
      reject(error)
      return
    }
    child = spawn(binary, ['--data-dir', app.getPath('userData')], {
      stdio: ['ignore', 'pipe', 'pipe']
    })
    let buffer = ''
    let settled = false
    child.stdout.setEncoding('utf8')
    child.stdout.on('data', (chunk) => {
      if (settled) return
      buffer += chunk
      const newline = buffer.indexOf('\n')
      if (newline < 0) return
      try {
        const ready = JSON.parse(buffer.slice(0, newline))
        if (!Number.isInteger(ready.port) || typeof ready.token !== 'string') {
          throw new Error('Core returned invalid startup data')
        }
        connection = ready
        settled = true
        resolve(ready)
      } catch (error) {
        settled = true
        reject(error)
      }
    })
    child.stderr.on('data', (chunk) => console.error('[Core]', String(chunk).trim()))
    child.on('error', (error) => {
      if (!settled) reject(error)
      settled = true
    })
    child.on('exit', (code) => {
      connection = null
      child = null
      starting = null
      if (!settled) reject(new Error(`Core exited during startup (${code})`))
      settled = true
    })
  })
  return starting
}

export async function coreRequest(action, body = {}) {
  if (!actions.has(action)) throw new Error('Unknown Core action')
  const ready = connection || (await startCore())
  const method = action === 'status' || action === 'discover' ? 'GET' : 'POST'
  const response = await fetch(`http://127.0.0.1:${ready.port}/${action}`, {
    method,
    headers: {
      'X-Piglett-Token': ready.token,
      ...(method === 'POST' ? { 'Content-Type': 'application/json' } : {})
    },
    ...(method === 'POST' ? { body: JSON.stringify(body) } : {})
  })
  const result = await response.json()
  if (!response.ok) throw new Error(result.error || `Core request failed (${response.status})`)
  return result
}

export function stopCore() {
  child?.kill()
  child = null
  connection = null
  starting = null
}
