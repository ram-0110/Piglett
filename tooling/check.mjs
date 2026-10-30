import { spawnSync } from 'node:child_process'
import { existsSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const required = ['Core/go.mod', 'Core/main.go', 'app/package.json', 'app/electron-builder.yml']
for (const file of required) {
  if (!existsSync(join(root, file))) {
    console.error(`Missing ${file}`)
    process.exitCode = 1
  }
}
for (const [command, args] of [['go', ['version']], ['npm', ['--version']]]) {
  const result = spawnSync(command, args, { encoding: 'utf8' })
  if (result.error || result.status !== 0) {
    console.error(`${command} is unavailable`)
    process.exitCode = 1
  } else {
    console.log(`${command}: ${result.stdout.trim()}`)
  }
}
if (!existsSync(join(root, 'app', 'node_modules'))) {
  console.error('Electron dependencies missing; run npm install in app/')
  process.exitCode = 1
}
