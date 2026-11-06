// Run against the local browser fixture. Never navigates to a live site or executes downloaded scripts.
import fs from 'node:fs'
import path from 'node:path'
import assert from 'node:assert/strict'
import crypto from 'node:crypto'
const { chromium } = await import(process.env.PLAYWRIGHT_MODULE_PATH || 'playwright')
const browser = await chromium.launch({ headless: true, executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE })
const origin = process.env.TUTORIAL_PREVIEW_ORIGIN || 'http://127.0.0.1:53231'
const out = path.resolve('output/config-tutorial')
fs.mkdirSync(out, { recursive: true })
const checks = [], errors = []
const steps = ['下载脚本后，复制首次启动命令', '按 ⌘空格打开「终端」，粘贴命令并回车，再选择刚下载的脚本。', '输入 1 连接星桥，看到「配置已保存」后重新打开 Codex']
try {
  for (const [name, width, height] of [['desktop', 1440, 900], ['mobile', 390, 844]]) {
    const context = await browser.newContext({ viewport: { width, height }, permissions: ['clipboard-read', 'clipboard-write'], acceptDownloads: true })
    const page = await context.newPage()
    page.on('pageerror', e => errors.push(e.message))
    const external = []
    page.on('request', r => { if (!r.url().startsWith(origin) && !r.url().startsWith('blob:') && !r.url().startsWith('data:')) external.push(r.url()) })
    await page.goto(`${origin}/src/components/keys/__tests__/browser/codex-config-tutorial.html`)
    await page.locator('[data-testid="quick-config-catalog-ready"]').waitFor()
    await page.waitForFunction(() => document.querySelector('video')?.readyState >= 3)
    assert.deepEqual(await page.locator('[data-testid="quick-config-mac-steps"] li').allTextContents(), steps)
    assert.equal(await page.locator('[data-testid="quick-config-run-instruction"]').count(), 0)
    assert.equal(await page.locator('[data-guide], .driver-popover, .config-confirmation').count(), 0)
    const video = page.locator('video'), button = page.locator('[data-testid="quick-config-copy-startup"]')
    const media = await page.request.get(new URL(await video.getAttribute('src'), origin).href)
    const mediaSha256 = crypto.createHash('sha256').update(await media.body()).digest('hex')
    assert.equal(mediaSha256, '911213ebdb2b94e18cee570dfbb6b801aa3247155396160e813a079a7c47ddfe')
    await page.locator('details').first().evaluate(e => { e.open = true })
    const continuity = []
    for (const mode of ['mouse-paused-first', 'keyboard-paused-repeat', 'mouse-playing-repeat', 'fallback-paused-repeat']) {
      await video.evaluate(async (v, mode) => { v.muted = true; v.currentTime = 8; await new Promise(r => v.addEventListener('seeked', r, { once: true })); if (mode.includes('playing')) await v.play(); else v.pause() }, mode)
      if (mode.startsWith('fallback')) await page.evaluate(() => Object.defineProperty(window, 'isSecureContext', { configurable: true, value: false }))
      await button.scrollIntoViewIfNeeded(); await button.focus()
      await page.evaluate(() => { const body = document.querySelector('.modal-body'), v = document.querySelector('video'), b = document.querySelector('[data-testid="quick-config-copy-startup"]'); window.__before = { body, v, b, scroll: body.scrollTop, time: v.currentTime, paused: v.paused, details: [...document.querySelectorAll('details')].map(e => [e, e.open]), now: performance.now() } })
      if (mode.startsWith('keyboard')) await page.keyboard.press('Enter'); else await button.click()
      await page.waitForTimeout(150)
      const result = await page.evaluate(() => { const a = window.__before, body = document.querySelector('.modal-body'), v = document.querySelector('video'), b = document.querySelector('[data-testid="quick-config-copy-startup"]'); return { before: a.scroll, after: body.scrollTop, bodySame: a.body === body, videoSame: a.v === v, buttonSame: a.b === b, focusSame: document.activeElement === b, timeBefore: a.time, timeAfter: v.currentTime, elapsed: (performance.now() - a.now) / 1000, pausedSame: a.paused === v.paused, detailsSame: a.details.every(([e, open]) => e.isConnected && e.open === open), feedback: b.textContent.trim() } })
      assert.ok(result.before > 0 && Math.abs(result.before - result.after) <= 1)
      assert.ok(result.bodySame && result.videoSame && result.buttonSame && result.focusSame && result.pausedSame && result.detailsSame)
      assert.ok(result.timeAfter >= result.timeBefore - .04 && result.timeAfter - result.timeBefore <= result.elapsed + .3)
      assert.equal(result.feedback, '已复制')
      if (mode.includes('playing')) assert.ok(result.timeAfter > result.timeBefore)
      const actualCommand = await page.evaluate(() => navigator.clipboard.readText())
      assert.ok(actualCommand.includes('/usr/bin/osascript') && actualCommand.includes('/bin/bash "$xq_script"'))
      continuity.push({ mode, ...result, realClipboardVerified: true })
      if (mode.startsWith('fallback')) await page.evaluate(() => { delete window.isSecureContext })
    }
    await video.evaluate(v => v.pause()); await page.mouse.move(0, 0)
    const overflow = await page.locator('.modal-body').evaluate(e => ({ body: e.scrollWidth > e.clientWidth + 1, viewport: document.documentElement.scrollWidth > innerWidth, stepPadding: getComputedStyle(e.querySelector('li')).paddingLeft }))
    assert.ok(!overflow.body && !overflow.viewport)
    await page.screenshot({ path: path.join(out, `${name}.png`) })
    // True native video fullscreen using a trusted click.
    await video.scrollIntoViewIfNeeded(); await video.evaluate(v => v.addEventListener('click', () => v.requestFullscreen(), { once: true })); await video.click({ position: { x: 18, y: 18 } })
    await page.waitForFunction(() => document.fullscreenElement?.tagName === 'VIDEO'); await page.evaluate(() => document.exitFullscreen())
    const downloadEvent = page.waitForEvent('download'); await page.locator('[data-testid="quick-config-download"]').click(); const download = await downloadEvent
    assert.equal(download.suggestedFilename(), 'xingqiao-codex-config.command'); const content = fs.readFileSync(await download.path(), 'utf8'); assert.ok(content.includes('CATALOG_PAYLOAD=') && content.includes('sk-preview-invalid-not-a-key') === false) // key lives in the generated base64 payload
    await video.evaluate(v => v.play()); const old = await video.elementHandle(); await page.locator('[data-testid="quick-config-windows-tab"]').click(); assert.equal(await old.evaluate(v => v.paused), true)
    assert.equal(await page.locator('video').count(), 0); assert.ok((await page.locator('[data-testid="quick-config-run-instruction"]').innerText()).includes('.cmd'))
    await page.locator('[data-testid="quick-config-linux-tab"]').click(); assert.ok((await page.locator('[data-testid="quick-config-run-instruction"]').innerText()).includes('目录'))
    await page.locator('[data-testid="quick-config-unix-tab"]').click(); await page.waitForFunction(() => document.querySelector('video')?.readyState >= 3); const closing = await page.locator('video').elementHandle(); await closing.evaluate(v => { v.muted = true; return v.play() }); await page.locator('[data-testid="quick-config-close"]').click(); assert.equal(await closing.evaluate(v => v.paused), true); await page.locator('#preview-reopen').click(); await page.locator('video').waitFor()
    assert.equal(await page.locator('[data-testid="quick-config-copy-startup"]').innerText(), '复制首次启动命令')
    // Actual failed media request, with a failed optional catalog: written steps and native copy/download stay available.
    await page.route('**/macos-top-caption-voice.mp4*', r => r.request().resourceType() === 'media' ? r.abort() : r.continue())
    await page.goto(`${origin}/src/components/keys/__tests__/browser/codex-config-tutorial.html?catalog=error`)
    await page.locator('[data-testid="quick-config-video-error"]').waitFor(); await page.locator('[data-testid="quick-config-catalog-error"]').waitFor()
    assert.deepEqual(await page.locator('[data-testid="quick-config-mac-steps"] li').allTextContents(), steps)
    await page.locator('[data-testid="quick-config-copy-startup"]').click(); await page.waitForFunction(() => document.querySelector('[data-testid="quick-config-copy-startup"]').textContent.includes('已复制'))
    assert.equal(await page.locator('[data-testid="quick-config-download"]').isEnabled(), true)
    await page.unroute('**/macos-top-caption-voice.mp4*'); await page.getByRole('button', { name: '重试加载' }).click(); await page.waitForFunction(() => document.querySelector('video')?.readyState >= 3)
    assert.equal(await page.locator('[data-testid="quick-config-video-error"]').count(), 0)
    assert.equal(external.length, 0)
    checks.push({ name, viewport: { width, height }, mediaSha256, continuity, overflow, fullscreen: 'VIDEO', realDownload: true, switchAndClosePause: true, mediaAndCatalogFailureRecovery: true, external })
    await context.close()
  }
  const page = await browser.newPage(); await page.goto(`${origin}/src/components/keys/__tests__/browser/codex-config-tutorial.html?locale=en`); await page.locator('video').waitFor(); assert.equal(await page.locator('[data-testid="quick-config-mac-steps"] li').count(), 3); assert.ok((await page.locator('[data-testid="quick-config-mac-steps"]').innerText()).includes('Terminal')); assert.ok((await page.locator('body').innerText()).includes('Chinese audio')); await page.close()
  assert.equal(errors.length, 0); fs.writeFileSync(path.join(out, 'browser-verification.json'), JSON.stringify({ checks, english: true, errors, limits: ['Chromium desktop and 390 viewport; not Safari or a physical device', 'Invalid demo key/local catalog fixture; no script executed or live account request'] }, null, 2)); console.log(JSON.stringify({ passed: checks.map(c => c.name), english: true, errors }))
} finally { await browser.close() }
