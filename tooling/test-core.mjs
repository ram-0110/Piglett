import { spawnSync } from 'node:child_process'
import { mkdirSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const coreRoot = join(root, 'Core')
const goCache = join(root, 'tooling', '.cache', 'go-build')
mkdirSync(goCache, { recursive: true })
const result = spawnSync('go', ['test', './...'], {
  cwd: coreRoot,
  stdio: 'inherit',
  env: { ...process.env, GOCACHE: goCache }
})
if (result.error) {
  console.error(`Could not run Go: ${result.error.message}`)
  process.exit(1)
}
process.exit(result.status ?? 1)
