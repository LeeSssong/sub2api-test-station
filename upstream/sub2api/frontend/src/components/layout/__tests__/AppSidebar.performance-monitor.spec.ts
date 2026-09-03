import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

const source = readFileSync('src/components/layout/AppSidebar.vue', 'utf8')

describe('performance monitor navigation contract', () => {
  it('uses the custom page path and removes the legacy fixed monitor path', () => {
    expect(source).toContain("t('nav.performanceMonitor')")
    expect(source).toContain("id: 'performance-monitor'")
    expect(source).toContain('PerformanceMonitorIcon')
    expect(source).toContain("item.id === 'performance-monitor' ? PerformanceMonitorIcon : null")
    expect(source).not.toContain("path: '/monitor', label: t('nav.channelStatus')")
    expect(source).not.toContain("path: '/subscriptions', label: t('nav.mySubscriptions')")
    expect(source).not.toContain("path: '/admin/operations/account-profitability', label: t('nav.accountProfitability')")
  })

  it('places the administrator scheduler settings entry before system settings', () => {
    const schedulerEntry = "path: '/admin/scheduler-logs', label: t('nav.schedulerLogs'), icon: OrderListIcon"
    const settingsEntry = "path: '/admin/settings', label: t('nav.settings'), icon: CogIcon"

    expect(source).toContain(schedulerEntry)
    expect(source.indexOf(schedulerEntry)).toBeLessThan(source.indexOf(settingsEntry))
  })
})
