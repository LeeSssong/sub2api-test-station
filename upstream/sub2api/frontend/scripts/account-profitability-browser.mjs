import { execFile, spawn } from 'node:child_process'
import { randomBytes, randomInt } from 'node:crypto'
import { existsSync } from 'node:fs'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { createConnection, createServer } from 'node:net'
import { delimiter, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { pathToFileURL } from 'node:url'
import { resolve as resolvePath } from 'node:path'
import { tmpdir } from 'node:os'
import { setTimeout as delay } from 'node:timers/promises'
import { promisify } from 'node:util'

const execFileAsync = promisify(execFile)

export async function findAvailablePort(host = '127.0.0.1') {
  const listener = createServer()
  await new Promise((resolve, reject) => { listener.once('error', reject); listener.listen({ host, port: 0 }, resolve) })
  const address = listener.address()
  await new Promise(resolve => listener.close(resolve))
  if (!address || typeof address === 'string') throw new Error('Unable to allocate loopback port')
  return address.port
}

export function createBrowserTestIdentity() {
  return { nonce: randomBytes(24).toString('hex') }
}

export function buildBrowserTestUrl(portOrOrigin, identity) {
  const origin = typeof portOrOrigin === 'number' ? `http://127.0.0.1:${portOrOrigin}` : String(portOrOrigin).replace(/\/$/, '')
  return `${origin}/browser/account-profitability.html?nonce=${encodeURIComponent(identity.nonce)}`
}

export function isTrustedReadiness(html, identity) {
  const escaped = identity.nonce.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  return new RegExp(`<meta\\s+name=["']browser-test-nonce["']\\s+content=["']${escaped}["']`, 'i').test(html)
}

export function parseBrowserResult(text, identity) {
  const result = JSON.parse(text)
  if (!result || result.nonce !== identity.nonce) throw new Error('Browser result nonce mismatch')
  return result
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

export async function parseProcessTableOutput(stdout, platform = process.platform, readLinuxStart = pid => readFile(`/proc/${pid}/stat`, 'utf8')) {
  const rows = stdout.split('\n').flatMap(line => {
    const match = line.match(/^\s*(\d+)\s+(\d+)\s+(\d+)\s+(\S+)\s+(\S+\s+\S+\s+\d+\s+\S+\s+\S+)\s+(.*)$/)
    if (!match) return []
    const parsedStart = Date.parse(match[5])
    if (Number.isNaN(parsedStart)) return []
    return [{ pid: Number(match[1]), ppid: Number(match[2]), pgid: Number(match[3]), state: match[4], startTime: parsedStart, command: match[6] }]
  })
  if (platform !== 'linux') return rows
  return await Promise.all(rows.map(async row => {
    try {
      const stat = await readLinuxStart(row.pid)
      const endOfCommand = stat.lastIndexOf(')')
      if (endOfCommand < 0) return null
      const fields = stat.slice(endOfCommand + 2).trim().split(/\s+/)
      const linuxStartTime = fields[19]
      if (/^\d+$/.test(linuxStartTime || '')) return { ...row, startTime: `linux:${linuxStartTime}` }
    } catch (error) {
      void error
    }
    return null
  })).then(parsed => parsed.filter(Boolean))
}

export async function processTable() {
  const { stdout } = await execFileAsync('ps', ['-axo', 'pid=,ppid=,pgid=,stat=,lstart=,command='], { maxBuffer: 10 * 1024 * 1024, env: { ...process.env, LC_ALL: 'C' } })
  return parseProcessTableOutput(stdout)
}

function sameIdentity(identity, row) {
  return Boolean(identity?.startTime && identity.command && row?.startTime && row.command) && row.startTime === identity.startTime && row.command === identity.command
}

export class OwnedProcessTree {
  constructor(child, rootMarkers, memberMarkers = rootMarkers, readTable = processTable) {
    this.child = child
    this.rootPid = child.pid
    this.pgid = child.pid
    this.rootMarkers = rootMarkers
    this.memberMarkers = memberMarkers
    this.readTable = readTable
    this.identities = new Map()
    this.pids = new Set(child.pid ? [child.pid] : [])
    this.livePids = new Set(child.pid ? [child.pid] : [])
    this.ownsGroup = false
    this.closed = false
    this.closePromise = new Promise(resolve => {
      child.once('close', () => { this.closed = true; resolve() })
      child.once('error', () => { this.closed = true; resolve() })
    })
  }

  async refresh() {
    const rows = await this.readTable()
    const byPid = new Map(rows.map(row => [row.pid, row]))
    const root = byPid.get(this.rootPid)
    if (root && this.rootMarkers.every(marker => root.command.includes(marker))) {
      if (!this.identities.has(root.pid)) this.identities.set(root.pid, { pid: root.pid, startTime: root.startTime, command: root.command, profile: this.memberMarkers[0] || '', nonce: this.rootMarkers[0] || '', rootPid: this.rootPid })
      this.ownsGroup = root.pgid === this.pgid && sameIdentity(this.identities.get(root.pid), root)
    } else {
      this.ownsGroup = false
    }

    const current = new Set(this.identities.keys())
    const rootIsTrusted = sameIdentity(this.identities.get(this.rootPid), root)
    const verifiedAncestors = rootIsTrusted
      ? new Set([...current].filter(pid => sameIdentity(this.identities.get(pid), byPid.get(pid))))
      : new Set()
    let changed = true
    while (changed) {
      changed = false
      for (const row of rows) {
        if (current.has(row.pid)) continue
        if (verifiedAncestors.has(row.ppid)) {
          current.add(row.pid)
          this.identities.set(row.pid, { pid: row.pid, startTime: row.startTime, command: row.command, profile: this.memberMarkers[0] || '', nonce: this.rootMarkers[0] || '', rootPid: this.rootPid })
          verifiedAncestors.add(row.pid)
          changed = true
        }
      }
    }
    for (const pid of current) this.pids.add(pid)
    this.livePids = new Set([...current].filter(pid => sameIdentity(this.identities.get(pid), byPid.get(pid))))
    return this.livePids
  }

  async signal(signal) {
    await this.refresh()
    if (this.livePids.size === 0) return
    for (const pid of this.livePids) {
      const current = (await this.readTable()).find(row => row.pid === pid)
      if (!sameIdentity(this.identities.get(pid), current)) {
        this.livePids.delete(pid)
        continue
      }
      try { process.kill(pid, signal) } catch (error) { if (error?.code !== 'ESRCH') throw error }
    }
  }

  async waitGone(timeoutMs, label) {
    await waitFor(async () => (await this.refresh()).size === 0, timeoutMs, label)
  }

  async waitReaped(timeoutMs, label) {
    if (this.closed) return
    await waitFor(() => this.closed, timeoutMs, label)
    await this.closePromise
  }

  evidence() {
    return { rootPid: this.rootPid, pgid: this.pgid, pids: [...this.pids].sort((a, b) => a - b) }
  }
}

async function stopOwnedProcess(tree, label, timeoutMs = 3000) {
  if (!tree) return
  await tree.refresh()
  if (tree.livePids.size > 0) {
    await tree.signal('SIGTERM')
    try {
      await tree.waitGone(timeoutMs, `${label} TERM cleanup`)
    } catch {
      await tree.signal('SIGKILL')
      await tree.waitGone(timeoutMs, `${label} KILL cleanup`)
    }
  }
  await tree.waitReaped(timeoutMs, `${label} child reap`)
}

async function isPortClosed(port) {
  if (!port) return true
  return await new Promise(resolve => {
    const socket = createConnection({ host: '127.0.0.1', port })
    socket.setTimeout(250)
    socket.once('connect', () => { socket.destroy(); resolve(false) })
    socket.once('timeout', () => { socket.destroy(); resolve(false) })
    socket.once('error', () => resolve(true))
  })
}

async function waitForPortClosed(port, label) {
  if (port) await waitFor(() => isPortClosed(port), 3000, `${label} port cleanup`)
}

async function cdpSession(target, chromeChild, viteChild) {
  if (chromeChild.exitCode !== null || viteChild.exitCode !== null) throw new Error('Vite/Chrome process exited before CDP connection')
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
  await waitFor(async () => {
    if (chromeChild.exitCode !== null || viteChild.exitCode !== null) throw new Error('Vite/Chrome process exited while waiting for browser result')
    return (await command('Runtime.evaluate', { expression: 'document.querySelector("#browser-result")?.textContent', returnByValue: true })).result.value
  }, 10000, 'browser result')
  const evaluation = await command('Runtime.evaluate', { expression: 'document.documentElement.outerHTML', returnByValue: true })
  return {
    html: evaluation.result.value,
    closeBrowser() {
      if (socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ id: ++id, method: 'Browser.close', params: {} }))
    },
    closeSocket() {
      if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING) socket.close()
    },
  }
}

export async function runBrowserTest(options = {}) {
  const root = fileURLToPath(new URL('..', import.meta.url))
  const identity = createBrowserTestIdentity()
  // Vite 5's CLI treats port 0 as its default port. Pick one ephemeral-range
  // candidate without probing/retrying; strictPort makes any collision fail closed.
  const port = randomInt(40000, 60000)
  const profile = await mkdtemp(join(tmpdir(), 'sub2api-account-profitability-'))
  const serverEnv = isolatedEnv()
  serverEnv.VITE_BROWSER_TEST_NONCE = identity.nonce
  serverEnv.VITE_DEV_PORT = String(port)
  const server = spawn('pnpm', ['vite', '--mode', 'browser-test', '--host', '127.0.0.1', '--strictPort'], { cwd: root, env: serverEnv, detached: true, stdio: ['ignore', 'pipe', 'pipe'] })
  const serverTree = new OwnedProcessTree(server, ['vite --mode browser-test'])
  let serverOutput = ''
  server.stdout.on('data', chunk => { serverOutput += chunk })
  server.stderr.on('data', chunk => { serverOutput += chunk })
  let chromeChild
  let chromeTree
  let chromePort
  let vitePort = port
  let session
  let cleanupPromise
  const resourceEvidence = () => ({
    nonce: identity.nonce,
    chrome: { ...chromeTree?.evidence(), port: chromePort, profile },
    vite: { ...serverTree.evidence(), port: vitePort },
  })
  const cleanup = () => cleanupPromise ??= (async () => {
    if (session && chromeTree && (await chromeTree.refresh()).size > 0) {
      try { session.closeBrowser() } catch (error) { void error }
      try { await chromeTree.waitGone(3000, 'Chrome graceful cleanup') } catch (error) { void error }
    }
    await stopOwnedProcess(chromeTree, 'Chrome')
    session?.closeSocket()
    await waitForPortClosed(chromePort, 'CDP')
    await stopOwnedProcess(serverTree, 'Vite')
    await waitForPortClosed(vitePort, 'Vite')
    await rm(profile, { recursive: true, force: true })
    await waitFor(() => !existsSync(profile), 3000, 'Chrome profile cleanup')
    options.onCleanup?.(resourceEvidence())
  })()
  const onSignal = signal => { process.exitCode = signal === 'SIGINT' ? 130 : 143; void cleanup().finally(() => process.exit(process.exitCode)) }
  process.once('SIGINT', onSignal)
  process.once('SIGTERM', onSignal)
  try {
    const origin = await waitFor(() => {
      if (server.exitCode !== null) throw new Error(`Vite exited with code ${server.exitCode}: ${serverOutput.slice(-1000)}`)
      const match = serverOutput.match(/Local:\s+(https?:\/\/127\.0\.0\.1:\d+)\//)
      return match?.[1]
    }, 10000, 'Vite URL')
    vitePort = Number(new URL(origin).port)
    const testUrl = buildBrowserTestUrl(origin, identity)
    await waitFor(async () => {
      if (server.exitCode !== null) throw new Error(`Vite exited while waiting for readiness: ${serverOutput.slice(-1000)}`)
      const response = await fetch(testUrl)
      if (!response.ok) return null
      return isTrustedReadiness(await response.text(), identity) ? true : null
    }, 10000, 'Vite readiness')
    const chrome = resolveChromeExecutable()
    const args = ['--headless=new', '--no-sandbox', '--disable-gpu', '--hide-scrollbars', '--no-first-run', '--no-default-browser-check', `--user-data-dir=${profile}`, '--remote-debugging-port=0', '--remote-allow-origins=*', testUrl]
    const childOutput = await new Promise((resolve, reject) => {
      chromeChild = spawn(chrome, args, { detached: true, stdio: ['ignore', 'pipe', 'pipe'] })
      chromeTree = new OwnedProcessTree(chromeChild, [`--user-data-dir=${profile}`, identity.nonce], [`--user-data-dir=${profile}`])
      let stdout = ''; let stderr = ''
      chromeChild.stdout.on('data', chunk => { stdout += chunk })
      chromeChild.stderr.on('data', chunk => { stderr += chunk })
      chromeChild.once('error', reject)
      const timer = setTimeout(() => reject(new Error('Chrome startup timed out')), 15000)
      ;(async () => {
        await chromeTree.refresh()
        const activePort = Number((await waitFor(async () => { try { return (await readFile(join(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0] } catch { return null } }, 10000, 'Chrome DevToolsActivePort')))
        chromePort = activePort
        const target = await waitFor(async () => {
          if (chromeChild.exitCode !== null) throw new Error('Chrome exited before target discovery')
          return (await (await fetch(`http://127.0.0.1:${activePort}/json/list`)).json()).find(item => item.type === 'page' && item.url === testUrl)
        }, 10000, 'Chrome target')
        session = await cdpSession(target, chromeChild, server)
        await chromeTree.refresh()
        await serverTree.refresh()
        options.onResources?.(resourceEvidence())
        stdout = session.html
        resolve({ stdout, stderr })
      })().catch(reject).finally(() => clearTimeout(timer))
    })
    const match = childOutput.stdout.match(/<pre id="browser-result">([^<]+)<\/pre>/)
    if (!match) throw new Error(`browser result missing\nstdout=${childOutput.stdout.slice(-2000)}\nstderr=${childOutput.stderr}\nserver=${serverOutput}`)
    const result = parseBrowserResult(match[1], identity)
    await cleanup()
    return result
  } finally {
    await cleanup()
    process.removeListener('SIGINT', onSignal)
    process.removeListener('SIGTERM', onSignal)
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolvePath(process.argv[1])).href) {
  runBrowserTest().then(report => { console.log(JSON.stringify(report, null, 2)); if (!report.pass) process.exitCode = 1 }).catch(error => { console.error(error.stack || error); process.exitCode = 1 })
}
