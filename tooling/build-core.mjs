import { spawnSync } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const coreRoot = join(root, 'Core')
const goCache = join(root, 'tooling', '.cache', 'go-build')
const binaryName = process.platform === 'win32' ? 'piglett-core.exe' : 'piglett-core'
const output = join(root, 'app', 'resources', 'native', process.platform, process.arch, binaryName)

mkdirSync(dirname(output), { recursive: true })
mkdirSync(goCache, { recursive: true })
const result = spawnSync('go', ['build', '-o', output, '.'], {
  cwd: coreRoot,
  stdio: 'inherit',
  env: { ...process.env, GOCACHE: goCache }
})
if (result.error) {
  console.error(`Could not run Go: ${result.error.message}`)
  process.exit(1)
}
if (result.status !== 0) process.exit(result.status ?? 1)
console.log(`Core ready: ${output}`)
