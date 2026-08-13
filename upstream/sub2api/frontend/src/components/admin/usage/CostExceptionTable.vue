<template>
  <div class="space-y-3 p-4">
    <div class="flex flex-wrap items-center gap-2">
      <input v-model.trim="search" class="input min-w-52" :placeholder="t('admin.costExceptions.search')" @keyup.enter="reload" />
      <select v-model="evidenceStatus" class="input" @change="reload">
        <option value="">{{ t('admin.costExceptions.allEvidence') }}</option>
        <option value="unavailable">unavailable</option>
        <option value="confirmed_zero">confirmed_zero</option>
      </select>
      <select v-model="reviewStatus" class="input" @change="reload">
        <option value="pending">{{ t('admin.costExceptions.pending') }}</option>
        <option value="reviewed">{{ t('admin.costExceptions.reviewed') }}</option>
        <option value="">{{ t('admin.costExceptions.allReviews') }}</option>
      </select>
      <button class="btn btn-secondary" data-test="review-selected" :disabled="selectedIds.length === 0 || mutating" @click="reviewSelection">{{ t('admin.costExceptions.reviewSelected') }}</button>
      <button class="btn btn-secondary" data-test="review-filtered" :disabled="items.length === 0 || mutating" @click="reviewCurrentFilter">{{ t('admin.costExceptions.reviewFiltered') }}</button>
      <button class="btn btn-secondary" data-test="export-cost-exceptions" :disabled="total === 0 || exporting" @click="exportCurrentFilter">{{ t('usage.exportExcel') }}</button>
    </div>
    <p v-if="reviewSummary" class="text-sm text-gray-600 dark:text-gray-300" role="status">{{ reviewSummary }}</p>
    <div class="overflow-x-auto">
      <table class="min-w-full text-sm">
        <thead><tr class="border-b text-left"><th class="p-2"></th><th class="p-2">ID</th><th class="p-2">{{ t('admin.costExceptions.time') }}</th><th class="p-2">{{ t('admin.costExceptions.account') }}</th><th class="p-2">{{ t('admin.costExceptions.requestId') }}</th><th class="p-2">{{ t('admin.costExceptions.model') }}</th><th class="p-2">{{ t('admin.costExceptions.revenue') }}</th><th class="p-2">{{ t('admin.costExceptions.source') }}</th><th class="p-2">{{ t('admin.costExceptions.evidence') }}</th><th class="p-2">{{ t('admin.costExceptions.reason') }}</th><th class="p-2">{{ t('admin.costExceptions.trace') }}</th><th class="p-2">{{ t('admin.costExceptions.reviewStatus') }}</th><th class="p-2">{{ t('common.actions') }}</th></tr></thead>
        <tbody>
          <tr v-for="item in items" :key="item.usage_log_id" class="border-b border-gray-100 dark:border-dark-700">
            <td class="p-2"><input :data-test="`select-${item.usage_log_id}`" type="checkbox" :value="item.usage_log_id" v-model="selectedIds" /></td>
            <td class="p-2 font-mono">{{ item.usage_log_id }}</td>
            <td class="p-2 whitespace-nowrap">{{ item.created_at }}</td>
            <td class="p-2 font-mono">{{ item.account_id }}</td>
            <td class="p-2 font-mono">{{ item.request_id || '-' }}</td>
            <td class="p-2">{{ item.model }}</td>
            <td class="p-2 font-mono">{{ item.revenue_cny }}</td>
            <td class="p-2">{{ item.source }}</td>
            <td class="p-2">{{ item.evidence_status }}</td>
            <td class="p-2 font-mono">{{ item.reason_code }}</td>
            <td class="p-2 font-mono">{{ formatTrace(item.cost_trace) }}</td>
            <td class="p-2">{{ item.review_status }}</td>
            <td class="p-2"><button class="btn btn-secondary btn-sm" :disabled="item.review_status === 'reviewed' || mutating" @click="reviewItem(item.usage_log_id)">{{ t('admin.costExceptions.reviewOne') }}</button><button class="btn btn-ghost btn-sm ml-1" @click="emit('detail', item.usage_log_id)">{{ t('usage.detail.action') }}</button></td>
          </tr>
        </tbody>
      </table>
    </div>
    <Pagination v-if="total > 0" :page="page" :total="total" :page-size="pageSize" @update:page="setPage" @update:pageSize="setPageSize" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import Pagination from '@/components/common/Pagination.vue'
import { adminUsageAPI, type CostExceptionQueryParams } from '@/api/admin/usage'
import type { AdminUsageCostException, UsageCostTrace } from '@/types'

const props = defineProps<{ filters: Record<string, unknown> }>()
const emit = defineEmits<{ detail: [id: number]; reviewed: [] }>()
const { t } = useI18n()
const items = ref<AdminUsageCostException[]>([])
const total = ref(0); const page = ref(1); const pageSize = ref(20)
const search = ref(''); const evidenceStatus = ref(''); const reviewStatus = ref('')
const selectedIds = ref<number[]>([]); const mutating = ref(false); const exporting = ref(false); const reviewSummary = ref('')

const query = computed<CostExceptionQueryParams>(() => ({
  page: page.value, page_size: pageSize.value,
  account_id: typeof props.filters.account_id === 'number' ? props.filters.account_id : undefined,
  start_time: typeof props.filters.start_time === 'string' ? props.filters.start_time : undefined,
  end_time: typeof props.filters.end_time === 'string' ? props.filters.end_time : undefined,
  search: search.value || undefined,
  evidence_status: evidenceStatus.value || (typeof props.filters.evidence_status === 'string' ? props.filters.evidence_status : undefined),
  review_status: reviewStatus.value || (typeof props.filters.review_status === 'string' ? props.filters.review_status : undefined),
}))
const reviewFilter = () => { const { page: _p, page_size: _s, ...filter } = query.value; return filter }

const reload = async () => { const res = await adminUsageAPI.listCostExceptions(query.value); items.value = res.items; total.value = res.total; page.value = res.page; pageSize.value = res.page_size; selectedIds.value = [] }
const reviewItem = async (id: number) => { mutating.value = true; try { await adminUsageAPI.reviewOne(id, {}); await reload(); emit('reviewed') } finally { mutating.value = false } }
const reviewSelection = async () => { mutating.value = true; try { await adminUsageAPI.reviewSelected({ usage_log_ids: selectedIds.value }); await reload(); emit('reviewed') } finally { mutating.value = false } }
const reviewCurrentFilter = async () => { mutating.value = true; try { const res = await adminUsageAPI.reviewFiltered({ filter: reviewFilter(), max_usage_log_id: 0 }); reviewSummary.value = `${t('admin.costExceptions.cutoff')}: ${res.cutoff}; ${t('admin.costExceptions.matched')}: ${res.matched}; ${t('admin.costExceptions.updated')}: ${res.updated}; ${t('admin.costExceptions.skipped')}: ${res.skipped}`; await reload(); emit('reviewed') } finally { mutating.value = false } }
const csvCell = (value: string | number | null | undefined) => `"${String(value ?? '').replace(/"/g, '""')}"`
const exportCurrentFilter = async () => {
  exporting.value = true
  try {
    const rows: Array<Array<string | number>> = [[
      'usage_log_id', 'account_id', 'created_at', 'request_id', 'model', 'revenue_cny',
      'source', 'evidence_status', 'reason_code', 'review_status', 'cost_trace',
    ]]
    let exportPage = 1
    while (true) {
      const response = await adminUsageAPI.listCostExceptions({ ...reviewFilter(), page: exportPage, page_size: 100 })
      for (const item of response.items) {
        rows.push([
          item.usage_log_id, item.account_id, item.created_at, item.request_id, item.model, item.revenue_cny,
          item.source, item.evidence_status, item.reason_code, item.review_status, formatTrace(item.cost_trace),
        ])
      }
      if (response.items.length < 100 || exportPage * response.page_size >= response.total) break
      exportPage += 1
    }
    saveAs(new Blob([rows.map((row) => row.map(csvCell).join(',')).join('\n')], { type: 'text/csv;charset=utf-8' }), 'cost-exceptions.csv')
  } finally { exporting.value = false }
}
const setPage = (value: number) => { page.value = value; void reload() }
const setPageSize = (value: number) => { pageSize.value = value; page.value = 1; void reload() }
const formatTrace = (trace: UsageCostTrace) => [trace.sub_actual_cost, trace.new_api_quota, trace.new_api_quota_per_unit, trace.normalized_cost_cny].map(v => v == null ? '-' : v).join(' / ')
watch(() => props.filters, () => { page.value = 1; void reload() }, { deep: true })
onMounted(reload)
defineExpose({ reload })
</script>
