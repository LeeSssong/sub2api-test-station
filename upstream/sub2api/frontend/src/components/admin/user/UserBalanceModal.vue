<template>
  <BaseDialog :show="show" :title="operation === 'add' ? t('admin.users.deposit') : t('admin.users.withdraw')" width="narrow" @close="$emit('close')">
    <form v-if="user" id="balance-form" @submit.prevent="handleBalanceSubmit" class="space-y-5">
      <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
        <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100"><span class="text-lg font-medium text-primary-700">{{ user.email.charAt(0).toUpperCase() }}</span></div>
        <div class="flex-1"><p class="font-medium text-gray-900 dark:text-gray-100">{{ user.email }}</p><p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.users.currentBalance') }}: ${{ formatBalance(Number(summary?.total_quota_balance_usd ?? user.balance)) }}</p><p v-if="summary" class="text-xs text-gray-400">¥{{ summary.cash_balance_cny }} 现金 · ${{ summary.paid_quota_balance_usd }} 付费 · ${{ summary.gift_quota_balance_usd }} 赠送</p></div>
      </div>
      <div>
        <label class="input-label">{{ operation === 'add' ? t('admin.users.depositAmount') : t('admin.users.withdrawAmount') }}</label>
        <div class="relative flex gap-2">
          <div class="relative flex-1"><div class="absolute left-3 top-1/2 -translate-y-1/2 font-medium text-gray-500">¥</div><input v-model.number="form.amount" type="number" step="any" min="0" required class="input pl-8" /></div>
          <button v-if="operation === 'subtract'" type="button" @click="fillAllBalance" class="btn btn-secondary whitespace-nowrap">{{ t('admin.users.withdrawAll') }}</button>
        </div>
      </div>
      <div v-if="operation === 'add'">
        <label class="input-label">{{ t('admin.users.giftQuota') }}</label>
        <div class="relative"><div class="absolute left-3 top-1/2 -translate-y-1/2 font-medium text-gray-500">$</div><input v-model.number="form.giftQuota" type="number" step="any" min="0" class="input pl-8" /></div>
      </div>
      <div v-else class="rounded-xl border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800">{{ t('admin.users.refundGiftClears') }}</div>
      <div><label class="input-label">{{ t('admin.users.notes') }}</label><textarea v-model="form.notes" rows="3" class="input"></textarea></div>
      <div v-if="form.amount > 0" class="rounded-xl border border-blue-200 bg-blue-50 p-4 dark:border-blue-800 dark:bg-blue-950"><div class="flex items-center justify-between text-sm"><span class="text-gray-700 dark:text-gray-300">{{ t('admin.users.newBalance') }}:</span><span class="font-bold text-gray-900 dark:text-gray-100">${{ formatBalance(calculateNewBalance()) }}</span></div></div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="$emit('close')" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="balance-form" :disabled="submitting || !form.amount" class="btn" :class="operation === 'add' ? 'bg-emerald-600 text-white' : 'btn-danger'">{{ submitting ? t('common.saving') : t('common.confirm') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI, type QuotaSummary } from '@/api/admin'
import type { AdminUser } from '@/types'
import { extractApiErrorMessage } from '@/utils/apiError'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ show: boolean, user: AdminUser | null, operation: 'add' | 'subtract' }>()
const emit = defineEmits(['close', 'success']); const { t } = useI18n(); const appStore = useAppStore()

const submitting = ref(false); const summary = ref<QuotaSummary | null>(null); const form = reactive({ amount: 0, giftQuota: 0, notes: '' })
watch(() => props.show, (v) => { if(v) { form.amount = 0; form.giftQuota = 0; form.notes = ''; summary.value = null; if (props.user) void loadSummary(props.user.id) } })
const loadSummary = async (id: number) => { try { summary.value = await adminAPI.users.getUserQuotaSummary(id) } catch { summary.value = null } }

// 格式化余额：显示完整精度，去除尾部多余的0
const formatBalance = (value: number) => {
  if (value === 0) return '0.00'
  // 最多保留8位小数，去除尾部的0
  const formatted = value.toFixed(8).replace(/\.?0+$/, '')
  // 确保至少有2位小数
  const parts = formatted.split('.')
  if (parts.length === 1) return formatted + '.00'
  if (parts[1].length === 1) return formatted + '0'
  return formatted
}

// 填入全部余额
const fillAllBalance = () => {
  if (props.user) {
    form.amount = Math.min(Number(summary.value?.cash_balance_cny ?? props.user.balance), Number(summary.value?.paid_quota_balance_usd ?? props.user.balance))
  }
}

const calculateNewBalance = () => {
  if (!props.user) return 0
  const paid = Number(summary.value?.paid_quota_balance_usd ?? props.user.balance)
  const gift = Number(summary.value?.gift_quota_balance_usd ?? 0)
  const result = props.operation === 'add' ? paid + gift + form.amount + form.giftQuota : paid + gift - form.amount
  // 避免浮点数精度问题导致的 -0.00 显示
  return Math.abs(result) < 1e-10 ? 0 : result
}
const handleBalanceSubmit = async () => {
  if (!props.user) return
  if (!form.amount || form.amount <= 0) {
    appStore.showError(t('admin.users.amountRequired'))
    return
  }
  // 退款时验证金额不超过实际余额
  if (props.operation === 'subtract' && form.amount > Math.min(Number(summary.value?.cash_balance_cny ?? props.user.balance), Number(summary.value?.paid_quota_balance_usd ?? props.user.balance))) {
    appStore.showError(t('admin.users.insufficientBalance'))
    return
  }
  submitting.value = true
  try {
    await adminAPI.users.createQuotaLedgerEntry(props.user.id, { record_type: props.operation === 'add' ? 'recharge' : 'refund', amount_cny: form.amount, gift_quota_usd: props.operation === 'add' ? form.giftQuota : 0, note: form.notes })
    appStore.showSuccess(t('common.success')); emit('success'); emit('close')
  } catch (e: any) {
    console.error('Failed to update balance:', e)
    appStore.showError(extractApiErrorMessage(e, t('common.error')))
  } finally { submitting.value = false }
}
</script>
