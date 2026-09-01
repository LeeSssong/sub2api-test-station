<template>
  <BaseDialog :show="show" title="账号信息" width="wide" @close="emit('close')">
    <div v-if="account" class="space-y-5" data-test="account-info-dialog">
      <section class="grid gap-3 sm:grid-cols-2" aria-label="账号基本信息">
        <InfoField label="账号名称" :value="account.name" />
        <InfoField label="账号 ID" :value="String(account.id)" mono />
        <InfoField label="平台" :value="account.platform" />
        <InfoField label="账号类型" :value="account.type" />
        <InfoField label="状态" :value="account.status" />
        <InfoField label="人工调度开关" :value="account.schedulable ? '开启' : '人工暂停'" />
        <InfoField label="有效调度状态" :value="effectiveSchedulableLabel" />
        <InfoField v-if="effectiveUnschedulableReason" label="有效阻断原因" :value="effectiveUnschedulableReason" />
        <InfoField v-if="account.effective_schedulable_at" label="有效状态快照" :value="formatDate(account.effective_schedulable_at)" mono />
      </section>

      <section class="space-y-3 border-t border-gray-100 pt-4 dark:border-dark-700" aria-label="账号运行配置">
        <h4 class="text-sm font-semibold text-gray-800 dark:text-gray-100">账号运行配置</h4>
        <div class="grid gap-3 sm:grid-cols-2">
          <InfoField label="所属分组" :value="groupNames" />
          <InfoField label="容量" :value="capacityLabel" />
          <InfoField label="代理" :value="proxyName" />
          <InfoField label="全局优先级" :value="String(account.priority)" mono />
          <InfoField label="调度评分" :value="schedulerScoreLabel" mono />
          <InfoField label="计费倍率" :value="formatMultiplier(account.rate_multiplier)" mono />
          <InfoField label="上游声明倍率" :value="upstreamMultiplierLabel" mono />
          <InfoField label="使用窗口" :value="usageWindowLabel" />
          <InfoField label="到期时间" :value="formatDate(account.expires_at)" />
          <InfoField label="最近使用" :value="formatDate(account.last_used_at)" />
          <InfoField label="创建时间" :value="formatDate(account.created_at)" />
          <InfoField label="最近更新时间" :value="formatDate(account.updated_at)" />
        </div>
      </section>

      <section v-if="account.notes" class="space-y-3 border-t border-gray-100 pt-4 dark:border-dark-700" aria-label="账号备注">
        <h4 class="text-sm font-semibold text-gray-800 dark:text-gray-100">账号备注</h4>
        <InfoField label="备注" :value="account.notes" />
      </section>

    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, defineComponent, h } from 'vue'
import type { Account } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'

type AccountInfo = Account & {
  effective_schedulable?: boolean
  effective_schedulable_at?: string | null
  effective_unschedulable_reason?: string | null
}

const props = defineProps<{ show: boolean; account: AccountInfo | null }>()
const emit = defineEmits<{ (event: 'close'): void }>()

const InfoField = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    mono: { type: Boolean, default: false },
  },
  setup(fieldProps) {
    return () => h('div', { class: 'min-w-0' }, [
      h('div', { class: 'text-xs text-gray-500 dark:text-gray-400' }, fieldProps.label),
      h('div', { class: ['mt-1 break-words text-sm text-gray-900 dark:text-white', fieldProps.mono ? 'font-mono' : ''] }, fieldProps.value || '--'),
    ])
  },
})

const groupNames = computed(() => {
  const groups = props.account?.groups?.map((group) => group.name).filter(Boolean) ?? []
  if (groups.length) return groups.join('、')
  const ids = props.account?.group_ids ?? []
  return ids.length ? ids.map((id) => `分组 #${id}`).join('、') : '未加入分组'
})

const effectiveSchedulableLabel = computed(() => {
  const account = props.account
  if (!account || account.effective_schedulable == null) return account?.schedulable ? '可调度' : '不可调度'
  return account.effective_schedulable ? '可调度' : '不可调度'
})

const effectiveUnschedulableReason = computed(() => {
  const account = props.account
  const reason = account?.effective_unschedulable_reason
  if (!reason || account?.effective_schedulable) return ''
  return ({
    inactive: '账号未激活',
    manual_disabled: '人工暂停',
    expired: '已过期',
    overload: '过载冷却',
    rate_limited: '限流冷却',
    temp_unschedulable: '临时不可调度',
    quota_exceeded: '额度耗尽',
  } as Record<string, string>)[reason] ?? reason
})

const proxyName = computed(() => {
  const account = props.account
  if (!account) return '--'
  return account.proxy?.name || (account.proxy_id == null ? '未配置' : `代理 #${account.proxy_id}`)
})

const capacityLabel = computed(() => {
  const account = props.account
  if (!account) return '--'
  const current = account.current_concurrency ?? 0
  const configured = account.concurrency > 0 ? String(account.concurrency) : '不限'
  return `${current} / ${configured}`
})

const schedulerScoreLabel = computed(() => {
  const account = props.account
  if (!account?.scheduler_score) return '--'
  const base = formatNumber(account.scheduler_score.base_score)
  const sticky = account.scheduler_score.sticky_score_infinity
    ? '+∞'
    : formatNumber(account.scheduler_score.sticky_score)
  return `${base} / ${sticky}`
})

const usageWindowLabel = computed(() => {
  const windows = (props.account as (Account & {
    usage_windows?: Array<{ name: string; utilization: number }>
  }) | null)?.usage_windows ?? []
  if (!windows.length) return '--'
  return windows.map((window) => `${window.name} ${formatPercent(window.utilization)}`).join('、')
})

const upstreamMultiplierLabel = computed(() => {
  const extra = props.account?.extra as Record<string, unknown> | undefined
  const probe = extra?.upstream_billing_probe as Record<string, unknown> | undefined
  const data = probe?.data as Record<string, unknown> | undefined
  return formatMultiplier(
    data?.effective_rate_multiplier as number | undefined
      ?? data?.resolved_rate_multiplier as number | undefined
      ?? props.account?.rate_multiplier,
  )
})

function formatNumber(value?: number | null): string {
  return value == null || !Number.isFinite(value) ? '--' : String(value)
}

function formatPercent(value?: number | null): string {
  return value == null || !Number.isFinite(value) ? '--' : `${(value * 100).toFixed(1)}%`
}

function formatMultiplier(value?: number | null): string {
  return value == null || !Number.isFinite(value) ? '--' : `${value.toFixed(2)}×`
}

function formatDate(value?: number | string | null): string {
  if (value == null || value === '') return '--'
  const date = typeof value === 'number' ? new Date(value * 1000) : new Date(value)
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString()
}
</script>
