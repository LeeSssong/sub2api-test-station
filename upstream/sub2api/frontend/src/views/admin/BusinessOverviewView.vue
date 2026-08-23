<template>
  <AppLayout>
    <main class="mx-auto w-full max-w-[1440px] space-y-6 overflow-x-hidden p-4 sm:p-6" data-test="business-overview-page">
      <header class="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 class="text-2xl font-semibold">经营总览</h1>
          <p class="text-sm text-gray-500">按用户实际扣费与上游实际成本查看站内经营结果。</p>
          <p class="mt-1 text-xs text-gray-500">经营口径 CNY / ¥ · 站内额度 Q（内部记账额度，不是美元）</p>
        </div>
        <button class="btn btn-secondary" data-test="business-refresh" :disabled="loading" @click="loadReport">刷新</button>
      </header>

      <section class="flex flex-wrap items-end gap-3" data-test="business-range-controls">
        <label class="text-sm">时间范围<select v-model="range" class="input ml-2" data-test="business-range" @change="loadReport"><option value="today">今天</option><option value="7d">近 7 天</option><option value="30d">近 30 天</option><option value="month">本月</option><option value="previous_month">上月</option><option value="custom">自定义</option></select></label>
        <template v-if="range === 'custom'">
          <label class="text-sm">开始<input v-model="startDate" class="input ml-2" type="date" data-test="business-start-date" @change="loadReport"></label>
          <label class="text-sm">结束<input v-model="endDate" class="input ml-2" type="date" data-test="business-end-date" @change="loadReport"></label>
        </template>
        <span v-if="report" class="text-xs text-gray-500">{{ report.start_date }} 至 {{ report.end_date }}（{{ report.timezone }}）</span>
      </section>

      <div v-if="error" class="rounded border border-red-300 bg-red-50 p-3 text-sm text-red-700" data-test="business-error" role="alert">{{ error }}</div>
      <div v-if="loading && !report" class="text-sm text-gray-500" data-test="business-loading">加载中</div>

      <template v-if="report">
        <p v-if="isPending" class="rounded border border-amber-300 bg-amber-50 p-3 text-sm text-amber-800" data-test="business-pending">当前账本或历史额度拆分尚未完整，收入和毛利显示“口径待确认”；上游成本仍按原生用量记录统计。</p>

        <section data-test="business-results">
          <div class="mb-3 flex items-center justify-between"><h2 class="text-lg font-semibold">经营结果</h2><span class="text-xs text-gray-500">{{ report.currency }} · 用户实际扣费</span></div>
          <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <article v-for="card in resultCards" :key="card.key" class="min-w-0 border-t border-gray-200 pt-3 dark:border-gray-700" :data-test="`business-card-${card.key}`">
              <div class="text-xs text-gray-500">{{ card.label }}</div>
              <div class="mt-1 break-words text-xl font-semibold" :class="card.tone">{{ card.value }}</div>
            </article>
          </div>
        </section>

        <section data-test="business-cash-balance">
          <div class="mb-3 flex flex-wrap items-center justify-between gap-2"><h2 class="text-lg font-semibold">充值与余额</h2><span class="text-xs text-gray-500">资金状态 · 不计入毛利</span></div>
          <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
            <article v-for="card in balanceCards" :key="card.key" class="min-w-0 border-t border-gray-200 pt-3 dark:border-gray-700" :data-test="`business-balance-${card.key}`"><div class="text-xs text-gray-500">{{ card.label }}</div><div class="mt-1 break-words text-base font-semibold">{{ card.value }}</div></article>
          </div>
          <p class="mt-3 text-xs text-gray-500" data-test="business-reconciliation">余额核对：{{ report.cash_and_balance.balance_reconciliation.status }}<template v-if="report.cash_and_balance.balance_reconciliation.difference_cny != null"> · 差额 ¥{{ formatMoney(report.cash_and_balance.balance_reconciliation.difference_cny) }}</template></p>
        </section>

        <section data-test="business-trend">
          <div class="mb-3 flex items-center justify-between"><h2 class="text-lg font-semibold">充值与消耗趋势</h2><span class="text-xs text-gray-500">人民币 · 按北京时间自然日</span></div>
          <div v-if="report.trend.length" class="space-y-3">
            <div class="h-72 rounded border border-gray-200 p-3 dark:border-gray-700" data-test="business-trend-chart-container"><Line :data="trendChartData" :options="trendChartOptions" /></div>
            <div class="overflow-x-auto rounded border border-gray-200 p-3 dark:border-gray-700"><div class="min-w-[620px]" data-test="business-trend-rows"><div v-for="row in report.trend" :key="row.date" class="grid grid-cols-4 gap-3 border-b border-gray-100 py-2 text-sm last:border-b-0 dark:border-gray-800"><span>{{ row.date }}</span><span>充值 ¥{{ formatMoney(row.cash_recharge_cny) }}</span><span>消耗 ¥{{ formatMoney(row.paid_consumption_cny) }}</span><span>净充值 ¥{{ formatMoney(row.net_settlement_cny) }}</span></div></div></div>
          </div>
          <p v-else class="text-sm text-gray-500">暂无趋势数据</p>
        </section>

        <section data-test="business-groups">
          <div class="mb-3 flex items-center justify-between"><h2 class="text-lg font-semibold">分组毛利分析</h2><span class="text-xs text-gray-500">仅站内分组，不展示上游明细</span></div>
          <div class="overflow-x-auto rounded border border-gray-200 dark:border-gray-700"><table class="min-w-[960px] w-full text-left text-sm"><thead class="bg-gray-50 text-xs text-gray-500 dark:bg-gray-800"><tr><th class="p-3">分组</th><th class="p-3">模型数</th><th class="p-3">调用次数</th><th class="p-3">站内收入</th><th class="p-3">上游成本</th><th class="p-3">实际毛利</th><th class="p-3">实际毛利率</th><th class="p-3">预设倍率</th></tr></thead><tbody><tr v-for="group in report.groups" :key="`${group.group_id ?? 'unassigned'}-${group.group_name}`" class="border-t border-gray-100 dark:border-gray-800" :data-test="`business-group-${group.group_id ?? 'unassigned'}`"><td class="p-3 font-medium">{{ group.group_name }}</td><td class="p-3">{{ group.model_count }}</td><td class="p-3">{{ group.request_count }}</td><td class="p-3">{{ formatMoneyOrPending(group.revenue_cny) }}</td><td class="p-3">{{ formatMoneyOrPending(group.upstream_cost_cny) }}</td><td class="p-3" :class="group.gross_profit_cny != null && group.gross_profit_cny < 0 ? 'text-red-600' : 'text-green-600'">{{ formatMoneyOrPending(group.gross_profit_cny) }}</td><td class="p-3">{{ formatPercent(group.gross_margin) }}</td><td class="p-3">{{ group.preset_status === 'unavailable' ? '待配置' : formatPercent(group.preset_margin) }}</td></tr><tr v-if="!report.groups.length"><td colspan="8" class="p-4 text-center text-sm text-gray-500">暂无分组数据</td></tr></tbody></table></div>
        </section>
      </template>
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { adminAPI } from '@/api/admin'
import type { BusinessOverviewRange, BusinessOverviewReport } from '@/api/admin/businessOverview'
import {
  Chart as ChartJS,
  CategoryScale,
  Filler,
  Legend,
  LinearScale,
  LineElement,
  PointElement,
  Tooltip,
} from 'chart.js'
import { Line } from 'vue-chartjs'

ChartJS.register(CategoryScale, Filler, Legend, LinearScale, LineElement, PointElement, Tooltip)

const range = ref<BusinessOverviewRange>('today')
const startDate = ref('')
const endDate = ref('')
const report = ref<BusinessOverviewReport | null>(null)
const loading = ref(false)
const error = ref('')
const isPending = computed(() => report.value?.revenue_status !== 'confirmed')
const formatMoney = (value: number) => value.toLocaleString('zh-CN', { minimumFractionDigits: 2, maximumFractionDigits: 2 })
const formatMoneyOrPending = (value: number | null) => value == null ? '口径待确认' : `¥${formatMoney(value)}`
const formatPercent = (value: number | null) => value == null ? '—' : `${(value * 100).toFixed(2)}%`
const trendChartData = computed(() => ({
  labels: report.value?.trend.map((row) => row.date) ?? [],
  datasets: [
    {
      label: '现金充值',
      data: report.value?.trend.map((row) => row.cash_recharge_cny) ?? [],
      borderColor: '#0f766e',
      backgroundColor: 'rgba(15, 118, 110, 0.12)',
      tension: 0.3,
      fill: true,
    },
    {
      label: '付费消耗',
      data: report.value?.trend.map((row) => row.paid_consumption_cny) ?? [],
      borderColor: '#c2410c',
      backgroundColor: 'rgba(194, 65, 12, 0.08)',
      tension: 0.3,
      fill: false,
    },
  ],
}))
const trendChartOptions = computed(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { intersect: false, mode: 'index' as const },
  plugins: {
    legend: { position: 'top' as const },
    tooltip: { callbacks: { label: (context: { dataset: { label?: string }; raw: unknown }) => `${context.dataset.label}: ¥${formatMoney(Number(context.raw) || 0)}` } },
  },
  scales: {
    y: { beginAtZero: true, ticks: { callback: (value: string | number) => `¥${formatMoney(Number(value) || 0)}` } },
  },
}))
const resultCards = computed(() => {
  const summary = report.value?.summary
  return [
    { key: 'revenue', label: '站内收入', value: formatMoneyOrPending(summary?.revenue_cny ?? null), tone: 'text-gray-900 dark:text-white' },
    { key: 'cost', label: '上游成本', value: formatMoneyOrPending(summary?.upstream_cost_cny ?? null), tone: 'text-gray-900 dark:text-white' },
    { key: 'profit', label: '毛利', value: formatMoneyOrPending(summary?.gross_profit_cny ?? null), tone: (summary?.gross_profit_cny ?? 0) < 0 ? 'text-red-600' : 'text-green-600' },
    { key: 'margin', label: '毛利率', value: formatPercent(summary?.gross_margin ?? null), tone: (summary?.gross_margin ?? 0) < 0 ? 'text-red-600' : 'text-green-600' },
  ]
})
const balanceCards = computed(() => {
  const balance = report.value?.cash_and_balance
  return [
    { key: 'recharge', label: `${report.value?.start_date ?? ''} - ${report.value?.end_date ?? ''} 现金充值`, value: formatMoneyOrPending(balance?.cash_recharge_cny ?? null) },
    { key: 'opening', label: `${report.value?.start_date ?? ''} 未消耗余额`, value: formatMoneyOrPending(balance?.opening_paid_balance_cny ?? null) },
    { key: 'consumption', label: `${report.value?.start_date ?? ''} - ${report.value?.end_date ?? ''} 实际消耗`, value: formatMoneyOrPending(balance?.paid_consumption_cny ?? null) },
    { key: 'closing', label: `${report.value?.end_date ?? ''} 未消耗余额`, value: formatMoneyOrPending(balance?.closing_paid_balance_cny ?? null) },
  ]
})
async function loadReport() {
  loading.value = true
  error.value = ''
  try {
    report.value = await adminAPI.businessOverview.getReport({ range: range.value, ...(range.value === 'custom' ? { start_date: startDate.value, end_date: endDate.value } : {}), timezone: 'Asia/Shanghai' })
  } catch (err) {
    error.value = err instanceof Error ? err.message : '经营总览加载失败'
  } finally {
    loading.value = false
  }
}
onMounted(loadReport)
</script>
