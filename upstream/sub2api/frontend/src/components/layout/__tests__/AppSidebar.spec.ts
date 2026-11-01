import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AppSidebar.vue')
const componentSource = readFileSync(componentPath, 'utf8')
const stylePath = resolve(dirname(fileURLToPath(import.meta.url)), '../../../style.css')
const styleSource = readFileSync(stylePath, 'utf8')

describe('AppSidebar custom SVG styles', () => {
  it('does not override uploaded SVG fill or stroke colors', () => {
    expect(componentSource).toContain('.sidebar-svg-icon {')
    expect(componentSource).toContain('color: currentColor;')
    expect(componentSource).toContain('display: block;')
    expect(componentSource).not.toContain('stroke: currentColor;')
    expect(componentSource).not.toContain('fill: none;')
  })
})

describe('AppSidebar scroll position persistence', () => {
  it('binds a template ref to the sidebar nav element', () => {
    expect(componentSource).toContain('ref="sidebarNavRef"')
    expect(componentSource).toContain('sidebar-nav')
  })

  it('declares sidebarNavRef in script setup', () => {
    expect(componentSource).toContain("const sidebarNavRef = ref<HTMLElement | null>(null)")
  })

  it('saves scroll position on beforeUnmount', () => {
    expect(componentSource).toContain('onBeforeUnmount')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('sidebarNavRef.value.scrollTop')
  })

  it('restores scroll position on mount', () => {
    expect(componentSource).toContain('onMounted')
    expect(componentSource).toContain('appStore.sidebarScrollTop')
    expect(componentSource).toContain('nextTick')
  })
})

describe('AppSidebar collapsible groups', () => {
  it('lets the user collapse a group even while a child route is active', () => {
    // The expand state must come from the user's override first, falling back
    // to the active-route heuristic only when the user has not clicked yet.
    expect(componentSource).toContain('const groupExpandOverrides = ref<Map<string, boolean>>(new Map())')
    expect(componentSource).not.toContain('expandedGroups.value.has(item.path) || isGroupActive(item)')
  })
})

describe('AppSidebar header styles', () => {
  it('does not clip the version badge dropdown', () => {
    const sidebarHeaderBlockMatch = styleSource.match(/\.sidebar-header\s*\{[\s\S]*?\n {2}\}/)
    const sidebarBrandBlockMatch = componentSource.match(/\.sidebar-brand\s*\{[\s\S]*?\n\}/)

    expect(sidebarHeaderBlockMatch).not.toBeNull()
    expect(sidebarBrandBlockMatch).not.toBeNull()
    expect(sidebarHeaderBlockMatch?.[0]).not.toContain('@apply overflow-hidden;')
    expect(sidebarBrandBlockMatch?.[0]).not.toContain('overflow: hidden;')
  })
})

describe('AppSidebar user navigation structure', () => {
  it('sends the recharge entry to redeem when payments are explicitly disabled', () => {
    expect(componentSource).toContain(':to="rechargeEntryPath"')
    expect(componentSource).toContain("appStore.cachedPublicSettings?.payment_enabled === false ? '/redeem' : '/purchase'")
    expect(componentSource).toContain('@click="handleMenuItemClick(rechargeEntryPath)"')
  })

  it('keeps only the confirmed primary entries for regular users', () => {
    const userItemsSource = componentSource.slice(
      componentSource.indexOf('function buildUserNavItems'),
      componentSource.indexOf('// User navigation items (for regular users)'),
    )

    expect(userItemsSource).toContain("{ path: '/dashboard', label: userNavLabel('aiTools', 'AI 工具'), icon: DashboardIcon }")
    expect(userItemsSource).toContain("{ path: '/usage', label: t('nav.usage'), icon: ChartIcon }")
    expect(userItemsSource).not.toContain("path: '/keys'")
    expect(userItemsSource).not.toContain("path: '/purchase'")
    expect(userItemsSource).not.toContain("path: '/orders'")
    expect(userItemsSource).not.toContain("path: '/redeem'")
    expect(userItemsSource).not.toContain("path: '/profile'")
    expect(componentSource).toContain('data-testid="user-sidebar-recharge"')
    expect(componentSource).toContain('data-testid="user-sidebar-account"')
    expect(componentSource).toContain('data-testid="user-sidebar-support"')
    expect(componentSource).toContain(':src="siteLogo || DEFAULT_SITE_LOGO"')
  })
})


describe('AppSidebar administrator custom-menu destinations', () => {
  it.each(['filtered', 'visible'])('keeps the native intelligence destination in the %s navigation path', (list) => {
    const entry = componentSource.split(`${list}.push({ path: cm.url`)[1]?.split('})')[0]
    expect(entry).toBeDefined()
    expect(entry).toContain("=== '/intelligence-test' ? '/intelligence-test' : `/custom/${cm.id}`")
    expect(entry).toContain("cm.label === '智商检测' ? '智商监测' : cm.label")
  })
})


describe('approved operational navigation', () => {
  it('groups all native operational entries immediately after account monitoring with capture feature gating', () => {
    const monitor = componentSource.indexOf("{ path: '/admin/accounts/monitor'")
    const group = componentSource.indexOf("{ path: '/admin/smart-ops'")
    const announcements = componentSource.indexOf("{ path: '/admin/announcements'")
    expect(group).toBeGreaterThan(monitor)
    expect(group).toBeLessThan(announcements)
    const block = componentSource.slice(group, announcements)
    for (const path of ['auto-config', 'priority-scheduling', 'account-quality', 'account-ops', 'token-guard', 'token-guard-v2', 'pelican-tests', 'request-captures', 'harvest-flow']) expect(block).toContain(`/admin/${path}`)
    expect(block).toContain('expandOnly: true')
    expect(block).toContain('featureFlag: () => adminSettingsStore.requestCaptureEnabled')
    expect(componentSource).toContain(".filter(item => item.url === '/intelligence-test')")
  })
})

describe('AppSidebar smart operations group', () => {
  const smartOpsBlock = componentSource.match(/path: '\/admin\/smart-ops'[\s\S]*?\n {4}\] \},/)?.[0] ?? ''
  const pathsIn = (source: string) => [...source.matchAll(/path: '([^']+)'/g)].map(match => match[1])

  it('nests request capture and the ticket harvest flow under 智能运维', () => {
    expect(smartOpsBlock).not.toBe('')
    expect(smartOpsBlock).toMatch(/path: '\/admin\/request-captures'[^\n]*featureFlag: \(\) => adminSettingsStore\.requestCaptureEnabled/)
    expect(smartOpsBlock).toContain("path: '/admin/harvest-flow'")
    // Each entry is declared once, so neither is still a top-level item.
    expect(componentSource.match(/path: '\/admin\/request-captures'/g)).toHaveLength(1)
    expect(componentSource.match(/path: '\/admin\/harvest-flow'/g)).toHaveLength(1)
  })

  it('keeps the group in the same order as the 智能运维 tab bar', () => {
    const navSource = readFileSync(resolve(dirname(componentPath), '../admin/operations/SmartOpsNav.vue'), 'utf8')
    expect(pathsIn(smartOpsBlock).slice(1)).toEqual(pathsIn(navSource))
  })
})

describe('AppSidebar support tickets', () => {
  it('gates both entries behind the opt-in switch and keeps admins on their own page', () => {
    expect(componentSource).toContain('const flagSupportTickets = makeSidebarFlag(FeatureFlags.supportTickets)')
    expect(componentSource).toContain('const flagUserSupportTickets = () => flagSupportTickets() && !authStore.isAdmin')
    expect(componentSource).toMatch(/path: '\/support-tickets'[^\n]*featureFlag: flagUserSupportTickets[^\n]*badge: \(\) => supportTicketStore\.userUnread/)
    expect(componentSource).toMatch(/path: '\/admin\/support-tickets'[^\n]*featureFlag: flagSupportTickets[^\n]*badge: \(\) => supportTicketStore\.adminPending/)
  })

  it('renders badges for every item list and caps the number', () => {
    expect(componentSource.match(/data-testid="sidebar-nav-badge"/g)).toHaveLength(3)
    expect(componentSource).toContain("return count > 99 ? '99+' : String(count)")
    expect(componentSource).toContain('.sidebar-nav-badge-collapsed {')
  })
})
