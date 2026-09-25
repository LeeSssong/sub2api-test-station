import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

const aiCss = readFileSync('src/styles/xingqiao-ai.css', 'utf8')
const header = readFileSync('src/components/user/UserPageHeader.vue', 'utf8')
const payment = readFileSync('src/views/user/PaymentView.vue', 'utf8')

describe('OPT-003 compact typography', () => {
  it('keeps the AI workspace page and tool headings on the agreed scale', () => {
    expect(aiCss).toMatch(/\.xq-ai \.page-head h1\{[^}]*font-size:20px;[^}]*font-weight:600/)
    expect(aiCss).toMatch(/\.xq-ai \.tool-title\{[^}]*font-size:15px;[^}]*font-weight:600/)
    expect(aiCss).toMatch(/\.xq-ai \.tool-type\{[^}]*font-size:11px/)
    expect(aiCss).toMatch(/\.xq-ai \.tool-card>small\{[^}]*font-size:11px/)
    expect(aiCss).toMatch(/\.xq-ai \.rate-badge,\.xq-dialog \.rate-badge\{[^}]*font-size:11px/)
    expect(aiCss).toMatch(/\.xq-dialog \.modal-title\{[^}]*font-size:20px/)
    expect(aiCss).not.toMatch(/@media\(max-width:700px\)\{[^\n]*\.xq-dialog \.modal-title\{font-size:17px\}/)
  })

  it('uses a 20px page title without changing shared user-shell CSS', () => {
    expect(header).toMatch(/\.user-page-header h1\s*\{[^}]*font-size:\s*20px;[^}]*font-weight:\s*600/)
  })

  it('keeps the recharge panel heading and main amount at 14px and 28px', () => {
    expect(payment).toMatch(/text-\[14px\] font-semibold leading-\[23px\] text-\[#f1f9f9\]">金额核对/)
    expect(payment).toMatch(/text-\[28px\] font-semibold leading-\[\d+px\] text-\[#eaf9f9\]/)
    expect(payment).toContain('text-[11px] leading-4 text-[#61c9d9]">本次支付')
  })
})
