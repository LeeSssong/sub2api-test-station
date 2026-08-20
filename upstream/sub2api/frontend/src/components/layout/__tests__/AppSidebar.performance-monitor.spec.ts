import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'

describe('performance monitor navigation contract', () => {
  it('uses the custom page path and removes the legacy fixed monitor path', () => {
    const source = readFileSync('src/components/layout/AppSidebar.vue', 'utf8')
    expect(source).toContain("t('nav.performanceMonitor')")
    expect(source).toContain("id: 'performance-monitor'")
    expect(source).not.toContain("path: '/monitor', label: t('nav.channelStatus')")
  })
})
