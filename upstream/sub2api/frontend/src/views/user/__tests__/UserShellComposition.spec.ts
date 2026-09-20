import { readFileSync } from 'node:fs'

import { describe, expect, it } from 'vitest'

const dashboard = readFileSync('src/views/user/DashboardView.vue', 'utf8')
const payment = readFileSync('src/views/user/PaymentView.vue', 'utf8')
const redeem = readFileSync('src/views/user/RedeemView.vue', 'utf8')
const usage = readFileSync('src/views/user/UsageView.vue', 'utf8')
const keys = readFileSync('src/views/user/KeysView.vue', 'utf8')
const profile = readFileSync('src/views/user/ProfileView.vue', 'utf8')
const orders = readFileSync('src/views/user/UserOrdersView.vue', 'utf8')

describe('confirmed user shell composition', () => {
  it('uses the native dashboard route as the AI tools workspace', () => {
    expect(dashboard).toContain('data-testid="ai-tools-workspace"')
    expect(dashboard).toContain('请选择你的 AI 工具')
    expect(dashboard).toContain('我的 AI 线路')
    expect(dashboard).toContain('getHybridPerformanceSnapshot')
    expect(dashboard).toContain('userGroupsAPI.getAvailable')
    expect(dashboard).toContain('keysAPI.list')
    expect(dashboard).not.toContain('<UserDashboardStats')
    expect(dashboard).not.toContain('<HybridPerformancePanel')
  })


  it('adds the frozen page hierarchy to native business pages', () => {
    expect(usage).toContain('<UserPageHeader title="使用记录"')
    expect(keys).toContain('<UserPageHeader title="我的密钥"')
    expect(keys).toContain("new URLSearchParams(window.location.search).get('group_id')")
    expect(payment).toContain('<UserPageHeader title="充值与兑换"')
    expect(redeem).toContain('<UserPageHeader title="充值与兑换"')
    expect(orders).toContain('<UserPageHeader title="我的订单"')
    expect(profile).toContain('<UserPageHeader title="个人资料"')
  })

  it('shares recharge navigation and balance across recharge and redeem pages', () => {
    expect(payment).toContain('<UserRechargeNav active="recharge"')
    expect(redeem).toContain('<UserRechargeNav active="redeem"')
    expect(payment).toContain(':amounts="[10, 30, 50, 100]"')
    expect(payment).toContain('const amount = ref(30)')
  })

  it('keeps orders as a secondary page with a recharge return action', () => {
    expect(orders).toContain('data-testid="back-to-recharge"')
    expect(orders).toContain("router.push('/purchase')")
  })
})
