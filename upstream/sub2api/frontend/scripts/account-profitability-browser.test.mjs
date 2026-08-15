import test from 'node:test'
import assert from 'node:assert/strict'
import { findAvailablePort, isolatedEnv, resolveChromeExecutable } from './account-profitability-browser.mjs'

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
