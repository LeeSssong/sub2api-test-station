<template>
  <AppLayout>
    <main class="mx-auto max-w-[1400px] space-y-5 p-5" data-test="account-profitability-page">
      <header class="flex items-center justify-between gap-3"><div><h1 class="text-2xl font-semibold">{{ t('admin.accountProfitability.title') }}</h1><p class="text-sm text-gray-500">{{ t('admin.accountProfitability.description') }}</p></div><button class="btn btn-secondary" data-test="financial-refresh" @click="load">{{ t('common.refresh') }}</button></header>
      <nav class="flex gap-2" aria-label="range"><button v-for="item in ranges" :key="item" :data-test="`range-${item}`" class="btn btn-secondary" :class="{ 'bg-primary-600 text-white': activeRange === item }" @click="selectRange(item)">{{ t(`admin.accountProfitability.ranges.${item}`) }}</button></nav>
      <div class="text-xs text-gray-500" data-test="financial-generated-at">{{ generatedAt }}</div>
      <div v-if="loadError" class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-950/30 dark:text-red-300" data-test="financial-load-error" role="alert">
        <span>{{ loadError }}</span>
        <button class="btn btn-secondary ml-3" data-test="financial-retry" @click="load">{{ t('admin.accountProfitability.retry') }}</button>
      </div>
      <section class="grid grid-cols-2 gap-3 lg:grid-cols-6"><article v-for="card in cards" :key="card.key" class="card p-4" :data-test="`summary-${card.key}`"><div class="text-xs text-gray-500">{{ card.label }}</div><div class="mt-2 text-xl font-semibold">{{ card.value }}</div></article></section>
      <nav class="flex max-w-full gap-2 overflow-x-auto pb-1" :aria-label="t('admin.accountProfitability.scope.label')">
        <button class="btn btn-secondary shrink-0" data-test="scope-all" :class="{ 'bg-primary-600 text-white': activeScope.kind === 'all' }" @click="activeScope = { kind: 'all' }">{{ t('admin.accountProfitability.scope.all') }}</button>
        <button v-for="group in report.groups" :key="`${group.unassigned ? 'unassigned' : 'group'}-${group.id}`" class="btn btn-secondary shrink-0" :data-test="`scope-group-${group.id}`" :class="{ 'bg-primary-600 text-white': isSelectedGroup(group.id, group.unassigned) }" @click="activeScope = { kind: 'group', id: group.id, unassigned: group.unassigned }">{{ group.unassigned ? t('admin.accountProfitability.scope.unassigned') : group.name }}</button>
      </nav>
      <section v-if="selectedGroup" class="space-y-3" :data-test="`group-summary-${selectedGroup.id}`">
        <div class="flex flex-wrap items-center justify-between gap-2"><h2 class="text-base font-semibold"><span class="text-gray-500">{{ t('admin.accountProfitability.scope.groupSummary') }}：</span>{{ selectedGroup.unassigned ? t('admin.accountProfitability.scope.unassigned') : selectedGroup.name }}</h2><span class="text-xs text-gray-500">{{ t('admin.accountProfitability.scope.accountCount', { count: selectedAccounts.length }) }}</span></div>
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-5"><article v-for="item in scopedCards" :key="item.key" class="card p-3"><div class="text-xs text-gray-500">{{ item.label }}</div><div class="mt-1 text-base font-semibold">{{ item.value }}</div></article></div>
        <p v-if="selectedGroup.has_unallocated_adjustments" class="rounded border border-amber-300 bg-amber-50 p-3 text-sm text-amber-900 dark:border-amber-700 dark:bg-amber-950/30 dark:text-amber-200" data-test="unallocated-adjustments-notice">{{ t('admin.accountProfitability.scope.unallocatedAdjustments') }}</p>
        <p v-if="!selectedGroup.complete" class="text-sm text-amber-700 dark:text-amber-300" data-test="incomplete-group-notice">{{ t('admin.accountProfitability.scope.incomplete') }}</p>
      </section>
      <section class="table-container overflow-x-auto" data-test="account-financial-table"><table class="table"><thead><tr><th>{{ t('admin.accountProfitability.columns.account') }}</th><th>{{ t('admin.accountProfitability.columns.revenue') }}</th><th>{{ t('admin.accountProfitability.columns.expense') }}</th><th>{{ t('admin.accountProfitability.columns.profit') }}</th><th>{{ t('admin.accountProfitability.columns.margin') }}</th><th>{{ t('admin.accountProfitability.columns.exceptions') }}</th><th>{{ t('admin.accountProfitability.columns.actions') }}</th></tr></thead><tbody><tr v-for="row in selectedAccounts" :key="row.id" :data-test="`account-financial-${row.id}`"><td>{{ row.name }}</td><td>{{ money(row.amounts.revenue) }}</td><td>{{ money(row.amounts.cost) }}</td><td>{{ money(row.amounts.profit) }}</td><td>{{ percent(row.amounts.margin) }}</td><td><button v-if="row.exception_count" :data-test="`account-exceptions-${row.id}`" @click="jump(row.id)">{{ row.exception_count }}</button></td><td><template v-if="activeRange === 'today' && activeScope.kind === 'all'"><input :data-test="`account-edit-revenue-${row.id}`" @change="saveOverride(row.id, 'revenue_cny', $event)" /><input :data-test="`account-edit-cost-${row.id}`" @change="saveOverride(row.id, 'cost_cny', $event)" /><input v-if="row.type === 'oauth'" :data-test="`account-edit-oauth-cost-${row.id}`" @change="saveOAuthCost(row.id, $event)" /></template></td></tr></tbody></table></section>
    </main>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { adminAPI } from '@/api/admin'
import type { AccountFinancialReport, FinancialRange } from '@/api/admin/accountFinancial'
import AppLayout from '@/components/layout/AppLayout.vue'
type FinancialScope = { kind: 'all' } | { kind: 'group'; id: number; unassigned: boolean }
const { t } = useI18n(); const router = useRouter(); const activeRange = ref<FinancialRange>('today'); const activeScope = ref<FinancialScope>({ kind: 'all' }); const loading = ref(false); const loadError = ref(''); const report = ref<AccountFinancialReport>({ generated_at: '', range: 'today', summary: { revenue: 0, cost: 0, profit: 0, margin: null, exception_count: 0, affected_revenue: 0 }, accounts: [], groups: [], exception_count: 0, affected_revenue: 0, user_unconsumed_balance_cny: 0 }); let timer: ReturnType<typeof setInterval> | undefined
const ranges: FinancialRange[] = ['today', '24h', '7d', '31d']
const generatedAt = computed(() => report.value.generated_at ? new Date(report.value.generated_at).toLocaleString() : '—')
const money = (v: number) => `¥${Number(v || 0).toFixed(2)}`; const percent = (v: number | null) => v == null ? '—' : `${(v * 100).toFixed(1)}%`
const cards = computed(() => [{ key: 'revenue', label: t('admin.accountProfitability.summary.revenue'), value: money(report.value.summary.revenue) }, { key: 'expense', label: t('admin.accountProfitability.summary.expense'), value: money(report.value.summary.cost) }, { key: 'profit', label: t('admin.accountProfitability.summary.profit'), value: money(report.value.summary.profit) }, { key: 'margin', label: t('admin.accountProfitability.summary.margin'), value: percent(report.value.summary.margin) }, { key: 'exceptions', label: t('admin.accountProfitability.summary.exceptions'), value: String(report.value.summary.exception_count) }, { key: 'unconsumed-balance', label: t('admin.accountProfitability.summary.unconsumedBalance'), value: money(report.value.user_unconsumed_balance_cny) }])
const selectedGroup = computed(() => {
  const scope = activeScope.value
  return scope.kind === 'group' ? report.value.groups.find((group) => group.id === scope.id && group.unassigned === scope.unassigned) : undefined
})
const selectedAccounts = computed(() => selectedGroup.value?.accounts ?? report.value.accounts)
const selectedAmounts = computed(() => selectedGroup.value?.amounts ?? report.value.summary)
const scopedCards = computed(() => [{ key: 'revenue', label: t('admin.accountProfitability.summary.revenue'), value: money(selectedAmounts.value.revenue) }, { key: 'expense', label: t('admin.accountProfitability.summary.expense'), value: money(selectedAmounts.value.cost) }, { key: 'profit', label: t('admin.accountProfitability.summary.profit'), value: money(selectedAmounts.value.profit) }, { key: 'margin', label: t('admin.accountProfitability.summary.margin'), value: percent(selectedAmounts.value.margin) }, { key: 'exceptions', label: t('admin.accountProfitability.summary.exceptions'), value: String(selectedAmounts.value.exception_count) }])
const isSelectedGroup = (id: number, unassigned: boolean) => activeScope.value.kind === 'group' && activeScope.value.id === id && activeScope.value.unassigned === unassigned
async function load() { loading.value = true; loadError.value = ''; try { report.value = await adminAPI.accountFinancial.getReport({ range: activeRange.value }); if (activeScope.value.kind === 'group' && !selectedGroup.value) activeScope.value = { kind: 'all' } } catch { loadError.value = t('admin.accountProfitability.loadError') } finally { loading.value = false } }
function selectRange(range: FinancialRange) { activeRange.value = range; void load() }
function jump(accountId: number) { router.push({ path: '/admin/usage', query: { tab: 'cost-exceptions', review: 'pending', range: activeRange.value, account_id: String(accountId) } }) }
function beijingDate() { return new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date()) }
async function saveOverride(accountId: number, field: 'revenue_cny' | 'cost_cny', event: Event) { if (activeRange.value !== 'today') return; await adminAPI.accountFinancial.setTodayOverride(accountId, { business_date: beijingDate(), [field]: Number((event.target as HTMLInputElement).value) }); await load() }
async function saveOAuthCost(accountId: number, event: Event) { if (activeRange.value !== 'today') return; await adminAPI.accountFinancial.setOAuthCost(accountId, { business_date: beijingDate(), cost_cny: Number((event.target as HTMLInputElement).value) }); await load() }
onMounted(() => { void load(); timer = setInterval(() => void load(), 60_000) }); onUnmounted(() => { if (timer) clearInterval(timer) })
</script>
