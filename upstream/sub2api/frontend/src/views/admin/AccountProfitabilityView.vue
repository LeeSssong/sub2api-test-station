<template>
  <AppLayout>
    <main class="mx-auto max-w-[1400px] space-y-5 p-5" data-test="account-profitability-page">
      <header class="flex items-center justify-between gap-3"><div><h1 class="text-2xl font-semibold">{{ t('admin.accountProfitability.title') }}</h1><p class="text-sm text-gray-500">{{ t('admin.accountProfitability.description') }}</p></div><button class="btn btn-secondary" data-test="financial-refresh" @click="load">{{ t('common.refresh') }}</button></header>
      <nav class="flex gap-2" aria-label="range"><button v-for="item in ranges" :key="item" :data-test="`range-${item}`" class="btn btn-secondary" :class="{ 'bg-primary-600 text-white': activeRange === item }" @click="selectRange(item)">{{ t(`admin.accountProfitability.ranges.${item}`) }}</button></nav>
      <div class="text-xs text-gray-500" data-test="financial-generated-at">{{ generatedAt }}</div>
      <section class="grid grid-cols-2 gap-3 lg:grid-cols-6"><article v-for="card in cards" :key="card.key" class="card p-4" :data-test="`summary-${card.key}`"><div class="text-xs text-gray-500">{{ card.label }}</div><div class="mt-2 text-xl font-semibold">{{ card.value }}</div></article></section>
      <section class="table-container" data-test="account-financial-table"><table class="table"><thead><tr><th>{{ t('admin.accountProfitability.columns.account') }}</th><th>{{ t('admin.accountProfitability.columns.revenue') }}</th><th>{{ t('admin.accountProfitability.columns.expense') }}</th><th>{{ t('admin.accountProfitability.columns.profit') }}</th><th>{{ t('admin.accountProfitability.columns.margin') }}</th><th>{{ t('admin.accountProfitability.columns.exceptions') }}</th><th>{{ t('admin.accountProfitability.columns.actions') }}</th></tr></thead><tbody><tr v-for="row in report.accounts" :key="row.id" :data-test="`account-financial-${row.id}`"><td>{{ row.name }}</td><td>{{ money(row.amounts.revenue) }}</td><td>{{ money(row.amounts.cost) }}</td><td>{{ money(row.amounts.profit) }}</td><td>{{ percent(row.amounts.margin) }}</td><td><button v-if="row.exception_count" :data-test="`account-exceptions-${row.id}`" @click="jump(row.id)">{{ row.exception_count }}</button></td><td><template v-if="activeRange === 'today'"><input :data-test="`account-edit-revenue-${row.id}`" @change="saveOverride(row.id, 'revenue_cny', $event)" /><input :data-test="`account-edit-cost-${row.id}`" @change="saveOverride(row.id, 'cost_cny', $event)" /><input v-if="row.type === 'oauth'" :data-test="`account-edit-oauth-cost-${row.id}`" @change="saveOAuthCost(row.id, $event)" /></template></td></tr></tbody></table></section>
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
const { t } = useI18n(); const router = useRouter(); const activeRange = ref<FinancialRange>('today'); const loading = ref(false); const report = ref<AccountFinancialReport>({ generated_at: '', range: 'today', summary: { revenue: 0, cost: 0, profit: 0, margin: null, exception_count: 0, affected_revenue: 0 }, accounts: [], exception_count: 0, affected_revenue: 0, user_unconsumed_balance_cny: 0 }); let timer: ReturnType<typeof setInterval> | undefined
const ranges: FinancialRange[] = ['today', '24h', '7d', '31d']
const generatedAt = computed(() => report.value.generated_at ? new Date(report.value.generated_at).toLocaleString() : '—')
const money = (v: number) => `¥${Number(v || 0).toFixed(2)}`; const percent = (v: number | null) => v == null ? '—' : `${(v * 100).toFixed(1)}%`
const cards = computed(() => [{ key: 'revenue', label: t('admin.accountProfitability.summary.revenue'), value: money(report.value.summary.revenue) }, { key: 'expense', label: t('admin.accountProfitability.summary.expense'), value: money(report.value.summary.cost) }, { key: 'profit', label: t('admin.accountProfitability.summary.profit'), value: money(report.value.summary.profit) }, { key: 'margin', label: t('admin.accountProfitability.summary.margin'), value: percent(report.value.summary.margin) }, { key: 'exceptions', label: t('admin.accountProfitability.summary.exceptions'), value: String(report.value.summary.exception_count) }, { key: 'unconsumed-balance', label: t('admin.accountProfitability.summary.unconsumedBalance'), value: money(report.value.user_unconsumed_balance_cny) }])
async function load() { loading.value = true; try { report.value = await adminAPI.accountFinancial.getReport({ range: activeRange.value }) } finally { loading.value = false } }
function selectRange(range: FinancialRange) { activeRange.value = range; void load() }
function jump(accountId: number) { router.push({ path: '/admin/usage', query: { tab: 'cost-exceptions', range: activeRange.value, account_id: String(accountId) } }) }
function beijingDate() { return new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date()) }
async function saveOverride(accountId: number, field: 'revenue_cny' | 'cost_cny', event: Event) { if (activeRange.value !== 'today') return; await adminAPI.accountFinancial.setTodayOverride(accountId, { business_date: beijingDate(), [field]: Number((event.target as HTMLInputElement).value) }); await load() }
async function saveOAuthCost(accountId: number, event: Event) { if (activeRange.value !== 'today') return; await adminAPI.accountFinancial.setOAuthCost(accountId, { business_date: beijingDate(), cost_cny: Number((event.target as HTMLInputElement).value) }); await load() }
onMounted(() => { void load(); timer = setInterval(() => void load(), 60_000) }); onUnmounted(() => { if (timer) clearInterval(timer) })
</script>
