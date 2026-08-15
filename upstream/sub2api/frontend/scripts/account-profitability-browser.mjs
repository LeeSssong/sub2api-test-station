import { spawn } from 'node:child_process'
import { existsSync } from 'node:fs'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { createServer } from 'node:net'
import { delimiter, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { pathToFileURL } from 'node:url'
import { resolve as resolvePath } from 'node:path'
import { tmpdir } from 'node:os'
import { setTimeout as delay } from 'node:timers/promises'

export async function findAvailablePort(host = '127.0.0.1') {
  const listener = createServer()
  await new Promise((resolve, reject) => { listener.once('error', reject); listener.listen({ host, port: 0 }, resolve) })
  const address = listener.address()
  await new Promise(resolve => listener.close(resolve))
  if (!address || typeof address === 'string') throw new Error('Unable to allocate loopback port')
  return address.port
}

export function isolatedEnv(env = process.env) {
  const copy = { ...env }
  for (const key of Object.keys(copy)) if (/^VITE_/i.test(key)) delete copy[key]
  copy.VITE_BROWSER_TEST = '1'
  return copy
}

export function resolveChromeExecutable(env = process.env, platform = process.platform) {
  const explicit = env.BROWSER_EXECUTABLE_PATH || env.CHROME_BIN || env.CHROME_PATH || env.BROWSER_PATH
  const commandNames = platform === 'darwin' ? ['Google Chrome'] : ['google-chrome', 'google-chrome-stable', 'chromium', 'chromium-browser']
  const candidates = explicit ? [explicit] : platform === 'darwin'
    ? ['/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', join(env.HOME || '', 'Applications/Google Chrome.app/Contents/MacOS/Google Chrome')]
    : ['/usr/bin/google-chrome', '/usr/bin/google-chrome-stable', '/usr/bin/chromium', '/usr/bin/chromium-browser']
  const pathCandidates = explicit ? candidates : [...candidates, ...(env.PATH || '').split(delimiter).flatMap(dir => commandNames.map(name => join(dir, name)))]
  const found = pathCandidates.find(candidate => candidate && existsSync(candidate))
  if (!found) throw new Error(`Chrome executable not found. Set BROWSER_EXECUTABLE_PATH (or CHROME_BIN/CHROME_PATH). Checked: ${pathCandidates.filter(Boolean).join(', ')}`)
  return found
}

async function waitFor(check, timeoutMs, label, intervalMs = 50) {
  const deadline = Date.now() + timeoutMs
  let lastError
  while (Date.now() < deadline) {
    try { const value = await check(); if (value) return value } catch (error) { lastError = error }
    await delay(intervalMs)
  }
  throw new Error(`${label} timed out${lastError ? `: ${lastError.message}` : ''}`)
}

function killTree(child, signal) {
  if (!child?.pid) return
  try { process.kill(-child.pid, signal) } catch { try { child.kill(signal) } catch {} }
}

async function stopProcess(child, timeoutMs = 3000) {
  if (!child || child.exitCode !== null) return
  killTree(child, 'SIGTERM')
  await Promise.race([new Promise(resolve => child.once('close', resolve)), delay(timeoutMs)])
  if (child.exitCode === null) killTree(child, 'SIGKILL')
}

async function cdpSession(target, child) {
  const socket = new WebSocket(target.webSocketDebuggerUrl)
  await waitFor(() => socket.readyState === WebSocket.OPEN ? true : null, 5000, 'CDP websocket')
  let id = 0
  const command = (method, params = {}) => new Promise((resolve, reject) => {
    const commandId = ++id
    const timer = setTimeout(() => { socket.removeEventListener('message', onMessage); reject(new Error(`CDP command timed out: ${method}`)) }, 5000)
    const onMessage = event => {
      const message = JSON.parse(event.data)
      if (message.id !== commandId) return
      clearTimeout(timer)
      socket.removeEventListener('message', onMessage)
      message.error ? reject(new Error(JSON.stringify(message.error))) : resolve(message.result)
    }
    socket.addEventListener('message', onMessage)
    socket.send(JSON.stringify({ id: commandId, method, params }))
  })
  await command('Network.enable')
  await command('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 1, mobile: false })
  await command('Runtime.evaluate', { expression: `location.href = ${JSON.stringify(target.url)}` })
  await waitFor(async () => (await command('Runtime.evaluate', { expression: 'document.querySelector("#browser-result")?.textContent', returnByValue: true })).result.value, 10000, 'browser result')
  const evaluation = await command('Runtime.evaluate', { expression: 'document.documentElement.outerHTML', returnByValue: true })
  socket.close()
  killTree(child, 'SIGTERM')
  return evaluation.result.value
}

export async function runBrowserTest() {
  const root = fileURLToPath(new URL('..', import.meta.url))
  const port = await findAvailablePort()
  const testUrl = `http://127.0.0.1:${port}/browser/account-profitability.html`
  const profile = await mkdtemp(join(tmpdir(), 'sub2api-account-profitability-'))
  const server = spawn('pnpm', ['vite', '--mode', 'browser-test', '--host', '127.0.0.1', '--port', String(port), '--strictPort'], { cwd: root, env: isolatedEnv(), detached: true, stdio: ['ignore', 'pipe', 'pipe'] })
  let serverOutput = ''
  server.stdout.on('data', chunk => { serverOutput += chunk })
  server.stderr.on('data', chunk => { serverOutput += chunk })
  let chromeChild
  let cleaning = false
  const cleanup = async () => {
    if (cleaning) return
    cleaning = true
    await stopProcess(chromeChild)
    await stopProcess(server)
    await rm(profile, { recursive: true, force: true })
  }
  const onSignal = signal => { process.exitCode = signal === 'SIGINT' ? 130 : 143; void cleanup().finally(() => process.exit(process.exitCode)) }
  process.once('SIGINT', onSignal)
  process.once('SIGTERM', onSignal)
  try {
    await waitFor(async () => { const response = await fetch(testUrl); return response.ok && (await response.text()).includes('account-profitability.ts') }, 10000, 'Vite readiness')
    const chrome = resolveChromeExecutable()
    const args = ['--headless=new', '--no-sandbox', '--disable-gpu', '--hide-scrollbars', '--no-first-run', '--no-default-browser-check', `--user-data-dir=${profile}`, '--remote-debugging-port=0', '--remote-allow-origins=*', testUrl]
    const childOutput = await new Promise((resolve, reject) => {
      chromeChild = spawn(chrome, args, { detached: true, stdio: ['ignore', 'pipe', 'pipe'] })
      let stdout = ''; let stderr = ''
      chromeChild.stdout.on('data', chunk => { stdout += chunk })
      chromeChild.stderr.on('data', chunk => { stderr += chunk })
      chromeChild.once('error', reject)
      const timer = setTimeout(() => reject(new Error('Chrome startup timed out')), 15000)
      ;(async () => {
        const activePort = Number((await waitFor(async () => { try { return (await readFile(join(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0] } catch { return null } }, 10000, 'Chrome DevToolsActivePort')))
        const target = await waitFor(async () => (await (await fetch(`http://127.0.0.1:${activePort}/json/list`)).json()).find(item => item.type === 'page' && item.url === testUrl), 10000, 'Chrome target')
        stdout = await cdpSession(target, chromeChild)
        resolve({ stdout, stderr })
      })().catch(reject).finally(() => clearTimeout(timer))
    })
    const match = childOutput.stdout.match(/<pre id="browser-result">([^<]+)<\/pre>/)
    if (!match) throw new Error(`browser result missing\nstdout=${childOutput.stdout.slice(-2000)}\nstderr=${childOutput.stderr}\nserver=${serverOutput}`)
    return JSON.parse(match[1])
  } finally {
    process.removeListener('SIGINT', onSignal)
    process.removeListener('SIGTERM', onSignal)
    await cleanup()
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolvePath(process.argv[1])).href) {
  runBrowserTest().then(report => { console.log(JSON.stringify(report, null, 2)); if (!report.pass) process.exitCode = 1 }).catch(error => { console.error(error.stack || error); process.exitCode = 1 })
}
