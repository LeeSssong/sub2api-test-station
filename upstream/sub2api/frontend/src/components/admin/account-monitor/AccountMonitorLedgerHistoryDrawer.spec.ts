import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountMonitorLedgerHistoryDrawer from './AccountMonitorLedgerHistoryDrawer.vue'

const history = vi.hoisted(() => vi.fn())

vi.mock('@/api/admin/reconciliation', () => ({
  history,
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('AccountMonitorLedgerHistoryDrawer', () => {
  beforeEach(() => {
    history.mockReset().mockResolvedValue({
      items: [{ day: '2026-07-24', upstream_cost: '0.02', user_charge: '1.00', paper_profit: '0.98', currency: 'USD' }],
    })
  })

  it('loads a thirty-day daily ledger for the current scope', async () => {
    const wrapper = mount(AccountMonitorLedgerHistoryDrawer, {
      props: { show: true, scope: { group_id: 3 } },
      global: { stubs: { Icon: true } },
      attachTo: document.body,
    })
    await flushPromises()

    expect(history).toHaveBeenCalledWith(expect.objectContaining({ group_id: 3, start: expect.any(String), end: expect.any(String) }))
    const params = history.mock.calls[0][0]
    expect(new Date(params.end).getTime() - new Date(params.start).getTime()).toBe(30 * 24 * 60 * 60 * 1000)
    expect(document.body.textContent).toContain('2026-07-24')
    expect(document.body.textContent).toContain('$0.02')
    wrapper.unmount()
  })

  it('shows a loading state while the daily ledger request is pending', async () => {
    history.mockReturnValueOnce(new Promise(() => {}))
    const wrapper = mount(AccountMonitorLedgerHistoryDrawer, {
      props: { show: true },
      global: { stubs: { Icon: true } },
      attachTo: document.body,
    })
    await flushPromises()

    expect(document.body.textContent).toContain('common.loading')
    expect(document.body.textContent).not.toContain('暂无账务记录')
    wrapper.unmount()
  })

  it('shows the API error instead of a misleading empty history state', async () => {
    history.mockRejectedValueOnce(new Error('历史按日返回了无效数据，请检查账务服务连接'))
    const wrapper = mount(AccountMonitorLedgerHistoryDrawer, {
      props: { show: true },
      global: { stubs: { Icon: true } },
      attachTo: document.body,
    })
    await flushPromises()

    expect(document.body.textContent).toContain('历史按日返回了无效数据')
    expect(document.body.textContent).not.toContain('暂无账务记录')
    wrapper.unmount()
  })

  it('rejects an invalid response contract instead of showing a false empty state', async () => {
    history.mockResolvedValueOnce({ rows: [] })
    const wrapper = mount(AccountMonitorLedgerHistoryDrawer, {
      props: { show: true },
      global: { stubs: { Icon: true } },
      attachTo: document.body,
    })
    await flushPromises()

    expect(document.body.textContent).toContain('账务历史返回了无效数据')
    expect(document.body.textContent).not.toContain('暂无账务记录')
    wrapper.unmount()
  })
})
