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

function pngDimensions(path: string) {
  const bytes = readFileSync(path)
  expect(bytes.subarray(0, 8).toString('hex')).toBe('89504e470d0a1a0a')
  return {
    width: bytes.readUInt32BE(16),
    height: bytes.readUInt32BE(20),
  }
}

describe('Xingqiao beginner guide', () => {
  it('uses approved Xingqiao content and omits disabled features', () => {
    const html = readFileSync(guidePath, 'utf8')
    for (const required of [
      '星桥AI 小白使用教程',
      'https://api.xingqiaolab.top/',
      '1080152144',
      '适合第一次使用 API、Codex 和 CC Switch 的用户',
      '充值并确认余额',
      '创建 API 密钥',
      '一键导入 CC Switch',
      '启动 Codex 并完成首次测试',
      '查看 Token 和扣费',
      '六、常见报错',
      '密钥安全建议',
      '八、完成检查',
      '输入 Token',
      '用户扣费',
    ]) expect(html).toContain(required)

    for (const forbidden of [
      'tkapi.fun',
      'xmhbao.cn',
      'sslip.io',
      '邮箱验证码',
      '邮箱验证',
      '一、注册',
      '安装 Codex',
      '安装 CC Switch',
      '邀请好友',
      '20% 返利',
      'pan.quark.cn',
    ]) expect(html).not.toContain(forbidden)
  })

  it('ships every approved local screenshot', () => {
    for (const name of assetNames) {
      expect(existsSync(resolve(publicRoot, 'docs/assets', name))).toBe(true)
    }

    // Keep the CC Switch capture focused on the Xingqiao provider row only.
    expect(pngDimensions(resolve(publicRoot, 'docs/assets/04-ccswitch.png'))).toEqual({
      width: 1365,
      height: 58,
    })
  })

  it('keeps normal user links on the Xingqiao origin', () => {
    const dom = new JSDOM(readFileSync(guidePath, 'utf8'))
    const allowed = new Set([
      '/',
      '/keys',
      '/usage',
      '/support',
    ])
    const hrefs = [...dom.window.document.querySelectorAll<HTMLAnchorElement>('a[href]')]
      .map((anchor) => anchor.getAttribute('href'))
    expect(hrefs.length).toBeGreaterThan(5)
    for (const href of hrefs) {
      expect(href).toBeTruthy()
      expect(href === '/' || href?.startsWith('#') || allowed.has(href ?? '')).toBe(true)
    }

    expect(dom.window.document.querySelectorAll('#toc-links a')).toHaveLength(16)
  })
})
