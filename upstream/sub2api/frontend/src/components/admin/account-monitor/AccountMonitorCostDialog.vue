<template>
  <BaseDialog :show="show" :title="`账号成本 · ${account.name}`" width="normal" @close="emit('close')">
    <form id="account-monitor-cost-form" class="space-y-4" @submit.prevent="save">
      <p class="text-sm text-gray-500 dark:text-slate-400">
        {{ usesMultiplier ? '此账号使用倍率来源；余额仅展示，不参与评分。' : '采购账号使用采购成本与预计可用额度计算成本倍率。' }}
      </p>

      <template v-if="usesMultiplier">
        <label class="block text-sm text-gray-600 dark:text-slate-300">
          当前倍率（x）
          <input v-model="draftMultiplier" data-test="multiplier-input" class="input mt-1 w-full font-mono" type="number" min="0" step="0.0001" :disabled="saving" />
        </label>
        <p class="text-xs text-gray-500 dark:text-slate-400">来源：{{ account.multiplier?.source === 'manual' ? '手工覆盖' : '上游托管' }}</p>
        <p v-if="multiplierError" class="text-sm text-red-600 dark:text-red-400" data-test="multiplier-error" role="alert">{{ multiplierError }}</p>
      </template>

      <template v-else>
        <div class="grid gap-3 sm:grid-cols-2">
          <label class="block text-sm text-gray-600 dark:text-slate-300">
            采购成本（CNY）
            <input v-model="draftCost" data-test="procurement-cost-input" class="input mt-1 w-full font-mono" type="number" min="0" step="0.01" :disabled="saving" />
          </label>
          <label class="block text-sm text-gray-600 dark:text-slate-300">
            预计可用额度（USD）
            <input v-model="draftQuota" data-test="estimated-quota-input" class="input mt-1 w-full font-mono" type="number" min="0.01" step="0.01" :disabled="saving" />
          </label>
        </div>
        <p class="text-sm text-gray-600 dark:text-slate-300" data-test="derived-multiplier">
          预计成本倍率：<span class="font-mono font-semibold">{{ derivedMultiplier }}</span>
        </p>
        <p v-if="costError" class="text-sm text-red-600 dark:text-red-400" data-test="cost-error" role="alert">{{ costError }}</p>
      </template>

      <p v-if="error" class="text-sm text-red-600 dark:text-red-400" data-test="dialog-error" role="alert">{{ error }}</p>
    </form>
    <template #footer>
      <div class="flex w-full items-center justify-between gap-2">
        <button v-if="usesProcurement && hasProcurement" type="button" class="btn btn-secondary" data-test="clear-cost" :disabled="saving" @click="emit('clear')">清空成本</button>
        <button v-else-if="isOpenAIAPIKey" type="button" class="btn btn-secondary" data-test="restore-auto" :disabled="saving" @click="emit('restoreAuto')">恢复自动获取</button>
        <span v-else />
        <div class="flex gap-2">
          <button type="button" class="btn btn-secondary" :disabled="saving" @click="emit('close')">取消</button>
          <button v-if="usesMultiplier" type="submit" form="account-monitor-cost-form" class="btn btn-primary" data-test="save-multiplier" :disabled="saving" @click.prevent="save">保存</button>
          <button v-else type="submit" form="account-monitor-cost-form" class="btn btn-primary" data-test="save-procurement" :disabled="saving" @click.prevent="save">保存</button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { AccountMonitorMultiplier } from '@/api/admin/accountMonitor'

type AccountMonitorCostAccount = {
  account_id: number
  name: string
  platform: string
  account_type: string
  procurement_cost_cny?: number | null
  estimated_usable_quota_usd?: number | null
  multiplier?: AccountMonitorMultiplier | null
}

const props = withDefaults(defineProps<{
  show: boolean
  account: AccountMonitorCostAccount
  saving?: boolean
  error?: string | null
}>(), { saving: false, error: null })

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'saveProcurement', cost: number, estimatedQuotaUSD: number): void
  (event: 'saveMultiplier', value: number): void
  (event: 'restoreAuto'): void
  (event: 'clear'): void
}>()

const isOpenAIAPIKey = computed(() => props.account.platform.toLowerCase() === 'openai' && isAPIKeyAccountType(props.account.account_type))
const usesProcurement = computed(() => isOAuthAccountType(props.account.account_type)
  || (props.account.platform.toLowerCase() === 'openai' ? !isOpenAIAPIKey.value : props.account.procurement_cost_cny != null))
const usesMultiplier = computed(() => !usesProcurement.value)
const hasProcurement = computed(() => props.account.procurement_cost_cny != null || props.account.estimated_usable_quota_usd != null)
const draftCost = ref('')
const draftQuota = ref('60')
const draftMultiplier = ref('')
const costError = ref('')
const multiplierError = ref('')

function isOAuthAccountType(value?: string | null): boolean {
  return value?.toLowerCase().replace(/[-_]/g, '') === 'oauth'
}

function isAPIKeyAccountType(value?: string | null): boolean {
  return value?.toLowerCase().replace(/[-_]/g, '') === 'apikey'
}

watch(() => [props.show, props.account] as const, ([show, account]) => {
  if (!show) return
  draftCost.value = account.procurement_cost_cny == null ? '' : String(account.procurement_cost_cny)
  draftQuota.value = account.estimated_usable_quota_usd == null ? '60' : String(account.estimated_usable_quota_usd)
  draftMultiplier.value = account.multiplier?.value == null ? '' : String(account.multiplier.value)
  costError.value = ''
  multiplierError.value = ''
}, { immediate: true, deep: true })

const derivedMultiplier = computed(() => {
  const cost = Number(draftCost.value)
  const quota = Number(draftQuota.value)
  return Number.isFinite(cost) && Number.isFinite(quota) && cost >= 0 && quota > 0 ? (cost / quota).toFixed(4) + '×' : '--'
})

function save() {
  if (props.saving) return
  if (usesMultiplier.value) {
    const value = Number(draftMultiplier.value)
    if (!String(draftMultiplier.value).trim() || !Number.isFinite(value) || value < 0) {
      multiplierError.value = '请输入大于或等于 0 的账号倍率'
      return
    }
    emit('saveMultiplier', value)
    return
  }
  const cost = Number(draftCost.value)
  const quota = Number(draftQuota.value)
  if (!String(draftCost.value).trim() || !Number.isFinite(cost) || cost < 0) {
    costError.value = '请输入大于或等于 0 的采购成本'
    return
  }
  if (!Number.isFinite(quota) || quota <= 0) {
    costError.value = '预计可用额度必须大于 0'
    return
  }
  emit('saveProcurement', cost, quota)
}
</script>
