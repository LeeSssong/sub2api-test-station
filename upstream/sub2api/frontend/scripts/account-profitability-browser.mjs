import { spawn } from 'node:child_process'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { setTimeout as delay } from 'node:timers/promises'

const root = fileURLToPath(new URL('..', import.meta.url))
const port = 4179
const profile = await mkdtemp(join(tmpdir(), 'sub2api-account-profitability-'))
const server = spawn('pnpm', ['vite', '--host', '127.0.0.1', '--port', String(port)], { cwd: root, stdio: ['ignore', 'pipe', 'pipe'] })
let serverOutput = ''
server.stdout.on('data', chunk => { serverOutput += chunk })
server.stderr.on('data', chunk => { serverOutput += chunk })
try {
  for (let attempt = 0; attempt < 50; attempt++) {
    try { await fetch(`http://127.0.0.1:${port}/browser/account-profitability.html`); break } catch { await delay(100) }
  }
  const chrome = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome'
  const debugPort = 9223
  const args = [`--headless=new`, '--no-sandbox', '--disable-gpu', '--no-first-run', '--no-default-browser-check', `--user-data-dir=${profile}`, `--remote-debugging-port=${debugPort}`, `http://127.0.0.1:${port}/browser/account-profitability.html`]
  const result = await new Promise((resolve, reject) => {
    const child = spawn(chrome, args, { stdio: ['ignore', 'pipe', 'pipe'] })
    let stdout = ''; let stderr = ''
    child.stdout.on('data', chunk => { stdout += chunk })
    child.stderr.on('data', chunk => { stderr += chunk })
    child.on('error', reject)
    const timer = setTimeout(() => child.kill('SIGTERM'), 8000)
    ;(async () => {
      let target
      for (let attempt = 0; attempt < 80 && !target; attempt++) {
        try { target = (await (await fetch(`http://127.0.0.1:${debugPort}/json`)).json()).find(item => item.type === 'page') } catch { await delay(100) }
      }
      if (!target) throw new Error('Chrome DevTools target did not start')
      const socket = new WebSocket(target.webSocketDebuggerUrl)
      await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject })
      let id = 0
      const command = (method, params = {}) => new Promise((resolve, reject) => {
        const commandId = ++id
        const onMessage = event => { const message = JSON.parse(event.data); if (message.id !== commandId) return; socket.removeEventListener('message', onMessage); message.error ? reject(new Error(JSON.stringify(message.error))) : resolve(message.result) }
        socket.addEventListener('message', onMessage); socket.send(JSON.stringify({ id: commandId, method, params }))
      })
      await command('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 1, mobile: false })
      await command('Page.reload', { ignoreCache: true })
      await delay(1800)
      const evaluation = await command('Runtime.evaluate', { expression: 'document.documentElement.outerHTML', returnByValue: true })
      stdout = evaluation.result.value
      socket.close(); child.kill('SIGTERM')
    })().catch(error => { stderr += `\n${error.stack}`; child.kill('SIGTERM') })
    child.on('close', code => { clearTimeout(timer); resolve({ code, stdout, stderr }) })
  })
  const match = result.stdout.match(/<pre id="browser-result">([^<]+)<\/pre>/)
  if (!match) throw new Error(`browser result missing (chrome=${result.code})\nstdout=${result.stdout.slice(-2000)}\nstderr=${result.stderr}\nserver=${serverOutput}`)
  const report = JSON.parse(match[1])
  console.log(JSON.stringify(report, null, 2))
  if (!report.pass) process.exitCode = 1
} finally {
  server.kill('SIGTERM')
  await rm(profile, { recursive: true, force: true })
}
