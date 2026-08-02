<template>
  <Teleport to="body">
    <Transition name="drawer">
      <aside v-if="show" class="fixed inset-y-0 right-0 z-50 flex w-full max-w-2xl flex-col border-l border-slate-200 bg-white shadow-2xl dark:border-slate-800 dark:bg-slate-950" role="dialog" aria-modal="true" aria-label="账务历史">
        <header class="flex items-center justify-between border-b border-slate-200 px-5 py-4 dark:border-slate-800">
          <div>
            <h2 class="text-base font-semibold text-slate-900 dark:text-white">账务历史</h2>
            <p class="mt-0.5 text-xs text-slate-500 dark:text-slate-400">近 30 天 · 按日汇总 · 真实上游扣费</p>
          </div>
          <button type="button" class="icon-button" aria-label="关闭" @click="emit('close')"><Icon name="x" size="sm" /></button>
        </header>
        <div class="flex-1 overflow-y-auto p-5">
          <div v-if="loading" class="py-10 text-center text-sm text-slate-500">{{ t('common.loading') }}</div>
          <div v-else-if="error" class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/20 dark:text-red-300">{{ error }}</div>
          <div v-else-if="items.length === 0" class="py-10 text-center text-sm text-slate-500">暂无账务记录</div>
          <div v-else class="space-y-2">
            <div v-for="item in items" :key="item.day" class="rounded-lg border border-slate-200 p-3 dark:border-slate-800 dark:bg-slate-900/50">
              <div class="flex items-center justify-between text-sm font-medium text-slate-900 dark:text-white"><span>{{ item.day }}</span><span class="font-mono">{{ item.currency }}</span></div>
              <div class="mt-2 grid grid-cols-3 gap-2 text-xs">
                <LedgerCell label="成本" :value="formatMoney(item.upstream_cost, item.currency)" />
                <LedgerCell label="计费" :value="formatMoney(item.user_charge, item.currency)" />
                <LedgerCell label="账号利润" :value="formatMoney(item.paper_profit, item.currency)" />
              </div>
            </div>
          </div>
        </div>
      </aside>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { history as loadLedgerHistory } from '@/api/admin/reconciliation'
import type { OperationsDailyRow, OperationsScopeParams } from '@/api/admin/reconciliation'

const props = defineProps<{ show: boolean; scope?: OperationsScopeParams }>()
const emit = defineEmits<{ (event: 'close'): void }>()
const { t } = useI18n()
const loading = ref(false)
const error = ref<string | null>(null)
const items = ref<OperationsDailyRow[]>([])
const scopedParams = computed(() => {
  const end = new Date()
  const start = new Date(end.getTime() - 30 * 24 * 60 * 60 * 1000)
  return { ...props.scope, start: start.toISOString(), end: end.toISOString() }
})
async function load() {
  loading.value = true
  error.value = null
  try {
    items.value = (await loadLedgerHistory(scopedParams.value)).items
  } catch (err) {
    error.value = err instanceof Error ? err.message : '账务历史加载失败'
  } finally {
    loading.value = false
  }
}
watch(() => props.show, (show) => { if (show) void load() }, { immediate: true })
function formatMoney(value: string | number | null | undefined, currency?: string | null): string {
  const amount = Number(value)
  if (!Number.isFinite(amount)) return '—'
  try {
    if (currency) {
      return new Intl.NumberFormat(undefined, { style: 'currency', currency, minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(amount)
    }
    return new Intl.NumberFormat(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(amount)
  } catch {
    return amount.toFixed(2)
  }
}
const LedgerCell = defineComponent({
  props: { label: { type: String, required: true }, value: { type: String, required: true } },
  setup(cellProps) {
    return () => h('div', { class: 'rounded bg-slate-50 p-2 dark:bg-slate-950' }, [
      h('div', { class: 'text-[11px] text-slate-500' }, cellProps.label),
      h('div', { class: 'mt-1 font-mono font-semibold text-slate-900 dark:text-white' }, cellProps.value),
    ])
  },
})
</script>
