<template>
  <BaseDialog :show="show" title="账号信息" width="wide" @close="emit('close')">
    <div v-if="account" class="space-y-5" data-test="account-info-dialog">
      <section class="grid gap-3 sm:grid-cols-2" aria-label="账号基本信息">
        <InfoField label="账号名称" :value="account.name" />
        <InfoField label="账号 ID" :value="String(account.id)" mono />
        <InfoField label="平台" :value="account.platform" />
        <InfoField label="账号类型" :value="account.type" />
        <InfoField label="状态" :value="account.status" />
        <InfoField label="调度状态" :value="account.schedulable ? '可调度' : '不可调度'" />
        <InfoField label="全局优先级" :value="String(account.priority)" mono />
        <InfoField label="代理" :value="account.proxy?.name || (account.proxy_id == null ? '未配置' : `代理 #${account.proxy_id}`)" />
      </section>

      <section class="space-y-3 border-t border-gray-100 pt-4 dark:border-dark-700" aria-label="账号配置">
        <h4 class="text-sm font-semibold text-gray-800 dark:text-gray-100">账号配置</h4>
        <div class="grid gap-3 sm:grid-cols-2">
          <InfoField label="所属分组" :value="groupNames" />
          <InfoField label="倍率" :value="formatNumber(account.rate_multiplier)" mono />
          <InfoField label="到期时间" :value="formatDate(account.expires_at)" />
          <InfoField label="创建时间" :value="formatDate(account.created_at)" />
          <InfoField label="最近更新时间" :value="formatDate(account.updated_at)" />
          <InfoField label="凭据状态" :value="credentialStatus" />
        </div>
      </section>

      <section v-if="account.notes" class="space-y-3 border-t border-gray-100 pt-4 dark:border-dark-700" aria-label="账号备注">
        <h4 class="text-sm font-semibold text-gray-800 dark:text-gray-100">账号备注</h4>
        <InfoField label="备注" :value="account.notes" />
      </section>

      <section class="space-y-3 border-t border-gray-100 pt-4 dark:border-dark-700" aria-label="账号全部字段" data-test="account-all-fields">
        <h4 class="text-sm font-semibold text-gray-800 dark:text-gray-100">全部字段</h4>
        <div class="grid gap-3 sm:grid-cols-2">
          <InfoField v-for="field in allFields" :key="field.key" :label="field.key" :value="field.value" :mono="field.mono" />
        </div>
      </section>

    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, defineComponent, h } from 'vue'
import type { Account } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean; account: Account | null }>()
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

const credentialStatus = computed(() => {
  const status = props.account?.credentials_status
  if (!status) return '未返回凭据状态'
  const keys = Object.keys(status).filter((key) => status[key])
  return keys.length ? `已配置（${keys.length} 项）` : '未配置'
})

const allFields = computed(() => Object.entries(props.account ?? {}).map(([key, value]) => ({
  key,
  value: value == null || value === '' ? '--' : typeof value === 'object' ? JSON.stringify(value) : String(value),
  mono: /id|token|key|url|rate|quota|limit|time|date/i.test(key),
})))

function formatNumber(value?: number | null): string {
  return value == null || !Number.isFinite(value) ? '--' : String(value)
}

function formatDate(value?: number | string | null): string {
  if (value == null || value === '') return '--'
  const date = typeof value === 'number' ? new Date(value * 1000) : new Date(value)
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString()
}
</script>
