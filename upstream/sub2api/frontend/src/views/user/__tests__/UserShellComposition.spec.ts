import { readFileSync } from 'node:fs'

import { describe, expect, it } from 'vitest'

const dashboard = readFileSync('src/views/user/DashboardView.vue', 'utf8')
const payment = readFileSync('src/views/user/PaymentView.vue', 'utf8')
const redeem = readFileSync('src/views/user/RedeemView.vue', 'utf8')
const orders = readFileSync('src/views/user/UserOrdersView.vue', 'utf8')

describe('confirmed user shell composition', () => {
  it('keeps dashboard stats and embeds group performance only', () => {
    expect(dashboard).toContain('<UserDashboardStats')
    expect(dashboard).toContain('<HybridPerformancePanel')
    expect(dashboard).toContain(':show-platform-breakdown="false"')
    expect(dashboard).not.toContain('<UserDashboardCharts')
    expect(dashboard).not.toContain('<UserDashboardRecentUsage')
    expect(dashboard).not.toContain('<UserDashboardQuickActions')
  })

  it('shares recharge navigation and balance across recharge and redeem pages', () => {
    expect(payment).toContain('<UserRechargeNav active="recharge"')
    expect(redeem).toContain('<UserRechargeNav active="redeem"')
    expect(payment).toContain(':amounts="[10, 30, 50, 50]"')
    expect(payment).toContain('const amount = ref(30)')
  })

  it('keeps orders as a secondary page with a recharge return action', () => {
    expect(orders).toContain('data-testid="back-to-recharge"')
    expect(orders).toContain("router.push('/purchase')")
  })
})
