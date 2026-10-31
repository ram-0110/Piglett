import { app, shell, BrowserWindow, ipcMain, dialog } from 'electron'
import { join } from 'path'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import icon from '../../resources/icon.png?asset'
import { resolveCoreBinaryPath } from './core/binary.js'
import { coreRequest, startCore, stopCore } from './core/client.js'

function createWindow() {
  // Create the browser window.
  const mainWindow = new BrowserWindow({
    width: 1120,
    height: 760,
    minWidth: 760,
    minHeight: 560,
    show: false,
    autoHideMenuBar: true,
    ...(process.platform === 'linux' ? { icon } : {}),
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      sandbox: false
    }
  })

  mainWindow.on('ready-to-show', () => {
    mainWindow.show()
  })

  mainWindow.webContents.setWindowOpenHandler((details) => {
    shell.openExternal(details.url)
    return { action: 'deny' }
  })

  // HMR for renderer base on electron-vite cli.
  // Load the remote URL for development or the local html file for production.
  if (is.dev && process.env['ELECTRON_RENDERER_URL']) {
    mainWindow.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    mainWindow.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

// This method will be called when Electron has finished
// initialization and is ready to create browser windows.
// Some APIs can only be used after this event occurs.
app.whenReady().then(() => {
  // Set app user model id for windows
  electronApp.setAppUserModelId('com.piglett.app')

  // Default open or close DevTools by F12 in development
  // and ignore CommandOrControl + R in production.
  // see https://github.com/alex8088/electron-toolkit/tree/master/packages/utils
  app.on('browser-window-created', (_, window) => {
    optimizer.watchWindowShortcuts(window)
  })

  ipcMain.handle('core:request', (_, action, body) => coreRequest(action, body))
  ipcMain.handle('core:choose-file', async (_, kind) => {
    const options = {
      blender: { title: 'Choose Blender executable', properties: ['openFile'] },
      project: { title: 'Choose Blender project folder', properties: ['openDirectory'] },
      output: { title: 'Choose output folder', properties: ['openDirectory', 'createDirectory'] }
    }[kind]
    if (!options) throw new Error('Unknown file picker')
    const result = await dialog.showOpenDialog(options)
    if (result.canceled) return null
    const selected = result.filePaths[0]
    return kind === 'blender' && selected.endsWith('.app')
      ? join(selected, 'Contents', 'MacOS', 'Blender')
      : selected
  })
  ipcMain.handle('core:binary-status', () => {
    try {
      resolveCoreBinaryPath()
      return { ready: true }
    } catch (error) {
      return { ready: false, error: error.message }
    }
  })

  createWindow()
  void startCore().catch((error) => console.error('Core startup failed:', error))

  app.on('activate', function () {
    // On macOS it's common to re-create a window in the app when the
    // dock icon is clicked and there are no other windows open.
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

// Quit when all windows are closed, except on macOS. There, it's common
// for applications and their menu bar to stay active until the user quits
// explicitly with Cmd + Q.
app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit()
  }
})
app.on('before-quit', stopCore)

// In this file you can include the rest of your app's specific main process
// code. You can also put them in separate files and require them here.
