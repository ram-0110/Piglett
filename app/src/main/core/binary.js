import { app } from 'electron'
import { existsSync } from 'node:fs'
import { join, resolve } from 'node:path'

export function resolveCoreBinaryPath() {
  const binaryName = process.platform === 'win32' ? 'piglett-core.exe' : 'piglett-core'
  const nativeRoot = app.isPackaged
    ? join(process.resourcesPath, 'native')
    : resolve(__dirname, '../../resources/native')
  const binaryPath = join(nativeRoot, process.platform, process.arch, binaryName)
  if (!existsSync(binaryPath)) {
    throw new Error(
      `Piglett Core binary missing: ${binaryPath}. Run npm run build:core from the repository root.`
    )
  }
  return binaryPath
}
