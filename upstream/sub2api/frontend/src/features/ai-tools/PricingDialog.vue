<template>
  <BaseDialog brand-theme :show="!!tool" :title="`${tool?.label || ''} 价格表`" width="full" panel-class="xq-pricing-dialog xq-context-pricing-dialog" :close-on-click-outside="true" @close="emit('close')">
    <div class="pricing-content">
      <section class="official-pricing-section" aria-labelledby="official-pricing-title">
        <div class="pricing-heading official-pricing-heading">
          <h3 id="official-pricing-title">{{ isOpenAiTool ? 'OpenAI 官方参考价' : '模型官方参考价' }}</h3>
          <div v-if="isOpenAiTool" class="pricing-filter">
            <Select v-model="selectedCategory" :options="categoryOptions" :searchable="false" brand aria-label="模型类型" />
          </div>
        </div>
        <div id="official-pricing-panel" aria-labelledby="official-pricing-title">
          <div class="pricing-tier-label">Standard · 美元 / 100 万 Token</div>
          <p v-if="loading && !hasLoaded" role="status">正在读取分组、模型与倍率…</p>
          <div v-else-if="error" class="pricing-error" role="alert">分组、模型或倍率读取失败，请重试。<button type="button" class="btn btn-secondary" @click="loadPricing">重试</button></div>
          <div v-else-if="priceRows.length" class="table-scroll model-pricing-table context-pricing-table" tabindex="0" aria-label="模型价格，美元 / 100 万 Token">
            <table :style="{ minWidth: `${180 + contextGroups.length * contextPriceColumns.length * 124}px` }">
              <colgroup><col class="model-column" /></colgroup>
              <colgroup v-for="group in contextGroups" :key="group.index" :span="contextPriceColumns.length" />
              <thead>
                <tr><th scope="col" rowspan="2" class="model-column">模型</th><th v-for="group in contextGroups" :key="group.index" scope="colgroup" :colspan="contextPriceColumns.length" class="context-divider context-heading">{{ group.label }}<span v-if="group.context"> {{ group.context }}</span></th></tr>
                <tr><template v-for="group in contextGroups" :key="group.index"><th v-for="(column, index) in contextPriceColumns" :key="column.key" scope="col" :class="{ 'context-divider': index === 0 }">{{ column.label }}</th></template></tr>
              </thead>
              <tbody><tr v-for="row in priceRows" :key="row.key">
                <th scope="row" class="model-column"><span class="model-name">{{ row.model }}</span><span v-if="!row.tiers[0].label" class="model-context">{{ row.tiers[0].context }}</span></th>
                <template v-for="group in contextGroups" :key="group.index">
                  <template v-if="row.tiers[group.index]">
                    <td v-for="(column, index) in contextPriceColumns" :key="column.key" :class="{ 'context-divider': index === 0 }">
                      <span v-if="index === 0 && !group.context && row.tiers[group.index].label" class="model-context context-range">{{ row.tiers[group.index].context }}</span>
                      <template v-if="column.key === 'cache_write_price' && row.tiers[group.index].prices.cache_write_1h_price != null && row.tiers[group.index].prices.cache_write_1h_price !== row.tiers[group.index].prices.cache_write_price">
                        <span class="cache-price"><span class="cache-duration">5分钟</span> <span class="price-value">{{ formatNativePrice(row.tiers[group.index].prices.cache_write_price) }}</span></span>
                        <span v-if="row.tiers[group.index].prices.cache_write_1h_price != null" class="cache-price"><span class="cache-duration">1小时</span> <span class="price-value">{{ formatNativePrice(row.tiers[group.index].prices.cache_write_1h_price) }}</span></span>
                      </template>
                      <span v-else class="price-value" :class="{ 'price-missing': formatNativePrice(row.tiers[group.index].prices[column.key]) === '—' }">{{ formatNativePrice(row.tiers[group.index].prices[column.key]) }}</span>
                    </td>
                  </template>
                  <td v-else :colspan="contextPriceColumns.length" class="context-divider context-unavailable">{{ row.tiers[0].label ? '无此档位' : row.tiers[0].context === '全部上下文' ? '适用全部上下文，单价同左侧' : '暂无参考价' }}</td>
                </template>
              </tr></tbody>
            </table>
          </div>
          <p v-else class="pricing-note" role="status">{{ allowedModels.length ? '当前类型暂无可用模型。' : '当前工具暂无可用模型，暂不展示模型价格。' }}</p>
        </div>
        <div class="pricing-source"><a :href="source" target="_blank" rel="noopener noreferrer" title="官方参考价格，美元 / 100 万 Token">官方费率来源 <Icon name="externalLink" size="sm" /></a></div>
      </section>
      <section aria-labelledby="group-pricing-title" :aria-busy="loading">
        <div class="pricing-heading fee-heading">
          <h3 id="group-pricing-title">星桥 AI Link 扣费标准</h3>
          <button type="button" class="btn btn-secondary pricing-refresh" aria-label="刷新价格与扣费标准" :disabled="loading" @click="loadPricing"><Icon name="refresh" size="sm" />{{ loading ? '刷新中…' : '刷新' }}</button>
        </div>
        <p v-if="loading && !hasLoaded" role="status">正在更新扣费标准…</p>
        <p v-else-if="error" class="pricing-note">分组扣费标准暂不可用，请在上方重试。</p>
        <div v-else-if="feeRows.length" class="table-scroll fee-table" tabindex="0" aria-label="分组扣费标准">
          <table><thead><tr><th scope="col">分组</th><th scope="col">支持模型</th><th scope="col">倍率</th><th scope="col">扣费标准</th></tr></thead>
            <tbody><tr v-for="row in feeRows" :key="row.group.id"><th scope="row">{{ row.group.name }}</th><td class="supported-models">{{ groupModels(row.group.id) }}</td><td>{{ formatLineRate(row.rate) }}</td><td>{{ row.rate == null ? '倍率暂不可用' : `官方计费金额 × ${row.rate}` }}</td></tr></tbody>
          </table>
        </div>
        <p v-else class="pricing-note">当前工具暂无分组。</p>
      </section>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatLineRate, resolveLineRate } from '@/components/keys/lineOptions'
import type { Group } from '@/types'
import userGroupsAPI from '@/api/groups'
import { getGroupModels, getModelPopularity, type GroupModels, type ModelPopularity } from './api'
import { toolIdsForGroup, tools } from './model'
import { modelCategories, modelCategory, sortModelsByPopularity, type ModelCategory } from './modelMetadata'
import { contextPriceColumns, formatNativePrice, nativeContextGroups, nativePriceRows } from './officialPricing'

const props = defineProps<{ tool: { id: string; label: string } | null; lines: Group[]; rates: Record<number, number> }>()
const emit = defineEmits<{ close: [] }>()
const modelData = ref<GroupModels[]>([])
const popularity = ref<ModelPopularity[]>([])
const nativeGroups = ref<Group[]>([])
const nativeRates = ref<Record<number, number>>({})
const loading = ref(false)
const error = ref(false)
const hasLoaded = ref(false)
const selectedCategory = ref<ModelCategory>('all')
let lastUpdatedAt = 0
let controller: AbortController | undefined
let refreshTimer: ReturnType<typeof setTimeout> | undefined
const REFRESH_INTERVAL_MS = 60_000
const isOpenAiTool = computed(() => props.tool?.id === 'codex')
const sources: Record<string, string> = {
  gemini: 'https://ai.google.dev/gemini-api/docs/pricing',
  codex: 'https://developers.openai.com/api/docs/pricing',
  claude: 'https://platform.claude.com/docs/en/about-claude/pricing',
  grok: 'https://docs.x.ai/docs/models', deepseek: 'https://api-docs.deepseek.com/quick_start/pricing'
}
const source = computed(() => sources[props.tool?.id || 'codex'])
const modelsByGroup = computed(() => new Map(modelData.value.map(group => [group.group_id, group.supported_models])))
function groupsForTool(groups: Group[]) {
  const platform = tools.find(tool => tool.id === props.tool?.id)?.platform
  // Native tool associations are authoritative. The dashboard hint is only
  // a compatibility fallback for responses from an older backend.
  const crossPlatformGroups = new Map(props.lines.filter(group => group.platform !== platform).map(group => [group.id, group.platform]))
  return groups.filter(group => group.status === 'active' && (group.tool_ids !== undefined
    ? toolIdsForGroup(group).includes(props.tool?.id || '')
    : group.platform === platform || crossPlatformGroups.get(group.id) === group.platform))
}
const pricingGroups = computed(() => groupsForTool(nativeGroups.value))
const feeRows = computed(() => pricingGroups.value.map(group => ({ group, rate: resolveLineRate(group, nativeRates.value) })))
const allowedModels = computed(() => {
  const models = [...new Set(pricingGroups.value.flatMap(line => modelsByGroup.value.get(line.id) || []))]
  return sortModelsByPopularity(models, popularity.value)
})
const categoryOptions = computed(() => modelCategories.map(category => ({
  ...category,
  label: `${category.label}（${category.value === 'all' ? allowedModels.value.length : allowedModels.value.filter(model => modelCategory(model) === category.value).length}）`,
})))
const filteredModels = computed(() => !isOpenAiTool.value || selectedCategory.value === 'all' ? allowedModels.value : allowedModels.value.filter(model => modelCategory(model) === selectedCategory.value))
const priceRows = computed(() => nativePriceRows(filteredModels.value, modelData.value.filter(group => pricingGroups.value.some(line => line.id === group.group_id))))
const contextGroups = computed(() => nativeContextGroups(priceRows.value))

function groupModels(id: number) {
  return modelsByGroup.value.get(id)?.join('、') || '暂无可用模型'
}
function stopRefreshTimer() {
  if (refreshTimer !== undefined) clearTimeout(refreshTimer)
  refreshTimer = undefined
}
function scheduleRefresh() {
  stopRefreshTimer()
  if (props.tool && !document.hidden) refreshTimer = setTimeout(() => { void loadPricing() }, error.value ? REFRESH_INTERVAL_MS : Math.max(0, REFRESH_INTERVAL_MS - (Date.now() - lastUpdatedAt)))
}
async function loadPopularity(signal: AbortSignal) {
  try {
    const stats = await getModelPopularity(signal)
    if (!signal.aborted) popularity.value = stats.models
  } catch {
    // Keep previous ordering; an optional aggregate must not hide prices.
  }
}
async function loadPricing() {
  stopRefreshTimer()
  controller?.abort()
  if (!props.tool) return
  const current = new AbortController()
  controller = current
  void loadPopularity(current.signal)
  loading.value = true
  error.value = false
  try {
    const [groups, rates] = await Promise.all([
      userGroupsAPI.getAvailable(current.signal),
      userGroupsAPI.getUserGroupRates(current.signal),
    ])
    if (current.signal.aborted) return
    const groupIDs = groupsForTool(groups).map(group => group.id)
    // Unrelated pinned catalogues must not block the selected tool's prices.
    const models = groupIDs.length ? await getGroupModels(current.signal, groupIDs, true) : []
    if (!current.signal.aborted) {
      nativeGroups.value = groups
      modelData.value = models
      nativeRates.value = rates
      hasLoaded.value = true
      lastUpdatedAt = Date.now()
    }
  } catch {
    if (!current.signal.aborted) {
      error.value = true
      hasLoaded.value = false
    }
  } finally {
    if (!current.signal.aborted) {
      loading.value = false
      scheduleRefresh()
    }
  }
}
watch(() => props.tool?.id, () => {
  controller?.abort()
  stopRefreshTimer()
  modelData.value = []
  popularity.value = []
  nativeGroups.value = []
  nativeRates.value = {}
  error.value = false
  loading.value = false
  hasLoaded.value = false
  lastUpdatedAt = 0
  selectedCategory.value = 'all'
  if (props.tool) void loadPricing()
}, { immediate: true })
function handleVisibilityChange() {
  if (document.hidden) stopRefreshTimer()
  else if (props.tool && !loading.value) {
    if (!hasLoaded.value || error.value || Date.now() - lastUpdatedAt >= REFRESH_INTERVAL_MS) void loadPricing()
    else scheduleRefresh()
  }
}
onMounted(() => document.addEventListener('visibilitychange', handleVisibilityChange))
onBeforeUnmount(() => {
  controller?.abort()
  stopRefreshTimer()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<style scoped>
.pricing-content { display:flex; flex-direction:column; gap:32px; width:100%; min-width:0; height:100%; overflow:auto; color:var(--xq-text); font-size:14px; line-height:1.7; }
.pricing-content > section { min-width:0; flex:0 0 auto; }
.official-pricing-section { padding:16px 20px; border:1px solid var(--xq-border); border-radius:16px; background:linear-gradient(145deg,var(--xq-raised),var(--xq-depth)); }
.pricing-content > section + section { padding-top:32px; border-top:1px solid var(--xq-border); }
.pricing-heading { display:flex; justify-content:space-between; align-items:center; gap:12px; }
.pricing-content h3 { margin:0; font-size:19px; font-weight:600; }
.official-pricing-heading { margin-bottom:16px; }
.official-pricing-heading h3 { flex:0 0 auto; }
.pricing-tier-label { margin-bottom:12px; color:var(--xq-muted); font-size:12px; }
.table-scroll:focus-visible { outline:2px solid var(--xq-accent); outline-offset:2px; }
.pricing-content p { margin:8px 0; color:var(--xq-muted); }
.pricing-note { font-size:12px; }
.pricing-error { display:flex; flex-wrap:wrap; align-items:center; gap:12px; }
.pricing-source { display:flex; justify-content:flex-end; margin-top:12px; font-size:12px; }
.pricing-source a { display:inline-flex; align-items:center; gap:4px; color:var(--xq-accent); text-decoration:underline; text-underline-offset:4px; }
.table-scroll { max-width:100%; overflow:auto; }
.model-pricing-table { min-height:200px; max-height:330px; border-block:1px solid var(--xq-border); }
.pricing-content table { width:100%; border-collapse:collapse; text-align:left; }
.pricing-content th,.pricing-content td { border-bottom:1px solid var(--xq-border); padding:8px 12px; }
.pricing-content thead th { font-weight:500; color:var(--xq-muted); }
.pricing-content tbody th { font-weight:600; }
.model-pricing-table table { table-layout:fixed; }
.model-pricing-table .model-column { width:180px; }
.model-pricing-table th.model-column { position:sticky; left:0; z-index:1; background:var(--xq-raised); }
.model-pricing-table .context-divider { border-left:1px solid var(--xq-border); }
.pricing-filter { flex:0 1 260px; min-width:0; margin-left:auto; }
.pricing-filter > :first-child { width:100%; }
.pricing-content .model-name { display:block; color:var(--xq-text); font-size:14px; font-weight:650; }
.pricing-content .price-value { color:var(--xq-text); font-size:14px; font-weight:600; }
.pricing-content .price-missing { color:var(--xq-muted); font-weight:400; }
.model-context { display:block; color:var(--xq-muted); font-size:11px; font-weight:400; line-height:1.8; }
.cache-price { display:block; line-height:1.8; }
.cache-duration { color:var(--xq-muted); font-size:11px; }
.context-range { margin-bottom:4px; }
.context-unavailable { color:var(--xq-muted); font-size:12px; }
.model-pricing-table td { vertical-align:top; }
.model-pricing-table tbody th { font-size:13px; overflow-wrap:anywhere; }
.model-pricing-table th,.model-pricing-table td { padding:10px 13px; }
.model-pricing-table thead th { position:sticky; top:40px; z-index:2; height:40px; box-sizing:border-box; background:var(--xq-depth); color:var(--xq-text); font-size:12px; line-height:1.3; white-space:nowrap; }
.model-pricing-table thead tr:first-child th { top:0; font-size:13px; }
.model-pricing-table thead th.model-column { z-index:3; background:var(--xq-depth); }
.model-pricing-table .context-heading span { margin-left:8px; color:var(--xq-muted); font-weight:400; }
.model-pricing-table td { font-variant-numeric:tabular-nums; white-space:nowrap; }
.model-pricing-table tbody tr:nth-child(even) { background:var(--xq-depth); }
.model-pricing-table tbody tr:nth-child(even) th.model-column { background:var(--xq-depth); }

.fee-heading { margin-bottom:24px; }
.pricing-refresh { display:inline-flex; align-items:center; gap:6px; flex:0 0 auto; }
.fee-table table { min-width:720px; table-layout:fixed; }
.pricing-content .fee-table thead th { color:var(--xq-text); font-size:12px; }
.pricing-content .fee-table tbody th,.pricing-content .fee-table td { color:var(--xq-text); font-size:14px; font-weight:600; }
.pricing-content .fee-table tbody th { font-weight:650; }
.fee-table thead th:nth-child(1) { width:18%; }
.fee-table thead th:nth-child(2) { width:40%; }
.fee-table thead th:nth-child(3) { width:12%; }
.fee-table thead th:nth-child(4) { width:30%; }
.fee-table td,.fee-table tbody th { vertical-align:top; }
.supported-models { overflow-wrap:anywhere; }
@media (min-width:641px) {
  :global(.xq-dialog .xq-pricing-dialog.xq-context-pricing-dialog) { width:min(1360px,calc(100vw - 48px)); }
}
@media (max-width:640px) {
  .fee-heading { flex-wrap:wrap; }
  .official-pricing-section { padding:16px 12px; }
  .pricing-content h3 { font-size:17px; }
  .official-pricing-heading { flex-wrap:wrap; }
  .pricing-filter { flex-basis:100%; margin-left:0; }
  .model-pricing-table .model-column { width:120px; }
}
</style>
