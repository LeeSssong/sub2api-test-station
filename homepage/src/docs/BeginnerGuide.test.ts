import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { JSDOM } from 'jsdom'
import { describe, expect, it } from 'vitest'

const publicRoot = resolve(process.cwd(), 'public')
const guidePath = resolve(publicRoot, 'docs/index.html')
const assetNames = [
  '01-create-key.png',
  '02-select-group.png',
  '03-key-actions.png',
  '04-ccswitch.png',
  '05-usage-and-billing.png',
]

describe('Xingqiao beginner guide', () => {
  it('uses approved Xingqiao content and omits disabled features', () => {
    const html = readFileSync(guidePath, 'utf8')
    for (const required of [
      '星桥AI 小白使用教程',
      'https://api.xingqiaolab.top/',
      '1080152144',
      '充值并确认余额',
      '创建 API 密钥',
      '导入 CC Switch',
      '查看 Token 和扣费',
      '密钥安全建议',
    ]) expect(html).toContain(required)

    for (const forbidden of [
      'tkapi.fun',
      'xmhbao.cn',
      'sslip.io',
      '邮箱验证码',
      '邀请好友',
      '20% 返利',
      'pan.quark.cn',
    ]) expect(html).not.toContain(forbidden)
  })

  it('ships every approved local screenshot', () => {
    for (const name of assetNames) {
      expect(existsSync(resolve(publicRoot, 'docs/assets', name))).toBe(true)
    }
  })

  it('keeps normal user links on the Xingqiao origin', () => {
    const dom = new JSDOM(readFileSync(guidePath, 'utf8'))
    const allowed = new Set([
      '/',
      '/keys',
      '/usage',
      '/custom/xingqiao-storefront',
      '/support',
    ])
    const hrefs = [...dom.window.document.querySelectorAll<HTMLAnchorElement>('a[href]')]
      .map((anchor) => anchor.getAttribute('href'))
    expect(hrefs.length).toBeGreaterThan(5)
    for (const href of hrefs) {
      expect(href).toBeTruthy()
      expect(href === '/' || href?.startsWith('#') || allowed.has(href ?? '')).toBe(true)
    }
  })
})
