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
})
