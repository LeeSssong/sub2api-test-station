<template>
  <BaseDialog :show="show" :title="t('admin.users.balanceHistoryTitle')" width="wide" :close-on-click-outside="true" :z-index="40" @close="$emit('close')">
    <div v-if="user" class="space-y-4">
      <!-- User header: two-row layout with full user info -->
      <div class="rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <!-- Row 1: avatar + email/username/created_at (left) + current balance (right) -->
        <div class="flex items-center gap-3">
          <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30">
            <span class="text-lg font-medium text-primary-700 dark:text-primary-300">
              {{ user.email.charAt(0).toUpperCase() }}
            </span>
          </div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <p class="truncate font-medium text-gray-900 dark:text-white">{{ user.email }}</p>
              <span v-if="user.deleted_at" class="flex-shrink-0 inline-flex items-center rounded px-1 py-px text-[10px] font-medium leading-tight bg-rose-100 text-rose-600 ring-1 ring-inset ring-rose-200 dark:bg-rose-500/20 dark:text-rose-400 dark:ring-rose-500/30">
                {{ t('admin.usage.userDeletedBadge') }}
              </span>
              <span
                v-if="user.username"
                class="flex-shrink-0 rounded bg-primary-50 px-1.5 py-0.5 text-xs text-primary-600 dark:bg-primary-900/20 dark:text-primary-400"
              >
                {{ user.username }}
              </span>
            </div>
            <p class="text-xs text-gray-400 dark:text-dark-500">
              {{ t('admin.users.createdAt') }}: {{ formatDateTime(user.created_at) }}
            </p>
          </div>
          <!-- Current balance: prominent display on the right -->
          <div class="flex-shrink-0 text-right">
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.users.currentSpendableBalance') }}</p>
            <p class="text-xl font-bold text-gray-900 dark:text-white">
              {{ quotaSummary ? `$${formatBalance(Number(quotaSummary.total_quota_balance_usd))}` : '—' }}
            </p>
          </div>
        </div>
        <!-- Row 2: notes + total recharged -->
        <div class="mt-2.5 flex items-center justify-between border-t border-gray-200/60 pt-2.5 dark:border-dark-600/60">
          <p class="min-w-0 flex-1 truncate text-xs text-gray-500 dark:text-dark-400" :title="user.notes || ''">
            <template v-if="user.notes">{{ t('admin.users.notes') }}: {{ user.notes }}</template>
            <template v-else>&nbsp;</template>
          </p>
          <p class="ml-4 flex-shrink-0 text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.users.refundableCashBalance') }}: <span class="font-semibold text-emerald-600 dark:text-emerald-400">{{ quotaSummary ? `¥${formatBalance(refundableCashBalance)}` : '—' }}</span>
          </p>
        </div>
      </div>

      <!-- Type filter + Action buttons -->
      <div class="flex items-center gap-3">
        <div class="flex rounded-lg border border-gray-200 p-1 dark:border-dark-600">
          <button class="rounded px-3 py-1 text-sm" :class="activeTab === 'legacy' ? 'bg-gray-100 dark:bg-dark-700' : ''" @click="activeTab = 'legacy'">{{ t('admin.users.legacyHistory') }}</button>
          <button class="rounded px-3 py-1 text-sm" :class="activeTab === 'quota' ? 'bg-gray-100 dark:bg-dark-700' : ''" @click="activeTab = 'quota'; loadQuotaLedger(1)">{{ t('admin.users.quotaLedger') }}</button>
        </div>
        <Select
          v-show="activeTab === 'legacy'"
          v-model="typeFilter"
          :options="typeOptions"
          class="w-56"
          @change="loadHistory(1)"
        />
        <!-- Deposit button - matches menu style -->
        <button
          v-if="!hideActions"
          @click="emit('deposit')"
          class="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
        >
          <Icon name="plus" size="sm" class="text-emerald-500" :stroke-width="2" />
          {{ t('admin.users.deposit') }}
        </button>
        <!-- Withdraw button - matches menu style -->
        <button
          v-if="!hideActions"
          @click="emit('withdraw')"
          class="flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
        >
          <svg class="h-4 w-4 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4" />
          </svg>
          {{ t('admin.users.withdraw') }}
        </button>
        <button v-if="!hideActions" class="rounded-lg border border-amber-200 px-3 py-2 text-sm text-amber-700" @click="loadRefundOrders('accounting')">{{ t('admin.users.accountingRefund') }}</button>
        <button v-if="!hideActions" class="rounded-lg border border-red-200 px-3 py-2 text-sm text-red-700" @click="loadRefundOrders('channel')">{{ t('admin.users.paymentChannelRefund') }}</button>
      </div>

      <div v-if="refundMode" class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
        <div class="mb-2 flex items-center justify-between"><span class="text-sm font-medium">{{ refundMode === 'accounting' ? t('admin.users.accountingRefund') : t('admin.users.paymentChannelRefund') }}</span><button class="text-xs text-gray-500" @click="refundMode = null">{{ t('common.close') }}</button></div>
        <div v-if="refundLoading" class="py-4 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
        <div v-else-if="refundOrders.length === 0" class="py-4 text-center text-sm text-gray-500">{{ t('admin.users.noRefundableOrders') }}</div>
        <div v-else class="space-y-2">
          <button v-for="order in refundOrders" :key="order.id" class="flex w-full items-center justify-between rounded border border-gray-200 p-2 text-left hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700" @click="emit('refund-order', order)">
            <span><span class="font-mono text-xs">#{{ order.id }}</span> <span class="ml-2 text-sm">{{ order.out_trade_no }}</span></span><span class="text-sm font-medium">${{ Number(order.paid_quota_usd ?? order.amount).toFixed(2) }}</span>
          </button>
        </div>
      </div>

      <!-- Loading -->
      <div v-if="activeTab === 'quota'" class="space-y-3">
        <div v-if="quotaLoading" class="py-8 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>
        <div v-else-if="quotaHistory.length === 0" class="py-8 text-center text-sm text-gray-500">{{ t('admin.users.noQuotaLedger') }}</div>
        <div v-else class="max-h-[28rem] space-y-3 overflow-y-auto">
          <div v-for="item in quotaHistory" :key="item.id" class="rounded-xl border border-gray-200 p-3 dark:border-dark-600">
            <div class="flex justify-between text-sm"><span class="font-medium">{{ item.record_type }}</span><span>{{ formatDateTime(item.created_at) }}</span></div>
            <div class="mt-1 grid grid-cols-3 gap-2 text-xs text-gray-500"><span>¥{{ item.cash_delta_cny }}</span><span>付费 {{ item.paid_quota_delta_usd }}</span><span>赠送 {{ item.gift_quota_delta_usd }}</span></div>
            <p v-if="item.note" class="mt-1 text-xs text-gray-500">{{ item.note }}</p>
          </div>
        </div>
      </div>

      <div v-if="activeTab === 'legacy' && loading" class="flex justify-center py-8">
        <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
        </svg>
      </div>

      <!-- Empty state -->
      <div v-else-if="activeTab === 'legacy' && history.length === 0" class="py-8 text-center">
        <p class="text-sm text-gray-500">{{ t('admin.users.noBalanceHistory') }}</p>
      </div>

      <!-- History list -->
      <div v-else-if="activeTab === 'legacy'" class="max-h-[28rem] space-y-3 overflow-y-auto">
        <div
          v-for="item in history"
          :key="item.id"
          class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800"
        >
          <div class="flex items-start justify-between">
            <!-- Left: type icon + description -->
            <div class="flex items-start gap-3">
              <div
                :class="[
                  'flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-lg',
                  getIconBg(item)
                ]"
              >
                <Icon :name="getIconName(item)" size="sm" :class="getIconColor(item)" />
              </div>
              <div>
                <p class="text-sm font-medium text-gray-900 dark:text-white">
                  {{ getItemTitle(item) }}
                </p>
                <!-- Notes (admin adjustment reason) -->
                <p
                  v-if="item.notes"
                  class="mt-0.5 text-xs text-gray-500 dark:text-dark-400"
                  :title="item.notes"
                >
                  {{ item.notes.length > 60 ? item.notes.substring(0, 55) + '...' : item.notes }}
                </p>
                <p class="mt-0.5 text-xs text-gray-400 dark:text-dark-500">
                  {{ formatDateTime(item.used_at || item.created_at) }}
                </p>
              </div>
            </div>
            <!-- Right: value -->
            <div class="text-right">
              <p :class="['text-sm font-semibold', getValueColor(item)]">
                {{ formatValue(item) }}
              </p>
              <p
                v-if="isAdminType(item.type)"
                class="text-xs text-gray-400 dark:text-dark-500"
              >
                {{ t('redeem.adminAdjustment') }}
              </p>
              <p
                v-else
                class="font-mono text-xs text-gray-400 dark:text-dark-500"
              >
                {{ item.code.slice(0, 8) }}...
              </p>
            </div>
          </div>
        </div>
      </div>

      <!-- Pagination -->
      <div v-if="activeTab === 'legacy' && totalPages > 1" class="flex items-center justify-center gap-2 pt-2">
        <button
          :disabled="currentPage <= 1"
          class="btn btn-secondary px-3 py-1 text-sm"
          @click="loadHistory(currentPage - 1)"
        >
          {{ t('pagination.previous') }}
        </button>
        <span class="text-sm text-gray-500 dark:text-dark-400">
          {{ currentPage }} / {{ totalPages }}
        </span>
        <button
          :disabled="currentPage >= totalPages"
          class="btn btn-secondary px-3 py-1 text-sm"
          @click="loadHistory(currentPage + 1)"
        >
          {{ t('pagination.next') }}
        </button>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI, type BalanceHistoryItem, type QuotaLedgerEntry, type QuotaSummary } from '@/api/admin'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { PaymentOrder } from '@/types/payment'
import { formatDateTime } from '@/utils/format'
import type { AdminUser } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{ show: boolean; user: AdminUser | null; hideActions?: boolean }>()
const emit = defineEmits(['close', 'deposit', 'withdraw', 'refund-order'])
const { t } = useI18n()

const history = ref<BalanceHistoryItem[]>([])
const loading = ref(false)
const currentPage = ref(1)
const total = ref(0)
const totalRecharged = ref(0)
const pageSize = 15
const typeFilter = ref('')
const activeTab = ref<'legacy' | 'quota'>('legacy')
const quotaHistory = ref<QuotaLedgerEntry[]>([])
const quotaLoading = ref(false)
const quotaTotal = ref(0)
const quotaPage = ref(1)
const quotaSummary = ref<QuotaSummary | null>(null)
const refundMode = ref<'accounting' | 'channel' | null>(null)
const refundOrders = ref<PaymentOrder[]>([])
const refundLoading = ref(false)

const refundableCashBalance = computed(() => {
  if (!quotaSummary.value) return 0
  return Math.max(0, Math.min(Number(quotaSummary.value.cash_balance_cny), Number(quotaSummary.value.paid_quota_balance_usd)))
})

const totalPages = computed(() => Math.ceil(total.value / pageSize) || 1)

// Type filter options
const typeOptions = computed(() => [
  { value: '', label: t('admin.users.allTypes') },
  { value: 'balance', label: t('admin.users.typeBalance') },
  { value: 'affiliate_balance', label: t('admin.users.typeAffiliateBalance') },
  { value: 'admin_balance', label: t('admin.users.typeAdminBalance') },
  { value: 'concurrency', label: t('admin.users.typeConcurrency') },
  { value: 'admin_concurrency', label: t('admin.users.typeAdminConcurrency') },
  { value: 'subscription', label: t('admin.users.typeSubscription') }
])

// Watch modal open
watch(() => props.show, (v) => {
  if (v && props.user) {
    typeFilter.value = ''
    activeTab.value = 'legacy'
    quotaHistory.value = []
    quotaSummary.value = null
    refundMode.value = null
    refundOrders.value = []
    loadHistory(1)
    void loadQuotaSummary()
  }
})

const loadRefundOrders = async (mode: 'accounting' | 'channel') => {
  if (!props.user) return
  refundMode.value = mode
  refundLoading.value = true
  try {
    const result = await adminPaymentAPI.getOrders({ user_id: props.user.id, payment_type: mode === 'accounting' ? 'admin_recharge' : undefined, page: 1, page_size: 100 })
    refundOrders.value = (result.data.items || []).filter((order) => {
      const active = order.status === 'COMPLETED' || order.status === 'PARTIALLY_REFUNDED'
      if (mode === 'accounting') return active && order.payment_type === 'admin_recharge'
      return active && order.payment_type !== 'admin_recharge' && Boolean(order.provider_instance_id)
    })
  } finally {
    refundLoading.value = false
  }
}

const loadQuotaSummary = async () => {
  if (!props.user) return
  try {
    quotaSummary.value = await adminAPI.users.getUserQuotaSummary(props.user.id)
  } catch {
    quotaSummary.value = null
  }
}

const formatBalance = (value: number) => {
  if (value === 0) return '0.00'
  const formatted = value.toFixed(8).replace(/\.?0+$/, '')
  const parts = formatted.split('.')
  if (parts.length === 1) return formatted + '.00'
  if (parts[1].length === 1) return formatted + '0'
  return formatted
}

const loadHistory = async (page: number) => {
  if (!props.user) return
  loading.value = true
  currentPage.value = page
  try {
    const res = await adminAPI.users.getUserBalanceHistory(
      props.user.id,
      page,
      pageSize,
      typeFilter.value || undefined
    )
    history.value = res.items || []
    total.value = res.total || 0
    totalRecharged.value = res.total_recharged || 0
  } catch (error) {
    console.error('Failed to load balance history:', error)
  } finally {
    loading.value = false
  }
}

const loadQuotaLedger = async (page: number) => {
  if (!props.user) return
  quotaLoading.value = true
  quotaPage.value = page
  try {
    const result = await adminAPI.users.getUserQuotaLedger(props.user.id, page, pageSize)
    quotaHistory.value = result.items
    quotaTotal.value = result.total
  } finally {
    quotaLoading.value = false
  }
}

// Helper: check if admin type
const isAdminType = (type: string) => type === 'admin_balance' || type === 'admin_concurrency'

// Helper: check if balance type (includes admin_balance)
const isBalanceType = (type: string) => type === 'balance' || type === 'admin_balance' || type === 'affiliate_balance'

// Helper: check if subscription type
const isSubscriptionType = (type: string) => type === 'subscription'

// Icon name based on type
const getIconName = (item: BalanceHistoryItem) => {
  if (isBalanceType(item.type)) return 'dollar'
  if (isSubscriptionType(item.type)) return 'badge'
  return 'bolt' // concurrency
}

// Icon background color
const getIconBg = (item: BalanceHistoryItem) => {
  if (isBalanceType(item.type)) {
    return item.value >= 0
      ? 'bg-emerald-100 dark:bg-emerald-900/30'
      : 'bg-red-100 dark:bg-red-900/30'
  }
  if (isSubscriptionType(item.type)) return 'bg-purple-100 dark:bg-purple-900/30'
  return item.value >= 0
    ? 'bg-blue-100 dark:bg-blue-900/30'
    : 'bg-orange-100 dark:bg-orange-900/30'
}

// Icon text color
const getIconColor = (item: BalanceHistoryItem) => {
  if (isBalanceType(item.type)) {
    return item.value >= 0
      ? 'text-emerald-600 dark:text-emerald-400'
      : 'text-red-600 dark:text-red-400'
  }
  if (isSubscriptionType(item.type)) return 'text-purple-600 dark:text-purple-400'
  return item.value >= 0
    ? 'text-blue-600 dark:text-blue-400'
    : 'text-orange-600 dark:text-orange-400'
}

// Value text color
const getValueColor = (item: BalanceHistoryItem) => {
  if (isBalanceType(item.type)) {
    return item.value >= 0
      ? 'text-emerald-600 dark:text-emerald-400'
      : 'text-red-600 dark:text-red-400'
  }
  if (isSubscriptionType(item.type)) return 'text-purple-600 dark:text-purple-400'
  return item.value >= 0
    ? 'text-blue-600 dark:text-blue-400'
    : 'text-orange-600 dark:text-orange-400'
}

// Item title
const getItemTitle = (item: BalanceHistoryItem) => {
  switch (item.type) {
    case 'balance':
      return t('redeem.balanceAddedRedeem')
    case 'affiliate_balance':
      return t('redeem.balanceAddedAffiliate')
    case 'admin_balance':
      return item.value >= 0 ? t('redeem.balanceAddedAdmin') : t('redeem.balanceDeductedAdmin')
    case 'concurrency':
      return t('redeem.concurrencyAddedRedeem')
    case 'admin_concurrency':
      return item.value >= 0 ? t('redeem.concurrencyAddedAdmin') : t('redeem.concurrencyReducedAdmin')
    case 'subscription':
      return t('redeem.subscriptionAssigned')
    default:
      return t('common.unknown')
  }
}

// Format display value
const formatValue = (item: BalanceHistoryItem) => {
  if (isBalanceType(item.type)) {
    const sign = item.value >= 0 ? '+' : ''
    return `${sign}$${item.value.toFixed(2)}`
  }
  if (isSubscriptionType(item.type)) {
    const days = item.validity_days || Math.round(item.value)
    const groupName = item.group?.name || ''
    return groupName ? `${days}d - ${groupName}` : `${days}d`
  }
  // concurrency types
  const sign = item.value >= 0 ? '+' : ''
  return `${sign}${item.value}`
}
</script>
