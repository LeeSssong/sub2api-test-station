import test from 'node:test'
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { existsSync } from 'node:fs'
import { connect } from 'node:net'
import { once } from 'node:events'
import {
  buildBrowserTestUrl,
  createBrowserTestIdentity,
  findAvailablePort,
  isolatedEnv,
  isTrustedReadiness,
  parseBrowserResult,
  resolveChromeExecutable,
  runBrowserTest,
  OwnedProcessTree,
  processTable,
} from './account-profitability-browser.mjs'

function isProcessAlive(pid) {
  try {
    process.kill(pid, 0)
    return true
  } catch (error) {
    if (error?.code === 'ESRCH') return false
    throw error
  }
}

async function assertPortClosed(port, label) {
  await assert.rejects(new Promise((resolve, reject) => {
    const socket = connect({ host: '127.0.0.1', port })
    socket.once('connect', () => { socket.destroy(); resolve() })
    socket.once('error', reject)
  }), undefined, `${label} port ${port} is still accepting connections`)
}

async function assertCleanupEvidence(evidence) {
  assert.ok(evidence, 'runner did not publish cleanup evidence')
  assert.ok(evidence.chrome.pids.length > 1, 'test did not observe Chrome helper descendants')
  assert.ok(evidence.vite.pids.length > 0, 'test did not observe the Vite process tree')
  for (const pid of [...evidence.chrome.pids, ...evidence.vite.pids]) {
    assert.equal(isProcessAlive(pid), false, `owned process ${pid} survived cleanup`)
  }
  assert.equal(existsSync(evidence.chrome.profile), false, `Chrome profile survived cleanup: ${evidence.chrome.profile}`)
  await assertPortClosed(evidence.chrome.port, 'CDP')
  await assertPortClosed(evidence.vite.port, 'Vite')
}

async function runSignaledRunner(signal, expectedExitCode) {
  const runnerUrl = new URL('./account-profitability-browser.mjs', import.meta.url).href
  const source = `
    import { runBrowserTest } from ${JSON.stringify(runnerUrl)}
    runBrowserTest({
      onResources: evidence => console.log('RESOURCES:' + JSON.stringify(evidence)),
      onCleanup: evidence => console.log('CLEANUP:' + JSON.stringify(evidence)),
    }).catch(error => { console.error(error.stack || error); process.exitCode = 1 })
  `
  const child = spawn(process.execPath, ['--input-type=module', '--eval', source], { stdio: ['ignore', 'pipe', 'pipe'] })
  let stdout = ''
  let stderr = ''
  let sent = false
  child.stdout.on('data', chunk => {
    stdout += chunk
    if (!sent && stdout.includes('RESOURCES:')) {
      sent = true
      try { process.kill(child.pid, signal) } catch (error) { if (error?.code !== 'ESRCH') throw error }
    }
  })
  child.stderr.on('data', chunk => { stderr += chunk })
  const [code, exitSignal] = await once(child, 'close')
  assert.equal(exitSignal, null, `${signal} runner was killed before its cleanup handler completed`)
  assert.equal(code, expectedExitCode, `${signal} runner exited unexpectedly: ${stderr}`)
  const cleanupLine = stdout.split('\n').find(line => line.startsWith('CLEANUP:'))
  assert.ok(cleanupLine, `${signal} runner did not publish final cleanup evidence`)
  await assertCleanupEvidence(JSON.parse(cleanupLine.slice('CLEANUP:'.length)))
}

test('allocates a non-fixed loopback port', async () => {
  const first = await findAvailablePort()
  const second = await findAvailablePort()
  assert.equal(Number.isInteger(first), true)
  assert.equal(first > 0 && first < 65536, true)
  assert.equal(second > 0 && second < 65536, true)
})

test('removes inherited Vite proxy configuration', () => {
  const env = isolatedEnv({ VITE_DEV_PROXY_TARGET: 'https://production.invalid', VITE_DEV_PORT: '4179', PATH: '/bin' })
  assert.equal(env.VITE_DEV_PROXY_TARGET, undefined)
  assert.equal(env.VITE_DEV_PORT, undefined)
  assert.equal(env.VITE_BROWSER_TEST, '1')
})

test('reports invalid explicit Chrome path as a controlled failure', () => {
  assert.throws(() => resolveChromeExecutable({ BROWSER_EXECUTABLE_PATH: '/definitely/missing/chrome' }, 'linux'), /Chrome executable not found/)
})

test('rejects a competitor response that only spoofs the static readiness marker', () => {
  const identity = createBrowserTestIdentity()
  assert.equal(isTrustedReadiness('<script src="./account-profitability.ts"></script>', identity), false)
  assert.equal(isTrustedReadiness(`<meta name="browser-test-nonce" content="${identity.nonce}">`, identity), true)
})

test('binds the URL and browser result to one generated identity', () => {
  const identity = createBrowserTestIdentity()
  const url = buildBrowserTestUrl(43210, identity)
  assert.equal(new URL(url).searchParams.get('nonce'), identity.nonce)
  assert.deepEqual(parseBrowserResult(JSON.stringify({ pass: true, nonce: identity.nonce }), identity), { pass: true, nonce: identity.nonce })
  assert.throws(() => parseBrowserResult(JSON.stringify({ pass: true, nonce: 'other' }), identity), /nonce/i)
})

async function waitForValue(read, timeoutMs = 3000) {
  const deadline = Date.now() + timeoutMs
  while (Date.now() < deadline) {
    const value = await read()
    if (value) return value
    await new Promise(resolve => setTimeout(resolve, 25))
  }
  throw new Error('timed out waiting for process fixture')
}

test('cleans a detached reparented helper after its process group changes', { timeout: 15_000 }, async () => {
  const profile = `owned-profile-${Date.now()}-${process.pid}`
  const nonce = `owned-nonce-${Date.now()}-${process.pid}`
  const source = `
    const { spawn } = require('node:child_process')
    const helper = spawn(process.execPath, ['-e', 'setTimeout(() => {}, 30000)', '--', '--profile=${profile}', '--nonce=${nonce}'], { detached: true, stdio: 'ignore' })
    console.log(helper.pid)
    setTimeout(() => process.exit(0), 100)
  `
  const root = spawn(process.execPath, ['--eval', source, '--', `--profile=${profile}`, `--nonce=${nonce}`], { detached: true, stdio: ['ignore', 'pipe', 'ignore'] })
  const helperPid = Number((await once(root.stdout, 'data'))[0].toString().trim())
  const tree = new OwnedProcessTree(root, [`--nonce=${nonce}`], [`--profile=${profile}`])
  await waitForValue(async () => (await tree.refresh()).has(helperPid))
  await once(root, 'close')
  const before = (await processTable()).find(row => row.pid === helperPid)
  assert.ok(before)
  assert.notEqual(before.pgid, tree.pgid, 'fixture helper did not get a distinct process group')
  await tree.signal('SIGTERM')
  await tree.waitGone(3000, 'detached helper cleanup')
  assert.equal(isProcessAlive(helperPid), false)
})

test('does not signal a PID after its stable identity changes', { timeout: 10_000 }, async () => {
  const child = spawn(process.execPath, ['-e', 'setTimeout(() => {}, 30000)'], { detached: true, stdio: 'ignore' })
  const realRows = await waitForValue(async () => {
    const row = (await processTable()).find(candidate => candidate.pid === child.pid)
    return row ? [row] : null
  })
  let table = realRows
  const tree = new OwnedProcessTree(child, [], ['__never-owned-marker__'], async () => table)
  await tree.refresh()
  table = [{ ...realRows[0], startTime: 'different-start', command: 'unrelated-process' }]
  await tree.signal('SIGTERM')
  assert.equal(isProcessAlive(child.pid), true, 'identity mismatch must not signal the live PID')
  process.kill(child.pid, 'SIGKILL')
  await once(child, 'close')
})

test('repeated real Chrome runs leave no owned processes, profiles, or listeners', { timeout: 120_000 }, async () => {
  for (let round = 1; round <= 6; round += 1) {
    let cleanupEvidence
    const result = await runBrowserTest({ onCleanup: evidence => { cleanupEvidence = evidence } })
    assert.equal(result.pass, true, `browser layout failed in cleanup round ${round}`)
    await assertCleanupEvidence(cleanupEvidence)
  }
})

test('an exception after Chrome startup still cleans every owned resource', { timeout: 30_000 }, async () => {
  let cleanupEvidence
  await assert.rejects(
    runBrowserTest({
      onResources: () => { throw new Error('injected post-start failure') },
      onCleanup: evidence => { cleanupEvidence = evidence },
    }),
    /injected post-start failure/,
  )
  await assertCleanupEvidence(cleanupEvidence)
})

test('a stopped Chrome tree is escalated from graceful close and TERM to KILL', { timeout: 30_000 }, async () => {
  let cleanupEvidence
  const result = await runBrowserTest({
    onResources: evidence => { process.kill(-evidence.chrome.pgid, 'SIGSTOP') },
    onCleanup: evidence => { cleanupEvidence = evidence },
  })
  assert.equal(result.pass, true)
  await assertCleanupEvidence(cleanupEvidence)
})

test('SIGINT and SIGTERM await the same idempotent cleanup', { timeout: 60_000 }, async () => {
  await runSignaledRunner('SIGINT', 130)
  await runSignaledRunner('SIGTERM', 143)
})

test('invalid Chrome startup still removes the Vite tree, profile, and listener', { timeout: 30_000 }, async () => {
  const previous = process.env.BROWSER_EXECUTABLE_PATH
  let cleanupEvidence
  process.env.BROWSER_EXECUTABLE_PATH = '/definitely/missing/chrome'
  try {
    await assert.rejects(runBrowserTest({ onCleanup: evidence => { cleanupEvidence = evidence } }), /Chrome executable not found/)
  } finally {
    if (previous === undefined) delete process.env.BROWSER_EXECUTABLE_PATH
    else process.env.BROWSER_EXECUTABLE_PATH = previous
  }
  assert.ok(cleanupEvidence.vite.pids.length > 0)
  for (const pid of cleanupEvidence.vite.pids) assert.equal(isProcessAlive(pid), false, `Vite process ${pid} survived invalid Chrome cleanup`)
  assert.equal(existsSync(cleanupEvidence.chrome.profile), false)
  await assertPortClosed(cleanupEvidence.vite.port, 'Vite')
})
