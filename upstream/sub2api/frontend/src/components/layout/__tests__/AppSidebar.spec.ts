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
  it('keeps only the confirmed primary entries for regular users', () => {
    const userItemsSource = componentSource.slice(
      componentSource.indexOf('function buildUserNavItems'),
      componentSource.indexOf('// Personal navigation items'),
    )

    expect(userItemsSource).toContain("{ path: '/dashboard', label: userNavLabel('myRoutes', '我的线路'), icon: DashboardIcon }")
    expect(userItemsSource).toContain("{ path: '/usage', label: t('nav.usage'), icon: ChartIcon }")
    expect(userItemsSource).toContain("{ path: '/keys', label: userNavLabel('myKeys', '我的密钥'), icon: KeyIcon }")
    expect(userItemsSource).not.toContain("path: '/purchase'")
    expect(userItemsSource).not.toContain("path: '/orders'")
    expect(userItemsSource).not.toContain("path: '/redeem'")
    expect(userItemsSource).not.toContain("path: '/profile'")
    expect(componentSource).toContain('data-testid="user-sidebar-recharge"')
    expect(componentSource).toContain('data-testid="user-sidebar-account"')
    expect(componentSource).toContain('data-testid="user-sidebar-support"')
    expect(componentSource).toContain("'/xingqiao-brand-logo.png'")
  })
})
