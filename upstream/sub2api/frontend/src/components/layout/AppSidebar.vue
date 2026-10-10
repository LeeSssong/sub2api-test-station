<template>
  <aside
    class="sidebar user-sidebar"
    :class="[
      sidebarCollapsed ? 'w-[72px]' : 'w-64',
      { 'admin-sidebar': isAdmin && !isUserSurface, 'admin-sidebar-collapsed': sidebarCollapsed, 'admin-sidebar-open': isAdmin && !isUserSurface && mobileOpen }
    ]"
  >
    <!-- Logo/Brand -->
    <div class="sidebar-header" :class="{ 'sidebar-header-collapsed': sidebarCollapsed }">
      <!-- Custom Logo or Default Logo -->
      <router-link
        :to="homePath"
        class="sidebar-logo flex h-9 w-9 items-center justify-center overflow-hidden rounded-xl shadow-glow transition-opacity hover:opacity-80"
        @click="handleMenuItemClick(homePath)"
      >
        <img v-if="settingsLoaded" :src="siteLogo || DEFAULT_SITE_LOGO" alt="Logo" class="h-full w-full object-contain" />
      </router-link>
      <div class="sidebar-brand" :class="{ 'sidebar-brand-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">
        <router-link
          :to="homePath"
          class="sidebar-brand-title text-lg font-bold text-gray-900 transition-colors hover:text-primary-600 dark:text-white dark:hover:text-primary-400"
          @click="handleMenuItemClick(homePath)"
        >
          {{ siteName }}
        </router-link>
        <!-- Version Badge -->
        <VersionBadge v-if="!isUserSurface" :version="siteVersion" />
        <small v-else-if="siteVersion" class="user-brand-version">{{ siteVersion.startsWith('v') ? siteVersion : `v${siteVersion}` }}</small>
      </div>
    </div>


    <!-- Navigation -->
    <nav ref="sidebarNavRef" class="sidebar-nav">
      <div v-if="authStore.isObserver" class="sidebar-section">
        <router-link to="/admin/accounts" class="sidebar-link mb-1"
          :class="{ 'sidebar-link-active': isActive('/admin/accounts'), 'sidebar-link-collapsed': sidebarCollapsed }"
          :title="sidebarCollapsed ? t('nav.accounts') : undefined"
          @click="handleMenuItemClick('/admin/accounts')">
          <GlobeIcon class="h-5 w-5 flex-shrink-0" />
          <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }">{{ t('nav.accounts') }}</span>
        </router-link>
        <router-link v-if="appStore.backendModeEnabled" to="/usage" class="sidebar-link mb-1"
          :class="{ 'sidebar-link-active': isActive('/usage'), 'sidebar-link-collapsed': sidebarCollapsed }"
          :title="sidebarCollapsed ? t('nav.usage') : undefined"
          @click="handleMenuItemClick('/usage')">
          <ChartIcon class="h-5 w-5 flex-shrink-0" />
          <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }">{{ t('nav.usage') }}</span>
        </router-link>
      </div>
      <!-- Admin View: Admin menu first, then personal menu -->
      <template v-if="isAdmin && !isUserSurface">
        <!-- Admin Section -->
        <div class="sidebar-section">
          <template v-for="item in adminNavItems" :key="item.path">
            <!-- Collapsible group (has children) -->
            <template v-if="item.children?.length">
              <button
                type="button"
                class="sidebar-link mb-1 w-full"
                :class="{
                  'sidebar-link-active': isGroupActive(item) && !isGroupExpanded(item),
                  'sidebar-link-collapsed': sidebarCollapsed
                }"
                :title="sidebarCollapsed ? item.label : undefined" :aria-label="item.label"
                @click="handleGroupClick(item)"
              >
                <component :is="item.icon" class="h-5 w-5 flex-shrink-0" />
                <span
                  class="sidebar-label sidebar-label-flex"
                  :class="{ 'sidebar-label-collapsed': sidebarCollapsed }"
                  :aria-hidden="sidebarCollapsed ? 'true' : 'false'"
                >
                  <span class="min-w-0 truncate">{{ item.label }}</span>
                  <ChevronDownIcon
                    class="h-4 w-4 flex-shrink-0 transition-transform duration-200"
                    :class="isGroupExpanded(item) ? 'rotate-180' : ''"
                  />
                </span>
              </button>
              <!-- Children -->
              <div v-if="!sidebarCollapsed && isGroupExpanded(item)" class="mb-1 ml-4 border-l border-gray-200 pl-2 dark:border-dark-600">
                <router-link
                  v-for="child in item.children"
                  :key="child.path"
                  :to="child.path"
                  class="sidebar-link mb-0.5 py-1.5 text-sm"
                  :class="{ 'sidebar-link-active': route.path === child.path }"
                  @click="handleMenuItemClick(child.path)"
                >
                  <component :is="child.icon" class="h-4 w-4 flex-shrink-0" />
                  <span>{{ child.label }}</span>
                </router-link>
              </div>
            </template>
            <!-- Normal item (no children) -->
            <router-link
              v-else
              :to="item.path"
              class="sidebar-link mb-1"
              :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': sidebarCollapsed }"
              :title="sidebarCollapsed ? item.label : undefined" :aria-label="item.label"
              :id="
                item.path === '/admin/accounts'
                  ? 'sidebar-channel-manage'
                  : item.path === '/admin/groups'
                    ? 'sidebar-group-manage'
                    : item.path === '/admin/redeem'
                      ? 'sidebar-wallet'
                      : undefined
              "
              @click="handleMenuItemClick(item.path)"
            >
              <span v-if="item.iconSvg" class="h-5 w-5 flex-shrink-0 sidebar-svg-icon" v-html="sanitizeSvg(item.iconSvg)"></span>
              <component v-else :is="item.icon" class="h-5 w-5 flex-shrink-0" />
              <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{ item.label }}</span>
              <span v-if="navBadge(item)" class="sidebar-nav-badge" :class="{ 'sidebar-nav-badge-collapsed': sidebarCollapsed }" data-testid="sidebar-nav-badge">{{ sidebarCollapsed ? '' : navBadgeText(item) }}</span>
            </router-link>
          </template>
        </div>

        <!-- Personal Section for Admin (hidden in simple mode) -->
        <div v-if="!authStore.isSimpleMode" class="sidebar-section">
          <div class="sidebar-section-title" :class="{ 'sidebar-section-title-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">
            <span class="sidebar-section-title-text" :class="{ 'sidebar-section-title-text-collapsed': sidebarCollapsed }">
              {{ t('nav.myAccount') }}
            </span>
          </div>

          <router-link
            v-for="item in personalNavItems"
            :key="item.path"
            :to="item.path"
            class="sidebar-link mb-1"
            :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': sidebarCollapsed }"
            :title="sidebarCollapsed ? item.label : undefined" :aria-label="item.label"
            :data-tour="item.path === '/keys' ? 'sidebar-my-keys' : undefined"
            @click="handleMenuItemClick(item.path)"
          >
            <span v-if="item.iconSvg" class="h-5 w-5 flex-shrink-0 sidebar-svg-icon" v-html="sanitizeSvg(item.iconSvg)"></span>
            <component v-else :is="item.icon" class="h-5 w-5 flex-shrink-0" />
            <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{ item.label }}</span>
            <span v-if="navBadge(item)" class="sidebar-nav-badge" :class="{ 'sidebar-nav-badge-collapsed': sidebarCollapsed }" data-testid="sidebar-nav-badge">{{ sidebarCollapsed ? '' : navBadgeText(item) }}</span>
          </router-link>
        </div>
      </template>

      <!-- Regular User View -->
      <template v-else-if="isUserSurface && (isAdmin || !appStore.backendModeEnabled)">
        <div class="sidebar-section">
          <router-link
            v-for="item in userNavItems"
            :key="item.path"
            :to="item.path"
            class="sidebar-link mb-1"
            :class="{ 'sidebar-link-active': isActive(item.path), 'sidebar-link-collapsed': sidebarCollapsed }"
            :aria-label="item.label"
            :aria-current="isActive(item.path) ? 'page' : undefined"
            :title="item.label"
            :data-tour="item.path === '/keys' ? 'sidebar-my-keys' : undefined"
            @click="handleMenuItemClick(item.path)"
          >
            <span v-if="item.iconSvg" class="h-5 w-5 flex-shrink-0 sidebar-svg-icon" aria-hidden="true" v-html="sanitizeSvg(item.iconSvg)"></span>
            <img v-else-if="userNavIcon(item.path)" :src="userNavIcon(item.path)" class="user-nav-icon" alt="" aria-hidden="true" />
            <component v-else :is="item.icon" class="h-5 w-5 flex-shrink-0" aria-hidden="true" />
            <span class="sidebar-label" :class="{ 'sidebar-label-collapsed': sidebarCollapsed }" :aria-hidden="sidebarCollapsed ? 'true' : 'false'">{{ item.label }}</span>
            <span v-if="navBadge(item)" class="sidebar-nav-badge" :class="{ 'sidebar-nav-badge-collapsed': sidebarCollapsed }" data-testid="sidebar-nav-badge">{{ sidebarCollapsed ? '' : navBadgeText(item) }}</span>
          </router-link>
        </div>
      </template>
    </nav>

    <!-- Shared fixed account area, outside the scrollable navigation. -->
    <div class="user-sidebar-bottom">
      <router-link
        :to="rechargeEntryPath"
        class="user-recharge-button"
        :class="{ 'user-recharge-button-collapsed': sidebarCollapsed }"
        data-testid="user-sidebar-recharge"
        :aria-label="`${formatMoney(userBalance)} ${userNavLabel('recharge', '充值')}`"
        @click="handleMenuItemClick(rechargeEntryPath)"
      >
        <strong>{{ formatMoney(userBalance) }}</strong>
        <span>{{ t('redeem.rechargeTitle') }}</span>
      </router-link>

      <div ref="accountMenuRef" class="user-sidebar-actions">
        <button
          type="button"
          class="user-account-button"
          data-testid="user-sidebar-account"
          :aria-label="t('common.userMenu')"
          :aria-expanded="accountMenuOpen"
          aria-haspopup="menu"
          @click.stop="toggleAccountMenu"
        >
          <img v-if="userAvatarUrl" :src="userAvatarUrl" :alt="displayName" />
          <span v-else class="user-account-avatar"><Icon name="user" size="md" aria-hidden="true" /></span>
          <span class="user-account-name">{{ displayName }}</span>
        </button>

        <button
          type="button"
          class="user-support-button"
          data-testid="user-sidebar-support"
          :aria-label="t('common.contactSupport')"
          :title="t('common.contactSupport')"
          @click="supportDialogOpen = true"
        >
          <Icon name="questionCircle" size="md" aria-hidden="true" />
        </button>

        <transition name="dropdown">
          <div v-if="accountMenuOpen" class="user-account-menu" role="menu">
            <div class="user-account-summary">
              <strong>{{ displayName }}</strong>
              <span>{{ user?.email }}</span>
            </div>
            <router-link to="/profile" class="user-account-menu-item" role="menuitem" @click="closeAccountMenu(); closeMobile()">
              <UserIcon class="h-4 w-4" />
              <span>{{ t('nav.profile') }}</span>
            </router-link>
            <router-link to="/keys" class="user-account-menu-item" role="menuitem" @click="closeAccountMenu(); closeMobile()">
              <KeyIcon class="h-4 w-4" />
              <span>{{ t('nav.apiKeys') }}</span>
            </router-link>
              <button type="button" class="user-account-menu-item" role="menuitem" @click="toggleTheme">
                <SunIcon v-if="isDark" class="h-4 w-4" /><MoonIcon v-else class="h-4 w-4" />
                <span>{{ isDark ? t('nav.lightMode') : t('nav.darkMode') }}</span>
              </button>
            <template v-if="isAdmin">
              <router-link v-if="isUserSurface" to="/admin/dashboard" class="user-account-menu-item" role="menuitem" @click="closeAccountMenu(); closeMobile()">
                <ChartIcon class="h-4 w-4" />
                <span>{{ t('admin.dashboard.title') }}</span>
              </router-link>
              <template v-if="!isUserSurface">
                <router-link v-if="modelPlazaEnabled" :to="{ path: '/model-plaza', query: { embedded: '1' } }" class="user-account-menu-item" role="menuitem" @click="closeAccountMenu(); closeMobile()">
                  <span>{{ t('nav.modelPlaza') }}</span>
                </router-link>

                <button type="button" class="user-account-menu-item shell-collapse-action" role="menuitem" @click="toggleSidebar(); closeAccountMenu()">
                  <ChevronDoubleRightIcon v-if="sidebarCollapsed" class="h-4 w-4" /><ChevronDoubleLeftIcon v-else class="h-4 w-4" />
                  <span>{{ sidebarCollapsed ? t('nav.expand') : t('nav.collapse') }}</span>
                </button>
                <button type="button" class="user-account-menu-item" role="menuitem" @click="closeAccountMenu(); onboardingStore.replay()">
                  <span>{{ t('onboarding.restartTour') }}</span>
                </button>
              </template>
            </template>
            <button type="button" class="user-account-menu-item user-account-menu-danger" role="menuitem" @click="handleLogout">
              <LogoutIcon class="h-4 w-4" />
              <span>{{ t('nav.logout') }}</span>
            </button>
          </div>
        </transition>
      </div>
    </div>
  </aside>

  <ContactSupportDialog :show="supportDialogOpen" @close="supportDialogOpen = false" />

  <!-- Mobile Overlay -->
  <transition name="fade">
    <div
      v-if="isAdmin && !isUserSurface && mobileOpen"
      class="fixed inset-0 z-40 bg-black/50 min-[701px]:hidden"
      @click="closeMobile"
    ></div>
  </transition>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import { useAppSurface } from '@/composables/useAppSurface'
import { computed, h, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAdminSettingsStore, useAppStore, useAuthStore, useOnboardingStore, useSupportTicketStore } from '@/stores'
import VersionBadge from '@/components/common/VersionBadge.vue'
import { sanitizeSvg } from '@/utils/sanitize'
import { sanitizeUrl } from '@/utils/url'
import { DEFAULT_SITE_LOGO } from '@/utils/branding'
import { FeatureFlags, makeSidebarFlag, isFeatureFlagEnabled } from '@/utils/featureFlags'
import ContactSupportDialog from './ContactSupportDialog.vue'

interface NavItem {
  path: string
  label: string
  icon: unknown
  iconSvg?: string
  hideInSimpleMode?: boolean
  children?: NavItem[]
  /**
   * When true, the parent item only toggles the expand/collapse state and
   * does NOT navigate to its `path`. The `path` is purely a stable key.
   */
  expandOnly?: boolean
  /**
   * 可选的功能开关 getter。返回 false 时菜单项被隐藏；返回 undefined/true 时显示。
   * 宽容策略（undefined → 显示）避免 public settings 未加载完成时菜单闪烁消失。
   * Getter 里访问的 reactive 来源（store / composable）会被 computed 自动追踪，
   * 开关切换时菜单自动更新。
   */
  featureFlag?: () => boolean | undefined
  /** Optional count shown as a red badge (a dot while the sidebar is collapsed). */
  badge?: () => number
}

// applyFeatureFlags 递归过滤掉 featureFlag() === false 的节点（含子节点）。
// 使用 `!== false` 宽容语义：undefined（设置未加载）或 true 都视为显示。
function applyFeatureFlags(items: NavItem[]): NavItem[] {
  const out: NavItem[] = []
  for (const item of items) {
    if (item.featureFlag && item.featureFlag() === false) continue
    if (item.children) {
      out.push({ ...item, children: applyFeatureFlags(item.children) })
    } else {
      out.push(item)
    }
  }
  return out
}

const { t } = useI18n()

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()
const onboardingStore = useOnboardingStore()
const adminSettingsStore = useAdminSettingsStore()

const isAdmin = computed(() => authStore.isAdmin)
const { isUserSurface } = useAppSurface()
const sidebarCollapsed = computed(() => isAdmin.value && !isUserSurface.value && appStore.sidebarCollapsed && !mobileOpen.value)
const mobileOpen = computed(() => appStore.mobileOpen)
const sidebarNavRef = ref<HTMLElement | null>(null)
const isDark = ref(document.documentElement.classList.contains('dark'))
const accountMenuOpen = ref(false)
const supportDialogOpen = ref(false)
const accountMenuRef = ref<HTMLElement | null>(null)

const homePath = computed(() => (
  isUserSurface.value ? '/dashboard' : isAdmin.value ? '/admin/dashboard' : authStore.isObserver ? '/admin/accounts' : '/dashboard'
))

// Per-group expand/collapse overrides. A group with no entry follows the
// automatic behavior (expanded while the active route is one of its children);
// a chevron click records the user's choice, which wins over the automatic
// state so an active group can still be collapsed manually.
const groupExpandOverrides = ref<Map<string, boolean>>(new Map())

// Site settings from appStore (cached, no flicker)
const siteName = computed(() => appStore.siteName)
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteVersion = computed(() => appStore.siteVersion)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))
const user = computed(() => authStore.user)
const userBalance = computed(() => Number(user.value?.balance || 0))
const rechargeEntryPath = computed(() => appStore.cachedPublicSettings?.payment_enabled === false ? '/redeem' : '/purchase')
const userAvatarUrl = computed(() => user.value?.avatar_url?.trim() || '')
const displayName = computed(() => user.value?.username || user.value?.email?.split('@')[0] || '')

// SVG Icon Components

const DashboardIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z'
        })
      ]
    )
}

const KeyIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z'
        })
      ]
    )
}

const ChartIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z'
        })
      ]
    )
}

const GiftIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M21 11.25v8.25a1.5 1.5 0 01-1.5 1.5H5.25a1.5 1.5 0 01-1.5-1.5v-8.25M12 4.875A2.625 2.625 0 109.375 7.5H12m0-2.625V7.5m0-2.625A2.625 2.625 0 1114.625 7.5H12m0 0V21m-8.625-9.75h18c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125h-18c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z'
        })
      ]
    )
}

const UserIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15.75 6a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0zM4.501 20.118a7.5 7.5 0 0114.998 0A17.933 17.933 0 0112 21.75c-2.676 0-5.216-.584-7.499-1.632z'
        })
      ]
    )
}

const UsersIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z'
        })
      ]
    )
}

const FolderIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-8.69-6.44l-2.12-2.12a1.5 1.5 0 00-1.061-.44H4.5A2.25 2.25 0 002.25 6v12a2.25 2.25 0 002.25 2.25h15A2.25 2.25 0 0021.75 18V9a2.25 2.25 0 00-2.25-2.25h-5.379a1.5 1.5 0 01-1.06-.44z'
        })
      ]
    )
}

const ChannelIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M6.429 9.75L2.25 12l4.179 2.25m0-4.5l5.571 3 5.571-3m-11.142 0L2.25 7.5 12 2.25l9.75 5.25-4.179 2.25m0 0l4.179 2.25L12 17.25 2.25 12m15.321-2.25l4.179 2.25L12 17.25l-9.75-5.25'
        })
      ]
    )
}

const CreditCardIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M2.25 8.25h19.5M2.25 9h19.5m-16.5 5.25h6m-6 2.25h3m-3.75 3h15a2.25 2.25 0 002.25-2.25V6.75A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25v10.5A2.25 2.25 0 004.5 19.5z'
        })
      ]
    )
}

const GlobeIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M12 21a9.004 9.004 0 008.716-6.747M12 21a9.004 9.004 0 01-8.716-6.747M12 21c2.485 0 4.5-4.03 4.5-9S14.485 3 12 3m0 18c-2.485 0-4.5-4.03-4.5-9S9.515 3 12 3m0 0a8.997 8.997 0 017.843 4.582M12 3a8.997 8.997 0 00-7.843 4.582m15.686 0A11.953 11.953 0 0112 10.5c-2.998 0-5.74-1.1-7.843-2.918m15.686 0A8.959 8.959 0 0121 12c0 .778-.099 1.533-.284 2.253m0 0A17.919 17.919 0 0112 16.5c-3.162 0-6.133-.815-8.716-2.247m0 0A9.015 9.015 0 013 12c0-1.605.42-3.113 1.157-4.418'
        })
      ]
    )
}


const ServerIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M5.25 14.25h13.5m-13.5 0a3 3 0 01-3-3m3 3a3 3 0 100 6h13.5a3 3 0 100-6m-16.5-3a3 3 0 013-3h13.5a3 3 0 013 3m-19.5 0a4.5 4.5 0 01.9-2.7L5.737 5.1a3.375 3.375 0 012.7-1.35h7.126c1.062 0 2.062.5 2.7 1.35l2.587 3.45a4.5 4.5 0 01.9 2.7m0 0a3 3 0 01-3 3m0 3h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008zm-3 6h.008v.008h-.008v-.008zm0-6h.008v.008h-.008v-.008z'
        })
      ]
    )
}

const BellIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M14.857 17.082a23.848 23.848 0 005.454-1.31A8.967 8.967 0 0118 9.75V9a6 6 0 10-12 0v.75a8.967 8.967 0 01-2.312 6.022c1.733.64 3.56 1.085 5.455 1.31m5.714 0a24.255 24.255 0 01-5.714 0m5.714 0a3 3 0 11-5.714 0'
        })
      ]
    )
}

const TicketIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M16.5 6v.75m0 3v.75m0 3v.75m0 3V18m-9-5.25h5.25M7.5 15h3M3.375 5.25c-.621 0-1.125.504-1.125 1.125v3.026a2.999 2.999 0 010 5.198v3.026c0 .621.504 1.125 1.125 1.125h17.25c.621 0 1.125-.504 1.125-1.125v-3.026a2.999 2.999 0 010-5.198V6.375c0-.621-.504-1.125-1.125-1.125H3.375z'
        })
      ]
    )
}

const CogIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z'
        }),
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M15 12a3 3 0 11-6 0 3 3 0 016 0z'
        })
      ]
    )
}

const SunIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M12 3v2.25m6.364.386l-1.591 1.591M21 12h-2.25m-.386 6.364l-1.591-1.591M12 18.75V21m-4.773-4.227l-1.591 1.591M5.25 12H3m4.227-4.773L5.636 5.636M15.75 12a3.75 3.75 0 11-7.5 0 3.75 3.75 0 017.5 0z'
        })
      ]
    )
}

const MoonIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M21.752 15.002A9.718 9.718 0 0118 15.75c-5.385 0-9.75-4.365-9.75-9.75 0-1.33.266-2.597.748-3.752A9.753 9.753 0 003 11.25C3 16.635 7.365 21 12.75 21a9.753 9.753 0 009.002-5.998z'
        })
      ]
    )
}

const ChevronDoubleLeftIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'm18.75 4.5-7.5 7.5 7.5 7.5m-6-15L5.25 12l7.5 7.5'
        })
      ]
    )
}

const OrderIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9 12h3.75M9 15h3.75M9 18h3.75m3 .75H18a2.25 2.25 0 002.25-2.25V6.108c0-1.135-.845-2.098-1.976-2.192a48.424 48.424 0 00-1.123-.08m-5.801 0c-.065.21-.1.433-.1.664 0 .414.336.75.75.75h4.5a.75.75 0 00.75-.75 2.25 2.25 0 00-.1-.664m-5.8 0A2.251 2.251 0 0113.5 2.25H15a2.25 2.25 0 012.15 1.586m-5.8 0c-.376.023-.75.05-1.124.08C9.095 4.01 8.25 4.973 8.25 6.108V8.25m0 0H4.875c-.621 0-1.125.504-1.125 1.125v11.25c0 .621.504 1.125 1.125 1.125h9.75c.621 0 1.125-.504 1.125-1.125V9.375c0-.621-.504-1.125-1.125-1.125H8.25zM6.75 12h.008v.008H6.75V12zm0 3h.008v.008H6.75V15zm0 3h.008v.008H6.75V18z'
        })
      ]
    )
}

const ChevronDoubleRightIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'm5.25 4.5 7.5 7.5-7.5 7.5m6-15 7.5 7.5-7.5 7.5'
        })
      ]
    )
}

const SignalIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9.348 14.651a3.75 3.75 0 010-5.303m5.304 0a3.75 3.75 0 010 5.303m-7.425 2.122a6.75 6.75 0 010-9.546m9.546 0a6.75 6.75 0 010 9.546M5.106 18.894c-3.808-3.807-3.808-9.98 0-13.788m13.788 0c3.808 3.807 3.808 9.98 0 13.788M12 12h.008v.008H12V12zm.375 0a.375.375 0 11-.75 0 .375.375 0 01.75 0z'
        })
      ]
    )
}

// Dedicated icon for the virtual performance monitor entry.  It intentionally
// uses currentColor so the existing sidebar link styles provide the neutral
// and active (teal) states in both expanded and collapsed navigation.
const PerformanceMonitorIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5', 'aria-hidden': 'true' },
      [
        h('rect', { x: '3.25', y: '4.25', width: '17.5', height: '13.5', rx: '2', 'stroke-linecap': 'round', 'stroke-linejoin': 'round' }),
        h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: 'M7 14.25l2.25-3 2.1 1.8 2.4-3.3 3.25 2.7' }),
        h('path', { 'stroke-linecap': 'round', d: 'M9 20h6M12 17.75V20' }),
      ],
    ),
}

const ShieldIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z'
        })
      ]
    )
}

const PriceTagIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M9.568 3H5.25A2.25 2.25 0 003 5.25v4.318c0 .597.237 1.17.659 1.591l9.581 9.581c.699.699 1.78.872 2.607.33a18.095 18.095 0 005.223-5.223c.542-.827.369-1.908-.33-2.607L11.16 3.66A2.25 2.25 0 009.568 3z'
        }),
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'M6 6h.008v.008H6V6z'
        })
      ]
    )
}

const ChevronDownIcon = {
  render: () =>
    h(
      'svg',
      { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' },
      [
        h('path', {
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          d: 'm19.5 8.25-7.5 7.5-7.5-7.5'
        })
      ]
    )
}

const LogoutIcon = {
  render: () =>
    h('svg', { fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' }, [
      h('path', {
        'stroke-linecap': 'round',
        'stroke-linejoin': 'round',
        d: 'M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15M12 9l-3 3m0 0l3 3m-3-3h12.75'
      })
    ])
}

// Public-settings flags go through the registry in utils/featureFlags.ts,
// which handles the opt-in vs opt-out fallback when settings haven't loaded
// yet. Admin-only flags (not in public settings) stay inline below.
const flagChannelMonitor = makeSidebarFlag(FeatureFlags.channelMonitor)

const flagAffiliate = makeSidebarFlag(FeatureFlags.affiliate)
const flagRiskControl = makeSidebarFlag(FeatureFlags.riskControl)
const QualityOpsIcon = { render: () => h(Icon, { name: 'badge', size: 'sm' }) }
const SupportTicketInboxIcon = { render: () => h(Icon, { name: 'inbox' }) }
const flagSupportTickets = makeSidebarFlag(FeatureFlags.supportTickets)
const flagUserSupportTickets = () => flagSupportTickets() && !authStore.isAdmin
const SupportTicketIcon = { render: () => h(Icon, { name: 'chat' }) }
const supportTicketStore = useSupportTicketStore()
const flagOpsMonitoring = () => adminSettingsStore.opsMonitoringEnabled
const flagAdminPayment = () => adminSettingsStore.paymentEnabled

function userNavLabel(key: string, fallback: string): string {
  const translated = t(`nav.${key}`)
  return translated === `nav.${key}` ? fallback : translated
}

function userNavIcon(path: string): string | undefined {
  const name = ({ '/dashboard': 'tools', '/usage': 'history', '/keys': 'key' } as Record<string, string>)[path]
  return name ? `/xingqiao/${name}.svg` : undefined
}

function buildUserNavItems(): NavItem[] {
  return [
    { path: '/dashboard', label: userNavLabel('aiTools', 'AI 工具'), icon: DashboardIcon },
    { path: '/image-studio', label: '生图工作站', icon: DashboardIcon },
    { path: '/usage', label: t('nav.usage'), icon: ChartIcon },
    { path: '/keys', label: userNavLabel('myKeys', '我的密钥'), icon: KeyIcon },
    { path: '/support-tickets', label: t('nav.supportTickets'), icon: SupportTicketIcon, featureFlag: flagUserSupportTickets, badge: () => supportTicketStore.userUnread },
    ...(flagAffiliate() ? [{ path: '/affiliate', label: t('nav.affiliate'), icon: GiftIcon }] : []),
    // Recharge and redemption are accessed only through the fixed balance entry.
    ...customMenuItemsForUser.value
      .filter(item => item.url === '/intelligence-test')
      .map((item): NavItem => ({
        path: '/intelligence-test',
        label: item.label === '智商检测' ? '智商监测' : item.label,
        icon: PerformanceMonitorIcon,
        iconSvg: item.icon_svg,
      })),
  ]
}

// User navigation items (for regular users)
const userNavItems = computed((): NavItem[] => applyFeatureFlags(buildUserNavItems()))

// The management console links to the same user workspace navigation.
const personalNavItems = userNavItems

// Custom menu items filtered by visibility
const customMenuItemsForUser = computed(() => {
  const items = appStore.cachedPublicSettings?.custom_menu_items ?? []
  const performanceMonitorItem = {
    id: 'performance-monitor',
    label: t('nav.performanceMonitor'),
    icon_svg: '',
    url: '/custom/performance-monitor',
    visibility: 'user' as const,
    sort_order: -1,
  }
  const configuredItems = items.filter((item) => item.visibility === 'user' && item.id !== performanceMonitorItem.id)
  if (flagChannelMonitor() === false) return configuredItems.sort((a, b) => a.sort_order - b.sort_order)
  return [performanceMonitorItem, ...configuredItems]
    .sort((a, b) => a.sort_order - b.sort_order)
})

const customMenuItemsForAdmin = computed(() => {
  return adminSettingsStore.customMenuItems
    .filter((item) => item.visibility === 'admin')
    .sort((a, b) => a.sort_order - b.sort_order)
})

// Admin navigation items
const adminNavItems = computed((): NavItem[] => {
  const baseItems: NavItem[] = [
    { path: '/admin/dashboard', label: t('nav.dashboard'), icon: DashboardIcon },
    { path: '/admin/ops', label: t('nav.ops'), icon: ChartIcon, featureFlag: flagOpsMonitoring },
    {
      path: '/admin/operations',
      label: t('nav.operations'),
      icon: ChartIcon,
      hideInSimpleMode: true,
      expandOnly: true,
      children: [
        { path: '/admin/operations/business-overview', label: t('nav.businessOverview'), icon: ChartIcon },
      ],
    },
    { path: '/admin/users', label: t('nav.users'), icon: UsersIcon, hideInSimpleMode: true },
    { path: '/admin/groups', label: t('nav.groups'), icon: FolderIcon },
    {
      path: '/admin/channels',
      label: t('nav.channelManagement'),
      icon: ChannelIcon,
      hideInSimpleMode: true,
      expandOnly: true,
      children: [
        { path: '/admin/channels/pricing', label: t('nav.channelPricing'), icon: PriceTagIcon },
        { path: '/admin/channels/monitor', label: t('nav.channelMonitor'), icon: SignalIcon, featureFlag: flagChannelMonitor },
      ],
    },
    { path: '/admin/subscriptions', label: t('nav.subscriptions'), icon: CreditCardIcon, hideInSimpleMode: true },
    { path: '/admin/accounts', label: t('nav.accounts'), icon: GlobeIcon },
    { path: '/admin/accounts/monitor', label: t('nav.accountMonitor'), icon: ChartIcon },
    { path: '/admin/smart-ops', label: t('accountOps.smartTitle'), icon: GlobeIcon, expandOnly: true, children: [
      { path: '/admin/auto-config', label: t('autoConfig.title'), icon: GlobeIcon },
      { path: '/admin/priority-scheduling', label: t('priorityScheduling.title'), icon: GlobeIcon },
      { path: '/admin/account-quality', label: t('qualityOps.title'), icon: ChartIcon },
      { path: '/admin/controlled-experiments', label: t('controlledExperiments.title'), icon: QualityOpsIcon },
      { path: '/admin/account-ops', label: t('accountOps.title'), icon: GlobeIcon },
      { path: '/admin/token-guard', label: t('tokenGuard.title'), icon: ShieldIcon },
      { path: '/admin/token-guard-v2', label: t('tokenGuardV2.title'), icon: ShieldIcon },
      { path: '/admin/pelican-tests', label: t('pelicanTests.title'), icon: ChartIcon },
      { path: '/admin/request-captures', label: t('admin.requestCapture.title'), icon: ChartIcon, featureFlag: () => adminSettingsStore.requestCaptureEnabled },
      { path: '/admin/harvest-flow', label: t('nav.harvestFlow'), icon: ChartIcon },
    ] },
    { path: '/admin/announcements', label: t('nav.announcements'), icon: BellIcon },
    { path: '/admin/support-tickets', label: t('nav.supportTickets'), icon: SupportTicketInboxIcon, featureFlag: flagSupportTickets, badge: () => supportTicketStore.adminPending },
    { path: '/admin/proxies', label: t('nav.proxies'), icon: ServerIcon },
    {
      path: '/admin/security-audit',
      label: t('nav.securityAudit'),
      icon: ShieldIcon,
      expandOnly: true,
      featureFlag: flagRiskControl,
      children: [
        { path: '/admin/risk-control', label: t('nav.contentModeration'), icon: ShieldIcon },
        { path: '/admin/prompt-audit', label: t('nav.promptAudit'), icon: ShieldIcon },
      ],
    },
    { path: '/admin/redeem', label: t('nav.redeemCodes'), icon: TicketIcon, hideInSimpleMode: true },
    { path: '/admin/promo-codes', label: t('nav.promoCodes'), icon: GiftIcon, hideInSimpleMode: true },
    {
      path: '/admin/affiliates',
      label: t('nav.affiliateManagement'),
      icon: UsersIcon,
      hideInSimpleMode: true,
      expandOnly: true,
      featureFlag: flagAffiliate,
      children: [
        { path: '/admin/affiliates/invites', label: t('nav.affiliateInviteRecords'), icon: UsersIcon },
        { path: '/admin/affiliates/rebates', label: t('nav.affiliateRebateRecords'), icon: OrderIcon },
        { path: '/admin/affiliates/transfers', label: t('nav.affiliateTransferRecords'), icon: CreditCardIcon },
      ],
    },
    {
      path: '/admin/orders',
      label: t('nav.orderManagement'),
      icon: OrderIcon,
      hideInSimpleMode: true,
      expandOnly: true,
      featureFlag: flagAdminPayment,
      children: [
        { path: '/admin/orders/dashboard', label: t('nav.paymentDashboard'), icon: ChartIcon },
        { path: '/admin/orders', label: t('nav.orderManagement'), icon: OrderIcon },
        { path: '/admin/orders/plans', label: t('nav.paymentPlans'), icon: CreditCardIcon },
      ],
    },
    { path: '/admin/usage', label: t('nav.usage'), icon: ChartIcon },
    { path: '/admin/audit-logs', label: t('nav.auditLogs'), icon: ShieldIcon, hideInSimpleMode: true }
  ]

  const visible = applyFeatureFlags(baseItems)

  // 简单模式下，在系统设置前插入 API密钥
  if (authStore.isSimpleMode) {
    const filtered = visible.filter(item => !item.hideInSimpleMode)
    filtered.push({ path: '/keys', label: t('nav.apiKeys'), icon: KeyIcon })
    filtered.push({ path: '/admin/settings', label: t('nav.settings'), icon: CogIcon })
    for (const cm of customMenuItemsForAdmin.value) {
      filtered.push({ path: cm.url === '/intelligence-test' ? '/intelligence-test' : `/custom/${cm.id}`, label: cm.url === '/intelligence-test' && cm.label === '智商检测' ? '智商监测' : cm.label, icon: null, iconSvg: cm.icon_svg })
    }
    return filtered
  }

  visible.push({ path: '/admin/settings', label: t('nav.settings'), icon: CogIcon })
  for (const cm of customMenuItemsForAdmin.value) {
    visible.push({ path: cm.url === '/intelligence-test' ? '/intelligence-test' : `/custom/${cm.id}`, label: cm.url === '/intelligence-test' && cm.label === '智商检测' ? '智商监测' : cm.label, icon: null, iconSvg: cm.icon_svg })
  }
  return visible
})

// Use exactly the visible navigation, including the personal section only when shown.

function toggleSidebar() {
  appStore.toggleSidebar()
}

function formatMoney(value: number): string {
  return `$${Number.isFinite(value) ? value.toFixed(2) : '0.00'}`
}

function toggleAccountMenu() {
  accountMenuOpen.value = !accountMenuOpen.value
}

function closeAccountMenu() {
  accountMenuOpen.value = false
}

async function handleLogout() {
  closeAccountMenu()
  try {
    await authStore.logout()
  } catch (error) {
    console.error('Logout error:', error)
  }
  await router.push('/login')
}

function handleDocumentClick(event: MouseEvent) {
  if (accountMenuRef.value && !accountMenuRef.value.contains(event.target as Node)) {
    closeAccountMenu()
  }
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function closeMobile() {
  appStore.setMobileOpen(false)
}

function handleMenuItemClick(itemPath: string) {
  if (mobileOpen.value) {
    setTimeout(() => {
      appStore.setMobileOpen(false)
    }, 150)
  }

  // Map paths to tour selectors
  const pathToSelector: Record<string, string> = {
    '/admin/groups': '#sidebar-group-manage',
    '/admin/accounts': '#sidebar-channel-manage',
    '/keys': '[data-tour="sidebar-my-keys"]'
  }

  const selector = pathToSelector[itemPath]
  if (selector && onboardingStore.isCurrentStep(selector)) {
    onboardingStore.nextStep(500)
  }
}

function navBadge(item: NavItem): number {
  return item.badge?.() ?? 0
}

function navBadgeText(item: NavItem): string {
  const count = navBadge(item)
  return count > 99 ? '99+' : String(count)
}

function isActive(path: string): boolean {
  return route.path === path || route.path.startsWith(path + '/')
}

function isGroupActive(item: NavItem): boolean {
  if (!item.children) return false
  return item.children.some(child => route.path === child.path)
}

function isGroupExpanded(item: NavItem): boolean {
  const override = groupExpandOverrides.value.get(item.path)
  if (override !== undefined) return override
  return isGroupActive(item)
}

function toggleGroup(item: NavItem) {
  groupExpandOverrides.value.set(item.path, !isGroupExpanded(item))
}

/**
 * Click handler for collapsible parent items.
 * - When sidebar is collapsed: do nothing (children are not visible).
 * - When `expandOnly` is true: only toggle expand state.
 * - Otherwise (default, e.g. /admin/orders): navigate to the parent path
 *   (router-link semantics) and ensure the group is expanded.
 */
function handleGroupClick(item: NavItem) {
  if (window.matchMedia('(max-width: 700px)').matches && !mobileOpen.value) {
    appStore.setMobileOpen(true)
  }
  if (sidebarCollapsed.value) return
  if (item.expandOnly) {
    toggleGroup(item)
    return
  }
  // Push to path and ensure expanded
  if (route.path !== item.path) {
    router.push(item.path)
  }
  groupExpandOverrides.value.set(item.path, true)
}

// Restore the same preference used before app mount; first visits start dark.
const savedTheme = localStorage.getItem('theme')
isDark.value = savedTheme !== 'light'
document.documentElement.classList.toggle('dark', isDark.value)

// Fetch admin settings (for feature-gated nav items like Ops).
watch(
  isAdmin,
  (v) => {
    if (v) {
      adminSettingsStore.fetch()
    }
  },
  { immediate: true }
)

onMounted(() => {
  document.addEventListener('click', handleDocumentClick)
  if (isAdmin.value) {
    adminSettingsStore.fetch()
  }
  // Restore sidebar scroll position after route change re-mounts the component
  if (appStore.sidebarScrollTop > 0 && sidebarNavRef.value) {
    void nextTick(() => {
      if (sidebarNavRef.value) {
        sidebarNavRef.value.scrollTop = appStore.sidebarScrollTop
      }
    })
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleDocumentClick)
  if (sidebarNavRef.value) {
    appStore.sidebarScrollTop = sidebarNavRef.value.scrollTop
  }
})
</script>

<style scoped>
.sidebar-logo {
  flex: 0 0 2.25rem;
  min-width: 2.25rem;
}

.sidebar-header-collapsed {
  gap: 0;
  padding-left: 1.125rem;
  padding-right: 1.125rem;
}

.sidebar-brand {
  min-width: 0;
  flex: 1 1 auto;
  white-space: nowrap;
  transition:
    max-width 0.22s ease,
    opacity 0.14s ease,
    transform 0.14s ease;
  max-width: 12rem;
}

.sidebar-brand-collapsed {
  max-width: 0;
  overflow: hidden;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}

.sidebar-brand-title {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar-link-collapsed {
  gap: 0;
  padding-left: 0.875rem;
  padding-right: 0.875rem;
}

.sidebar-section-title {
  position: relative;
  display: flex;
  align-items: center;
  min-height: 1.25rem;
  overflow: hidden;
  white-space: nowrap;
}

.sidebar-section-title-text {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    opacity 0.16s ease,
    transform 0.16s ease;
}

.sidebar-section-title::after {
  content: '';
  position: absolute;
  left: 0.75rem;
  right: 0.75rem;
  top: 50%;
  height: 1px;
  background: rgb(229 231 235);
  opacity: 0;
  transform: translateY(-50%);
  transition: opacity 0.18s ease;
}

.dark .sidebar-section-title::after {
  background: rgb(55 65 81);
}

.sidebar-section-title-text-collapsed {
  opacity: 0;
  transform: translateX(-4px);
}

.sidebar-section-title-collapsed::after {
  opacity: 1;
  transition-delay: 0.08s;
}

.sidebar-label {
  display: block;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  transition:
    max-width 0.2s ease,
    opacity 0.12s ease,
    transform 0.12s ease;
  max-width: 12rem;
}

.sidebar-label-flex {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}

.sidebar-link {
  position: relative;
}

.sidebar-nav-badge {
  margin-left: auto;
  flex-shrink: 0;
  min-width: 1.25rem;
  height: 1.25rem;
  padding: 0 0.375rem;
  border-radius: 9999px;
  background: rgb(239 68 68);
  color: white;
  font-size: 0.6875rem;
  font-weight: 600;
  line-height: 1.25rem;
  text-align: center;
}

.sidebar-nav-badge-collapsed {
  position: absolute;
  top: 0.4rem;
  left: 1.85rem;
  min-width: 0;
  width: 0.5rem;
  height: 0.5rem;
  padding: 0;
}

.sidebar-label-collapsed {
  max-width: 0;
  opacity: 0;
  transform: translateX(-4px);
  pointer-events: none;
}

/* Custom SVG icon in sidebar: constrain size without overriding uploaded SVG colors */
.sidebar-svg-icon {
  color: currentColor;
}

.sidebar-svg-icon :deep(svg) {
  display: block;
  width: 1.25rem;
  height: 1.25rem;
}

/* Shared chrome uses the confirmed user shell; admin navigation keeps its groups. */
.user-sidebar {
  width: 242px !important;
  transform: none !important;
  padding: 0 18px var(--xq-bottom-gutter, 24px);
  background: var(--xq-canvas);
  border-right: 1px solid var(--xq-line);
  box-shadow: none;
  color: var(--xq-text);
}
.user-sidebar .sidebar-header {
  height: var(--xq-header-height, 64px); min-height: 0; flex-shrink: 0; align-items: center;
  gap: 12px; padding: 0 10px; margin-bottom: 28px;
  border-bottom: 1px solid var(--xq-line);
}
.user-sidebar .sidebar-logo {
  width: 32px; min-width: 32px; height: 32px; margin-top: 0; flex: 0 0 32px;
  border-radius: 5px; background: #040a12; box-shadow: none;
}
.user-sidebar .sidebar-brand { min-width: 0; }
.user-sidebar .sidebar-brand-title { color: var(--xq-text); font-size: 16px; line-height: 1.6; white-space: nowrap; }
.user-brand-version { display: block; font-size: 10px; margin-top: 4px; color: var(--xq-muted); }
.user-sidebar .sidebar-nav {
  min-height: 0; padding: 0 0 16px; margin-bottom: 12px;
  scroll-padding-block: 12px; overscroll-behavior: contain;
  scrollbar-width: thin; scrollbar-color: var(--xq-border) transparent;
}
.user-sidebar .sidebar-nav::-webkit-scrollbar { width: 4px; }
.user-sidebar .sidebar-nav::-webkit-scrollbar-thumb { background: var(--xq-border); border-radius: 4px; }
.user-sidebar .sidebar-section-title { color: var(--xq-muted); }
.user-sidebar .sidebar-section { display: flex; flex-direction: column; gap: 12px; margin: 0; padding: 0; }
.user-sidebar .sidebar-link {
  position: relative; height: 42px; min-height: 42px; margin: 0; padding: 0 12px; gap: 16px;
  border: 1px solid transparent; border-radius: 9px; color: var(--xq-secondary);
  font-size: 13px; font-weight: 500; background: transparent;
  transition: color .18s ease, background .18s ease, border-color .18s ease;
}
.user-sidebar .sidebar-link:hover { color: var(--xq-text); background: color-mix(in srgb, var(--xq-raised) 42%, transparent); border-color: var(--xq-border); }
.user-sidebar .sidebar-link-active {
  color: var(--xq-text); background: var(--xq-raised);
  border-color: var(--xq-border); box-shadow: inset 3px 0 var(--xq-accent);
}
.user-nav-icon { width: 20px; height: 20px; flex-shrink: 0; }
.user-sidebar-bottom {
  position: relative; flex-shrink: 0; margin: auto -18px calc(-1 * var(--xq-bottom-gutter, 24px));
  padding: 16px 18px var(--xq-bottom-gutter, 24px);
  border-top: 1px solid var(--xq-border); background: var(--xq-depth);
}
.user-sidebar-bottom :is(button, a):focus-visible { outline: 2px solid var(--xq-accent); outline-offset: 3px; }
.user-recharge-button {
  display: flex; width: 100%; height: 58px; padding: 0 12px;
  align-items: center; justify-content: space-between; margin: 0 0 16px;
  border: 1px solid var(--xq-border); border-radius: 9px;
  background: var(--xq-surface);
  color: var(--xq-secondary); text-decoration: none;
}
.user-recharge-button strong { color: var(--xq-text); font-size: 18px; font-weight: 600; font-variant-numeric: tabular-nums; }
.user-recharge-button span { color: var(--xq-secondary); font-size: 12px; }
.user-sidebar-actions { position: relative; display: flex; gap: 8px; align-items: center; }
.user-account-button {
  display: flex; flex: 1; min-width: 0; padding: 0; gap: 10px; align-items: center;
  border: 0; background: none; text-align: left; color: var(--xq-secondary); font-size: 12px;
}
.user-account-button > img, .user-account-avatar {
  display: inline-flex; align-items: center; justify-content: center; width: 34px; height: 34px;
  flex-shrink: 0; border: 1px solid var(--xq-border); border-radius: 50%; background: var(--xq-raised);
  color: var(--xq-accent); overflow: hidden; object-fit: cover;
}

.user-account-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.user-support-button {
  display: inline-flex; align-items: center; justify-content: center; width: 40px; min-height: 38px;
  flex-shrink: 0; padding: 0; border: 1px solid var(--xq-border); border-radius: 9px; background: var(--xq-raised); color: var(--xq-secondary);
}
.user-recharge-button:hover, .user-support-button:hover { border-color: var(--xq-accent); background: var(--xq-raised); color: var(--xq-text); }
.user-account-menu {
  position: absolute; bottom: 48px; left: 0; width: 210px; z-index: 40;
  border: 1px solid var(--xq-border); border-radius: 10px; padding: 6px;
  background: var(--xq-surface); box-shadow: 0 12px 32px rgb(4 10 18 / 12%);
  max-height: calc(100dvh - 130px); overflow-y: auto;
}
.user-account-summary { padding: 8px 10px; border-bottom: 1px solid var(--xq-line); }
.user-account-summary strong, .user-account-summary span { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.user-account-summary strong { color: var(--xq-text); font-size: 12px; }
.user-account-summary span { color: var(--xq-muted); font-size: 11px; }
.user-account-menu-item { display: flex; align-items: center; gap: 8px; width: 100%; min-height: 38px; padding: 0 10px; border: 0; background: none; color: var(--xq-secondary); font-size: 12px; text-align: left; border-radius: 6px; }
.user-account-menu-item:hover { background: var(--xq-raised); color: var(--xq-text); }
.user-account-menu-danger { color: var(--xq-danger); }
@media (min-width: 701px) and (max-width: 1050px) { .user-sidebar { width: 200px !important; } }
@media (max-width: 700px) {
  .user-sidebar { width: 76px !important; padding: 0 10px var(--xq-bottom-gutter, 18px); }
  .user-sidebar .sidebar-header { padding: 0 12px; }
  .user-sidebar-bottom { margin-left: -10px; margin-right: -10px; padding-left: 10px; padding-right: 10px; }
  .user-sidebar .sidebar-brand, .user-sidebar .sidebar-label,
  .user-recharge-button strong, .user-recharge-button span, .user-account-name { display: none !important; }
  .user-sidebar .sidebar-link { justify-content: center; padding: 0; }
  .user-recharge-button { padding: 0; justify-content: center; }
  .user-recharge-button::after { content: "充值"; font-size: 12px; }
  .user-sidebar-actions { flex-direction: column; }
  .user-account-menu { left: 58px; bottom: 0; }
}
.admin-sidebar { z-index: 50; }
.admin-sidebar .sidebar-section + .sidebar-section { margin-top: 20px; }
.admin-sidebar .sidebar-section-title { flex-shrink: 0; }
.admin-sidebar .sidebar-link { flex-shrink: 0; }
@media (min-width: 701px) {
  .admin-sidebar-collapsed { width: 76px !important; padding-left: 10px; padding-right: 10px; }
  .admin-sidebar-collapsed .user-sidebar-bottom { margin-left: -10px; margin-right: -10px; padding-left: 10px; padding-right: 10px; }
  .admin-sidebar-collapsed .sidebar-header { padding-left: 12px; padding-right: 12px; }
  .admin-sidebar-collapsed .sidebar-link { justify-content: center; padding: 0; }
  .admin-sidebar-collapsed .user-recharge-button { justify-content: center; padding: 0; }
  .admin-sidebar-collapsed .user-recharge-button strong, .admin-sidebar-collapsed .user-recharge-button span,
  .admin-sidebar-collapsed .user-account-name { display: none; }
  .admin-sidebar-collapsed .user-recharge-button::after { content: "充值"; font-size: 12px; }
  .admin-sidebar-collapsed .user-sidebar-actions { flex-direction: column; }
  .admin-sidebar-collapsed .user-account-menu { left: 58px; bottom: 0; }
}
@media (max-width: 700px) {
  .shell-collapse-action { display: none; }
  .admin-sidebar:not(.admin-sidebar-open) .sidebar-section-title { display: none; }
  .admin-sidebar:not(.admin-sidebar-open) .sidebar-link > span:not(.sidebar-svg-icon),
  .admin-sidebar:not(.admin-sidebar-open) .sidebar-section > div { display: none; }
  .admin-sidebar-open { width: 242px !important; padding: 0 18px var(--xq-bottom-gutter, 18px); }
  .admin-sidebar-open .user-sidebar-bottom { margin-left: -18px; margin-right: -18px; padding-left: 18px; padding-right: 18px; }
  .admin-sidebar-open .sidebar-brand, .admin-sidebar-open .sidebar-label,
  .admin-sidebar-open .user-recharge-button strong, .admin-sidebar-open .user-recharge-button span,
  .admin-sidebar-open .user-account-name { display: block !important; }
  .admin-sidebar-open .sidebar-label-flex { display: flex !important; }
  .admin-sidebar-open .sidebar-link { justify-content: flex-start; padding: 0 12px; }
  .admin-sidebar-open .user-recharge-button { justify-content: space-between; padding: 0 12px; }
  .admin-sidebar-open .user-recharge-button::after { content: none; }
  .admin-sidebar-open .user-sidebar-actions { flex-direction: row; }
  .admin-sidebar-open .user-account-menu { left: 0; bottom: 48px; }
}
</style>
