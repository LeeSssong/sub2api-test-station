<template>
  <article class="w-full min-w-0 overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-950" data-test="monitor-card" :data-account-id="account.account_id">
    <div class="grid min-w-0 gap-x-4 gap-y-3 px-[18px] py-4 max-[430px]:px-[14px] max-[430px]:py-[14px] xl:grid-cols-[minmax(15rem,1.45fr)_minmax(10rem,.9fr)_minmax(11rem,1fr)_minmax(15rem,1.35fr)_minmax(13rem,1.1fr)_auto]" data-test="monitor-card-header">
      <section class="min-w-0" data-test="identity-column" aria-label="账号身份与状态">
        <div class="flex min-w-0 items-start gap-2">
          <span class="mt-1.5 h-2 w-2 shrink-0 rounded-full" :class="statusDotClass" aria-hidden="true" />
          <div class="min-w-0">
            <h2 class="break-words text-base font-semibold leading-6 text-gray-900 dark:text-white [overflow-wrap:anywhere]" data-test="account-identity">
              <a
                v-if="account.homepage_url"
                :href="account.homepage_url"
                target="_blank"
                rel="noopener noreferrer"
                class="border-b border-dotted border-gray-300 dark:border-slate-600"
                data-test="account-homepage-link"
                :title="`打开上游网站：${account.homepage_url}`"
              >{{ account.name }}</a>
              <span v-else>{{ account.name }}</span>
              <span class="font-mono text-xs font-normal text-gray-500 dark:text-slate-400"> #{{ account.account_id }}</span>
            </h2>
            <div class="mt-1 flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[10px] leading-4 text-gray-500 dark:text-slate-400" data-test="account-metadata">
              <span>平台 {{ platformLabel }}</span>
              <span aria-hidden="true">/</span>
              <span class="min-w-0 break-words">当前分组 {{ currentGroupLabel }}</span>
              <span aria-hidden="true">/</span>
              <span>调度状态 {{ schedulableLabel }}</span>
              <template v-if="recommendation">
                <span aria-hidden="true">/</span>
                <HelpTooltip v-if="formalMigration" class="!ml-0" width-class="w-80" data-test="recommendation-tooltip-trigger">
                  <template #trigger>
                    <span class="inline-flex min-w-0 items-center gap-0.5 text-amber-600 dark:text-amber-300" data-test="group-recommendation">
                      <span class="break-words">{{ recommendationLabel }}</span>
                      <button class="inline-flex h-5 w-5 shrink-0 items-center justify-center focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-500" data-test="recommendation-warning" type="button" :title="recommendationTooltip" :aria-label="recommendationTooltip">!</button>
                    </span>
                  </template>
                  <div data-test="group-recommendation-tooltip">{{ recommendationTooltip }}</div>
                </HelpTooltip>
                <span v-else class="break-words" data-test="group-recommendation" :class="recommendationTextClass">{{ recommendationLabel }}</span>
              </template>
            </div>
          </div>
        </div>
        <div class="mt-2 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span class="rounded-full px-2 py-0.5 text-xs font-semibold" :class="statusBadgeClass" data-test="status-badge">{{ statusLabel }}</span>
        </div>
      </section>

      <section class="min-w-0 border-l border-gray-100 pl-4 dark:border-slate-800" data-test="quality-column" aria-label="质量视图">
        <div class="min-h-[121px] min-w-0 p-[14px] max-[430px]:min-h-[114px] max-[430px]:px-2 max-[430px]:py-[11px]" data-test="score-metric" :title="scoreTooltip" :aria-label="scoreTooltip">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">质量评分</div>
          <div class="mt-1 flex items-baseline gap-1.5">
          <HelpTooltip class="!ml-0" trigger="click" width-class="w-80" data-test="score-tooltip-trigger">
            <template #trigger>
              <button class="flex min-w-0 items-baseline gap-1.5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500" type="button" :title="scoreTooltip" :aria-label="scoreTooltip">
                <strong class="font-mono text-2xl font-semibold text-gray-900 dark:text-white">{{ scoreLabel }}</strong><span class="text-xs font-semibold text-gray-500 dark:text-slate-400">/ 100</span>
              </button>
            </template>
            <div data-test="score-breakdown-tooltip">{{ scoreTooltip }}</div>
          </HelpTooltip>
          </div>
          <p class="mt-1 text-[10px] text-gray-500 dark:text-slate-400" data-test="score-evidence-detail">{{ evidenceDetail }}</p>
        </div>
        <div class="mt-3" data-test="rank-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">{{ qualityRankTitle }}</div>
          <div class="mt-0.5 flex min-w-0 items-baseline gap-1.5"><strong class="truncate font-mono text-lg font-semibold text-gray-900 dark:text-white" data-test="quality-rank">{{ qualityRankLabel }}<span v-if="qualityRanked" class="text-xs font-normal text-gray-500 dark:text-slate-400"> / {{ qualityRankTotalLabel }}</span></strong></div>
        </div>
      </section>

      <section class="min-w-0 border-l border-gray-100 pl-4 dark:border-slate-800" data-test="scheduler-column" aria-label="调度优先级">
        <div class="text-[11px] text-gray-500 dark:text-slate-400">调度优先级</div>
        <div class="mt-1 flex min-w-0 items-baseline gap-1.5"><strong class="truncate font-mono text-lg font-semibold text-gray-900 dark:text-white" data-test="scheduler-rank">{{ schedulerRankLabel }}<span v-if="schedulerRanked" class="text-xs font-normal text-gray-500 dark:text-slate-400"> / {{ schedulerRankTotalLabel }}</span></strong></div>
        <p class="mt-1 break-words text-[10px] text-gray-500 dark:text-slate-400" data-test="scheduler-policy">{{ schedulerPolicyLabel }}</p>
        <div class="mt-2 flex items-center gap-1" data-test="priority-control">
          <span class="text-[10px] text-gray-500 dark:text-slate-400">全局优先级</span>
          <template v-if="editingPriority">
            <label class="sr-only" :for="`account-priority-${account.account_id}`">全局优先级</label>
            <input :id="`account-priority-${account.account_id}`" ref="priorityInput" v-model="draftPriority" class="input h-8 min-w-0 w-20 px-2 py-1 font-mono text-sm" data-test="priority-input" inputmode="numeric" min="1" step="1" type="number" :disabled="savingPriority" @keyup.enter="savePriority" @keyup.esc="cancelPriorityEdit">
            <button class="icon-button h-8 w-8 shrink-0" data-test="save-priority" type="button" title="保存全局优先级" aria-label="保存全局优先级" :disabled="savingPriority" @click="savePriority"><Icon name="check" size="xs" /></button>
            <button class="icon-button h-8 w-8 shrink-0" data-test="cancel-priority" type="button" title="取消编辑全局优先级" aria-label="取消编辑全局优先级" :disabled="savingPriority" @click="cancelPriorityEdit"><Icon name="x" size="xs" /></button>
          </template>
          <template v-else>
            <strong class="font-mono text-base text-gray-900 dark:text-white">{{ displayedPriority }}</strong>
            <button class="icon-button h-8 w-8 shrink-0" data-test="edit-priority" type="button" title="编辑全局优先级" aria-label="编辑全局优先级" @click="beginPriorityEdit"><Icon name="edit" size="xs" /></button>
          </template>
        </div>
        <p v-if="priorityError" class="mt-1 text-[11px] text-red-600 dark:text-red-400" data-test="priority-error" role="alert">{{ priorityError }}</p>
      </section>

      <section class="grid min-w-0 grid-cols-2 gap-x-3 gap-y-2 text-xs sm:grid-cols-3 lg:grid-cols-2" data-test="key-metrics" aria-label="关键服务指标">
        <MetricCell data-test="success-rate-metric" tone="success" label="可用性" :value="formatPercent(probeSuccessRate)" detail="当前可用性" />
        <MetricCell data-test="ttft-metric" tone="ttft" label="首 Token P50" :value="formatMs(probeTTFTP50MS)" detail="成功响应" />
        <MetricCell data-test="latency-metric" tone="latency" label="完整响应 P95" :value="formatMs(probeLatencyP95MS)" detail="成功响应" />
        <div class="service-metric min-w-0 border-l border-violet-200 bg-violet-50 px-2 pl-3 dark:border-violet-900/50 dark:bg-violet-950/20" data-test="cost-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400"><HelpTooltip class="!ml-0" trigger="click" width-class="w-72" data-test="cost-tooltip-trigger"><template #trigger><button class="cursor-help text-left" type="button" :title="costSourceTooltip" :aria-label="costSourceTooltip">账号成本<span v-if="manualCost" class="ml-1 font-bold text-amber-500 dark:text-amber-300" data-test="manual-cost-warning">!</span></button></template><div data-test="cost-source-tooltip">{{ costSourceTooltip }}</div></HelpTooltip></div>
          <div class="mt-1 break-words font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ costValue }}</div>
          <p class="mt-1 break-words text-[10px] leading-4 text-gray-400 dark:text-slate-500" data-test="cost-detail">{{ costDetail }}</p>
          <div class="mt-1 flex items-center gap-1" data-test="cost-actions"><button class="icon-button h-7 w-7" data-test="edit-cost" type="button" title="编辑账号成本" aria-label="编辑账号成本" @click="emit('editCost', account)"><Icon name="edit" size="xs" /></button></div>
        </div>
        <div v-if="isOpenAIAPIKey" class="min-w-0 border-l border-cyan-200 bg-cyan-50/70 px-2 pl-3 dark:border-cyan-900/50 dark:bg-cyan-950/20" data-test="balance-metric"><div class="text-[11px] text-gray-500 dark:text-slate-400">上游余额</div><div class="mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ balanceValue }}</div><p class="mt-1 break-words text-[10px] leading-4 text-gray-400 dark:text-slate-500">{{ balanceDetail }}</p></div>
        <div class="service-metric min-w-0 border-l border-gray-200 bg-gray-50 pl-3 dark:border-slate-700 dark:bg-slate-900/50" data-test="concurrency-metric"><div class="text-[11px] text-gray-500 dark:text-slate-400">当前并发</div><div class="mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ concurrencyValue }}</div><p class="mt-1 break-words text-[10px] text-gray-400 dark:text-slate-500">{{ concurrency?.delayed ? '数据延迟' : '近实时运维快照' }}</p></div>
        <div class="col-span-2 min-w-0" data-test="equivalent-cost-multiplier"><CostMetric label="成本折合本站倍率" :value="formatMultiplier(account.equivalent_site_multiplier)" /></div>
      </section>

      <section class="min-w-0 border-l border-gray-100 pl-4 dark:border-slate-800" data-test="timeline-section" aria-label="近期表现">
        <div class="flex min-w-0 items-center justify-between gap-2"><h3 class="text-xs font-semibold text-gray-800 dark:text-slate-100">近期表现</h3><button type="button" class="shrink-0 text-[11px] text-primary-600 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-primary-300" data-test="edit-connection-probe-model" @click="openModelDetectionDialog">{{ t('admin.accounts.modelDetection.editConnectionProbeModel') }}</button></div>
        <div class="mt-3 grid h-9 min-w-0 grid-cols-[repeat(24,minmax(3px,1fr))] items-end gap-1" role="img" :aria-label="timelineAriaLabel"><span v-for="(bar, index) in probeBars" :key="`${account.account_id}-${index}`" class="min-w-0 rounded-sm" :class="bar.colorClass" :style="{ height: `${bar.height}%` }" :title="bar.title" data-test="probe-bar" aria-hidden="true" /></div>
        <div class="mt-1 flex justify-between text-[10px] text-gray-400 dark:text-slate-500"><span>较早</span><span>最近</span></div>
      </section>

      <section class="flex min-w-0 flex-wrap items-start justify-end gap-1 xl:flex-col xl:items-stretch" data-test="account-actions" aria-label="账号操作">
        <span class="sr-only">账号操作</span>
        <button class="icon-button h-8 w-8 xl:w-full" data-test="account-info" type="button" title="查看账号信息" aria-label="查看账号信息" @click="emit('accountInfo', account)"><Icon name="eye" size="xs" /><span class="sr-only xl:not-sr-only xl:ml-1 xl:text-[11px]">账号信息</span></button>
        <button class="icon-button h-8 w-8 xl:w-full" data-test="account-edit" type="button" title="编辑账号" aria-label="编辑账号" @click="emit('accountEdit', account)"><Icon name="edit" size="xs" /><span class="sr-only xl:not-sr-only xl:ml-1 xl:text-[11px]">编辑</span></button>
        <button class="icon-button h-8 w-8 xl:w-full" data-test="account-delete" type="button" title="删除账号" aria-label="删除账号" @click="emit('accountDelete', account)"><Icon name="trash" size="xs" /><span class="sr-only xl:not-sr-only xl:ml-1 xl:text-[11px]">删除</span></button>
        <button class="icon-button h-8 w-8 xl:w-full" data-test="account-more" type="button" title="更多账号操作" aria-label="更多账号操作" @click="emit('accountMore', account, $event)"><Icon name="more" size="xs" /><span class="sr-only xl:not-sr-only xl:ml-1 xl:text-[11px]">更多</span></button>
        <button class="icon-button h-8 w-8 xl:w-full" data-test="refresh-account" type="button" title="刷新当前账号" aria-label="刷新当前账号" :disabled="running" @click="emit('refresh', account.account_id)"><Icon name="refresh" size="sm" :class="{ 'animate-spin': running }" /><span class="sr-only xl:not-sr-only xl:ml-1 xl:text-[11px]">刷新</span></button>
      </section>

      <div class="min-w-0 xl:col-span-6">
        <div class="flex min-w-0 items-start gap-3 border-t border-gray-100 pt-3 dark:border-slate-800">
          <p class="min-w-0 flex-1 break-words text-[11px] leading-4 text-gray-500 dark:text-slate-400" data-test="ranking-reason">{{ rankingReason }}</p>
          <button class="inline-flex min-h-8 shrink-0 items-center gap-1 rounded-md px-2 text-xs font-medium text-gray-600 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-slate-300 dark:hover:bg-slate-800" data-test="ranking-explanation-toggle" type="button" :aria-controls="rankingExplanationID" :aria-expanded="rankingExplanationExpanded" @click="rankingExplanationExpanded = !rankingExplanationExpanded"><Icon name="eye" size="xs" /><span>{{ rankingExplanationExpanded ? '收起依据' : '查看排名依据' }}</span><Icon name="chevronDown" size="xs" :class="{ 'rotate-180': rankingExplanationExpanded }" /></button>
        </div>
        <div v-if="rankingExplanationExpanded" :id="rankingExplanationID" class="grid min-w-0 gap-3 border-t border-gray-100 pt-3 text-xs dark:border-slate-800 sm:grid-cols-2 lg:grid-cols-4" data-test="ranking-explanation">
          <div class="min-w-0"><div class="font-semibold text-gray-700 dark:text-slate-200">质量构成</div><div class="mt-1 grid gap-1 text-gray-500 dark:text-slate-400"><span v-for="item in qualityBreakdownItems" :key="item.key">{{ item.label }} {{ item.score }} / {{ item.max }}</span><span v-if="!qualityBreakdownItems.length">暂无评分构成</span><span v-if="qualityExplanationSummary">{{ qualityExplanationSummary }}</span></div></div>
          <div class="min-w-0"><div class="font-semibold text-gray-700 dark:text-slate-200">调度事实</div><div class="mt-1 grid gap-1 break-words text-gray-500 dark:text-slate-400"><span>策略 {{ schedulerPolicyLabel }}</span><span>{{ schedulerEligibilityLabel }}</span><span v-if="schedulerCandidateLabel">{{ schedulerCandidateLabel }}</span><span v-if="schedulerScopeLabel">范围 {{ schedulerScopeLabel }}</span></div></div>
          <div class="min-w-0"><div class="font-semibold text-gray-700 dark:text-slate-200">快照与权重</div><div class="mt-1 grid gap-1 break-words text-gray-500 dark:text-slate-400"><span v-if="effectiveWeightsLabel">权重 {{ effectiveWeightsLabel }}</span><span v-if="schedulerSnapshotLabel">快照 {{ schedulerSnapshotLabel }}</span><span v-if="schedulerTieBreakLabel">并列处理 {{ schedulerTieBreakLabel }}</span><span v-if="!effectiveWeightsLabel && !schedulerSnapshotLabel && !schedulerTieBreakLabel">暂无额外调度事实</span></div></div>
          <div class="min-w-0"><div class="font-semibold text-gray-700 dark:text-slate-200">来源</div><div class="mt-1 grid gap-1 break-words text-gray-500 dark:text-slate-400"><span>评分来源 {{ qualitySourceLabel }}</span><span>评分样本 {{ qualitySampleLabel }}</span><span>评分时间 {{ qualityObservedAtLabel }}</span><span v-if="schedulerReasonLabel">原因 {{ schedulerReasonLabel }}</span></div></div>
        </div>
      </div>

      <section class="min-w-0 border-t border-gray-100 pt-3 xl:col-span-6 dark:border-slate-800" data-test="model-detection-section">
        <button type="button" class="flex min-h-8 w-full min-w-0 items-center gap-2 text-left text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500" data-test="model-detection-status-row" :aria-expanded="modelDetectionDialogOpen" @click="openModelDetectionEntry"><span class="font-semibold text-gray-700 dark:text-slate-200">{{ t('admin.accounts.modelDetection.section') }}</span><span class="rounded-full px-2 py-0.5" :class="modelDetectionStatusClass">{{ modelDetectionStatusLabel }}</span><span class="min-w-0 flex-1 truncate text-gray-500 dark:text-slate-400">{{ modelDetectionStatusHint }}</span><Icon name="chevronDown" size="xs" /></button>
      </section>

      <section class="min-w-0 border-t border-gray-100 xl:col-span-6 dark:border-slate-800" data-test="calls-disclosure">
        <button class="flex min-h-10 w-full min-w-0 items-center gap-2 text-left text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500" data-test="calls-toggle" type="button" :aria-controls="callsPanelID" :aria-expanded="callsExpanded" @click="callsExpanded = !callsExpanded"><span class="font-semibold text-gray-800 dark:text-slate-100">{{ callsTitle }}</span><span class="min-w-0 truncate text-[11px] text-gray-500 dark:text-slate-400">{{ callsSummary }}</span><Icon name="chevronDown" size="xs" class="ml-auto" :class="{ 'rotate-180': callsExpanded }" /></button>
        <div v-if="callsExpanded" :id="callsPanelID" class="grid grid-cols-2 gap-3 border-t border-gray-100 pb-1 pt-3 text-xs dark:border-slate-800"><div><div class="text-[10px] text-gray-500 dark:text-slate-400">成功请求</div><div class="mt-1 font-mono font-semibold text-gray-900 dark:text-white">{{ successfulRequestCount }}</div></div><div><div class="text-[10px] text-gray-500 dark:text-slate-400">失败请求</div><div class="mt-1 font-mono font-semibold text-gray-900 dark:text-white">{{ account.error_count }}</div></div></div>
      </section>

      <footer class="flex min-w-0 flex-wrap items-center justify-between gap-2 border-t border-gray-100 pt-3 text-[11px] text-gray-500 max-[430px]:flex-col max-[430px]:items-start max-[430px]:gap-[3px] max-[430px]:py-[9px] xl:col-span-6 dark:border-slate-800 dark:text-slate-400" data-test="card-footer"><span class="break-words">检查于 {{ checkedAtLabel }} · 统计截止 {{ statisticsCutoffLabel }}</span></footer>
    </div>
    <AccountModelDetectionDialog :show="modelDetectionDialogOpen" :account="account" :models="modelDetectionModels" :saving="savingModelDetection" :detecting="detectingModelDetection" @close="modelDetectionDialogOpen = false" @save="emit('saveModelDetectionModels', account.account_id, $event)" @detect="emit('detectModelDetection', account.account_id)" />
  </article>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import type { AccountModelDetectionModelsResponse, AccountMonitorAccount, AccountMonitorConcurrencyItem, AccountMonitorGroupRecommendation, AccountMonitorRange } from '@/api/admin/accountMonitor'
import AccountModelDetectionDialog from './AccountModelDetectionDialog.vue'

type CardConcurrency = AccountMonitorConcurrencyItem & { delayed?: boolean }
type ProbeBar = { colorClass: string, height: number, title: string }
type SchedulerDetails = NonNullable<AccountMonitorAccount['scheduler_explanation']> & {
  reason?: string | null
  tie_break?: string | null
  tie_break_reason?: string | null
}
type BreakdownItem = { key: string, label: string, score: string, max: string }

const props = withDefaults(defineProps<{
  account: AccountMonitorAccount
  concurrency?: CardConcurrency | null
  running?: boolean
  rankedAccountCount?: number
  rankingScope?: 'group' | 'global'
  statisticsCutoff?: string | null
  selectedRange?: AccountMonitorRange
  modelDetectionModels?: AccountModelDetectionModelsResponse | null
  savingModelDetection?: boolean
  detectingModelDetection?: boolean
  useHistoryPanel?: boolean
}>(), { concurrency: null, running: false, rankedAccountCount: 0, rankingScope: 'group', statisticsCutoff: null, selectedRange: '24h', useHistoryPanel: false })
const { t } = useI18n()

const emit = defineEmits<{
  (event: 'updatePriority', accountID: number, priority: number, completion: { resolve: () => void; reject: (reason?: unknown) => void }): void
  (event: 'editCost', account: AccountMonitorAccount): void
  (event: 'accountInfo', account: AccountMonitorAccount): void
  (event: 'accountEdit', account: AccountMonitorAccount): void
  (event: 'accountDelete', account: AccountMonitorAccount): void
  (event: 'accountMore', account: AccountMonitorAccount, triggerEvent?: MouseEvent): void
  (event: 'refresh', accountID: number): void
  (event: 'editConnectionProbeModel', account: AccountMonitorAccount): void
  (event: 'saveModelDetectionModels', accountID: number, payload: { connectionModel: string; detectionModel: string }): void
  (event: 'detectModelDetection', accountID: number): void
  (event: 'openModelDetectionHistory', accountID: number): void
}>()

const displayedPriority = ref(props.account.priority)
const editingPriority = ref(false)
const savingPriority = ref(false)
const draftPriority = ref(String(displayedPriority.value))
const priorityError = ref('')
const priorityInput = ref<HTMLInputElement | null>(null)
const callsExpanded = ref(false)
const modelDetectionDialogOpen = ref(false)
function openModelDetectionDialog() {
  modelDetectionDialogOpen.value = true
  emit('editConnectionProbeModel', props.account)
}
function openModelDetectionEntry() {
  if (props.useHistoryPanel) {
    emit('openModelDetectionHistory', props.account.account_id)
    return
  }
  openModelDetectionDialog()
}

const platformLabel = computed(() => props.account.platform || '--')
const currentGroupLabel = computed(() => props.account.group_names?.filter(Boolean).join('、') || '--')
const schedulableLabel = computed(() => props.account.status !== 'active' ? '暂停' : props.account.schedulable ? '可调度' : '不可调度')
const recommendation = computed<AccountMonitorGroupRecommendation | null>(() => props.account.group_recommendation ?? null)
const isTestGroup = computed(() => props.account.group_names?.some((name) => name.trim().toLowerCase().replace(/ /g, '') === 'gpt-测试分组') ?? false)
const formalMigration = computed(() => recommendation.value?.action === 'migrate' && !isTestGroup.value)
const recommendationTargetLabel = computed(() => recommendation.value?.target_name || ({ gpt_pro: 'GPT-Pro', gpt_plus: 'GPT-Plus', gpt_special: 'GPT-特惠' }[recommendation.value?.target ?? ''] ?? '目标分组'))
const recommendationLabel = computed(() => {
  switch (recommendation.value?.status) {
    case 'recommended': return `推荐：${recommendationTargetLabel.value}`
    case 'observe': return '继续观察'
    case 'blocked': return '暂缓迁入'
    case 'not_recommended': return '暂不建议入组'
    default: return ''
  }
})
const recommendationTextClass = computed(() => ({
  'text-emerald-600 dark:text-emerald-300': recommendation.value?.status === 'recommended',
  'text-amber-600 dark:text-amber-300': recommendation.value?.status === 'observe' || recommendation.value?.status === 'blocked',
  'text-red-600 dark:text-red-300': recommendation.value?.status === 'not_recommended',
}))
const recommendationTooltip = computed(() => {
  const item = recommendation.value
  if (!item) return ''
  return `推荐迁移至 ${recommendationTargetLabel.value}`
})

watch(() => props.account.priority, (value) => {
  if (!editingPriority.value && !savingPriority.value) displayedPriority.value = value
})

const statusLabel = computed(() => {
  if (props.account.availability_status === 'disabled' || props.account.management_state === 'paused') return '暂停'
  if (props.account.availability_status === 'normal') return '正常'
  if (props.account.availability_status === 'abnormal') return '异常'
  if (props.account.availability_status === 'stale') return '待确认'
  if (props.account.availability_status === 'unavailable') return '不可用'
  if (props.account.service_state === 'available') return '正常'
  if (props.account.service_state === 'pending') return '待确认'
  return '不可用'
})
const statusBadgeClass = computed(() => ({
  'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300': statusLabel.value === '正常',
  'bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300': statusLabel.value === '待确认' || statusLabel.value === '异常',
  'bg-gray-100 text-gray-600 dark:bg-slate-800 dark:text-slate-300': statusLabel.value === '暂停',
  'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-300': statusLabel.value === '不可用',
}))
const statusDotClass = computed(() => ({
  'bg-emerald-500': statusLabel.value === '正常',
  'bg-amber-500': statusLabel.value === '待确认' || statusLabel.value === '异常',
  'bg-gray-400': statusLabel.value === '暂停',
  'bg-red-500': statusLabel.value === '不可用',
}))
const evidenceDetail = computed(() => {
  if (props.account.evidence_source === 'historical_final') {
    return `${t('admin.accounts.monitor.historyFallback')} · ${t('admin.accounts.monitor.historyFallbackAt')} ${checkedAtLabel.value}`
  }
  return props.account.quality_score == null ? '评分暂不可用' : '当前服务表现'
})
const manualCost = computed(() => {
  if (props.account.platform.toLowerCase() === 'openai' && isAPIKeyAccountType(props.account.account_type)) return props.account.multiplier.source === 'manual'
  return props.account.cost_mode === 'procurement' || (props.account.procurement_cost_cny != null && props.account.multiplier.source !== 'declared')
})
const costSourceTooltip = computed(() => {
  if (manualCost.value) {
    if (props.account.cost_mode === 'procurement' || props.account.procurement_cost_cny != null) return '手工维护：采购成本/预计额度由管理员录入'
    return '手工维护：账号倍率由管理员录入'
  }
  if (props.account.multiplier.source === 'declared') return '上游原生：倍率来自上游 billing 字段'
  return '成本来源：当前没有可用的上游原生或手工成本证据'
})
const scoreTooltip = computed(() => {
  return props.account.quality_score == null ? '当前服务评分暂不可用' : `当前服务评分 ${scoreLabel.value} / 100`
})
const scoreEligible = computed(() => props.account.score_status
  ? ['eligible', 'capped'].includes(props.account.score_status)
  : props.account.eligible === true)
const scoreLabel = computed(() => {
  if (!scoreEligible.value || props.account.quality_score == null || !Number.isFinite(props.account.quality_score)) return '--'
  const value = props.account.score_status === 'capped' ? Math.min(props.account.quality_score, 70) : props.account.quality_score
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(value)
})
const qualityRankValue = computed(() => props.account.quality_rank ?? props.account.group_rank)
const qualityRanked = computed(() => qualityRankValue.value != null)
const qualityRankLabel = computed(() => qualityRanked.value ? `第 ${qualityRankValue.value}` : '未排名')
const qualityRankTotalLabel = computed(() => props.account.quality_rank_total ?? props.rankedAccountCount)
const qualityRankTitle = computed(() => props.rankingScope === 'global' ? '全站质量排名' : '组内质量排名')
const schedulerExplanation = computed(() => props.account.scheduler_explanation as SchedulerDetails | null)
const schedulerRanked = computed(() => props.account.scheduler_rank != null)
const schedulerRankLabel = computed(() => schedulerRanked.value ? `第 ${props.account.scheduler_rank}` : '暂不可用')
const schedulerRankTotalLabel = computed(() => props.account.scheduler_rank_total ?? '--')
const schedulerPolicyLabel = computed(() => schedulerExplanation.value?.policy_label || (props.account.scheduler_unavailable ? '调度投影暂不可用' : '暂无策略说明'))
const schedulerEligibilityLabel = computed(() => schedulerExplanation.value ? (schedulerExplanation.value.eligible ? '符合调度条件' : '不符合调度条件') : '资格状态暂不可用')
const schedulerCandidateLabel = computed(() => schedulerExplanation.value?.candidate_total == null ? '' : `候选数 ${formatNumber(schedulerExplanation.value.candidate_total)}`)
const schedulerScopeLabel = computed(() => schedulerExplanation.value?.candidate_scope || '')
const schedulerSnapshotLabel = computed(() => schedulerExplanation.value?.snapshot_at ? formatDateTime(schedulerExplanation.value.snapshot_at) : '')
const schedulerTieBreakLabel = computed(() => schedulerExplanation.value?.tie_break_reason || schedulerExplanation.value?.tie_break || '')
const schedulerReasonLabel = computed(() => schedulerExplanation.value?.primary_reason_label || schedulerExplanation.value?.reason || '')
const rankingReason = computed(() => schedulerReasonLabel.value || (props.account.scheduler_unavailable ? '调度投影暂不可用' : '暂无服务端调度原因'))
const rankingExplanationExpanded = ref(false)
const rankingExplanationID = computed(() => `account-ranking-explanation-${props.account.account_id}`)
const qualityBreakdownItems = computed<BreakdownItem[]>(() => {
  const breakdown = props.account.quality_explanation?.breakdown
  if (breakdown) {
    return [
      { key: 'cost', label: '成本', score: formatMetricNumber(breakdown.cost.score), max: formatMetricNumber(breakdown.cost.max) },
      { key: 'success', label: '成功', score: formatMetricNumber(breakdown.success.score), max: formatMetricNumber(breakdown.success.max) },
      { key: 'ttft', label: '首 Token', score: formatMetricNumber(breakdown.ttft.score), max: formatMetricNumber(breakdown.ttft.max) },
      { key: 'latency', label: '延迟', score: formatMetricNumber(breakdown.latency.score), max: formatMetricNumber(breakdown.latency.max) },
    ]
  }
  const legacyBreakdown = props.account.score_breakdown
  if (!legacyBreakdown) return []
  return [
    { key: 'cost', label: '成本', score: formatMetricNumber(legacyBreakdown.cost), max: '--' },
    { key: 'success', label: '成功', score: formatMetricNumber(legacyBreakdown.success), max: '--' },
    { key: 'ttft', label: '首 Token', score: formatMetricNumber(legacyBreakdown.ttft), max: '--' },
    { key: 'latency', label: '延迟', score: formatMetricNumber(legacyBreakdown.latency), max: '--' },
  ]
})
const qualityExplanationSummary = computed(() => {
  const explanation = props.account.quality_explanation
  if (!explanation) return ''
  return `窗口 ${explanation.window} · 样本 ${formatNumber(explanation.sample_count)} · 来源 ${explanation.source}`
})
const qualitySourceLabel = computed(() => props.account.quality_explanation?.source || props.account.evidence_source || '--')
const qualitySampleLabel = computed(() => formatNumber(props.account.quality_explanation?.sample_count ?? props.account.sample_count))
const qualityObservedAtLabel = computed(() => formatDateTime(props.account.quality_explanation?.observed_at ?? props.account.checked_at ?? null))
const effectiveWeightsLabel = computed(() => {
  const weights = schedulerExplanation.value?.effective_weights
  if (!weights) return ''
  return Object.entries(weights).map(([key, value]) => `${formatWeightKey(key)} ${formatMetricNumber(value)}%`).join(' · ')
})
const concurrencyValue = computed(() => props.concurrency ? `${props.concurrency.current} / ${props.concurrency.limit}` : '-- / --')
const callsPanelID = computed(() => `account-calls-${props.account.account_id}`)
const callsTitle = computed(() => ({ '24h': '24 小时调用', '7d': '7 天调用', '30d': '30 天调用' }[props.selectedRange]))
const callsSummary = computed(() => `${formatNumber(props.account.request_count)} 次请求 · ${formatNumber(props.account.error_count)} 次失败`)
const successfulRequestCount = computed(() => Math.max(0, Number(props.account.request_count) - Number(props.account.error_count)))
const probeSuccessRate = computed(() => props.account.probe_success_rate ?? props.account.success_rate ?? 0)
const probeTTFTP50MS = computed(() => props.account.probe_ttft_p50_ms ?? props.account.ttft_p50_ms ?? null)
const probeLatencyP95MS = computed(() => props.account.probe_latency_p95_ms ?? props.account.latency_p95_ms ?? null)
const checkedAtLabel = computed(() => formatDateTime(props.account.checked_at ?? props.account.latest?.checked_at ?? null))
const statisticsCutoffLabel = computed(() => formatShortTime(props.statisticsCutoff))
const timelinePoints = computed(() => (props.account.timeline ?? []).slice(-24))
const probeBars = computed<ProbeBar[]>(() => {
  const bars: ProbeBar[] = Array.from({ length: Math.max(0, 24 - timelinePoints.value.length) }, () => ({ colorClass: 'bg-gray-200 dark:bg-slate-700', height: 15, title: '暂无探测' }))
  for (const point of timelinePoints.value) {
    const timestamp = formatDateTime(point.checked_at)
    if (isCompletedProbe(point.status)) {
      const latency = point.latency_ms ?? point.ttft_ms
      bars.push({ colorClass: 'bg-emerald-500 dark:bg-emerald-400', height: point.status === 'unavailable' || point.status === 'model_unavailable' || point.status === 'degraded' ? 40 : latencyBarHeight(latency), title: `${timestamp} · ${latency == null ? '探测完成' : `成功 · ${formatMs(latency)}`}` })
    } else if (isFailedProbe(point.status)) {
      bars.push({ colorClass: 'bg-gray-200 dark:bg-slate-700', height: 15, title: `${timestamp} · 暂无结果` })
    } else {
      bars.push({ colorClass: 'bg-gray-200 dark:bg-slate-700', height: 15, title: `${timestamp} · 暂无结果` })
    }
  }
  return bars
})
const timelineAriaLabel = computed(() => '近期成功表现')
const modelDetectionStatus = computed(() => props.account.model_detection?.status ?? 'untested')
const modelDetectionStatusLabel = computed(() => t(`admin.accounts.modelDetection.status.${modelDetectionStatus.value}`))
const modelDetectionStatusHint = computed(() => {
  if (modelDetectionStatus.value === 'service_unconfigured') return t('admin.accounts.modelDetection.detectorUnconfigured')
  if (modelDetectionStatus.value === 'service_unavailable') return t('admin.accounts.modelDetection.detectorUnavailable')
  if (props.account.model_detection?.recent?.source === 'historical_final') return t('admin.accounts.modelDetection.historicalFallback')
  if (props.account.model_detection?.recent?.source === 'historical') return t('admin.accounts.modelDetection.historicalRecordHint')
  return modelDetectionStatus.value === 'abnormal' ? t('admin.accounts.modelDetection.observedAbnormal') : props.account.model_detection?.recent?.error_message ?? t('admin.accounts.modelDetection.viewRecent')
})
const modelDetectionStatusClass = computed(() => ({ 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300': modelDetectionStatus.value === 'normal' && !['historical', 'historical_final'].includes(props.account.model_detection?.recent?.source ?? ''), 'bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300': ['queued', 'running', 'abnormal', 'insufficient'].includes(modelDetectionStatus.value), 'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-300': ['failed', 'service_unavailable'].includes(modelDetectionStatus.value), 'bg-gray-100 text-gray-600 dark:bg-slate-800 dark:text-slate-300': ['untested', 'unsupported', 'service_unconfigured'].includes(modelDetectionStatus.value) || ['historical', 'historical_final'].includes(props.account.model_detection?.recent?.source ?? '') }))
const multiplierAvailable = computed(() => {
  const multiplier = props.account.multiplier
  return multiplier.value != null
    && Number.isFinite(multiplier.value)
    && (multiplier.source === 'manual' || multiplier.status === 'ok')
})
const isOpenAIAPIKey = computed(() => props.account.platform.toLowerCase() === 'openai' && isAPIKeyAccountType(props.account.account_type))
const isOpenAINonAPIKey = computed(() => props.account.platform.toLowerCase() === 'openai' && !isOpenAIAPIKey.value)
const costValue = computed(() => {
  if (isOpenAIAPIKey.value) return multiplierAvailable.value ? formatMultiplier(props.account.multiplier.value) : '--'
  if (isOpenAINonAPIKey.value) return props.account.procurement_cost_cny != null ? `¥${props.account.procurement_cost_cny.toFixed(2)}` : '--'
  if (props.account.procurement_cost_cny != null) return `¥${props.account.procurement_cost_cny.toFixed(2)}`
  if (multiplierAvailable.value) return formatMultiplier(props.account.multiplier.value)
  return '--'
})
const costDetail = computed(() => {
  if (isOpenAIAPIKey.value) {
    if (props.account.multiplier.source === 'manual' && multiplierAvailable.value) return '手工录入倍率'
    if (multiplierAvailable.value) return '上游托管倍率'
    return '未录入账号倍率'
  }
  if (isOpenAINonAPIKey.value && props.account.procurement_cost_cny == null) return '成本待确认'
  if (props.account.procurement_cost_cny == null) {
    if (props.account.multiplier.source === 'manual') return '手工录入倍率'
    if (multiplierAvailable.value) return '上游托管倍率'
    return '未录入账号倍率'
  }
  const quota = props.account.estimated_usable_quota_usd
  if (quota == null || !Number.isFinite(quota) || quota <= 0) return '成本待确认'
  return `预计可用额度 ${quota.toFixed(0)} USD · 预计成本倍率 ${formatMultiplier(props.account.procurement_cost_cny / quota)}`
})
const balanceValue = computed(() => {
  const balance = props.account.balance
  if (balance?.value_usd != null && Number.isFinite(balance.value_usd)) return `$${balance.value_usd.toFixed(2)}`
  return '暂不可用'
})
const balanceDetail = computed(() => {
  const balance = props.account.balance
  if (!balance) return '暂无余额快照'
  if (balance.status === 'stale' || balance.status === 'failed') return '数据延迟'
  if (balance.source) return `来源：${balance.source}`
  return '余额快照'
})
function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return '--'
  return new Intl.NumberFormat('zh-CN', { style: 'percent', maximumFractionDigits: 1 }).format(value)
}
function formatMs(value?: number | null): string {
	if (value == null || !Number.isFinite(value)) return '--'
	return `${Math.round(value)} ms`
}
function parseFiniteNumber(value: unknown, allowNegative = false): number | null {
	if (typeof value !== 'number' && typeof value !== 'string') return null
	if (typeof value === 'string' && !/^[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?$/.test(value.trim())) return null
	const parsed = typeof value === 'number' ? value : Number(value.trim())
  if (!Number.isFinite(parsed) || (!allowNegative && parsed < 0)) return null
  return parsed
}
function formatMultiplier(value?: string | number | null): string {
  const parsed = parseFiniteNumber(value)
  if (parsed == null) return '--'
  return `${parsed.toFixed(2)}×`
}
function isAPIKeyAccountType(value?: string | null): boolean {
  return value?.toLowerCase().replace(/[-_]/g, '') === 'apikey'
}
function formatDateTime(value?: string | null): string {
  if (!value) return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '--'
  return new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', year: 'numeric', month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false }).format(date)
}
function formatShortTime(value?: string | null): string {
	if (!value) return '--'
	const date = new Date(value)
	if (Number.isNaN(date.getTime())) return '--'
	return new Intl.DateTimeFormat('zh-CN', { timeZone: 'Asia/Shanghai', hour: '2-digit', minute: '2-digit', hour12: false }).format(date)
}
function formatNumber(value: number): string {
  return new Intl.NumberFormat('zh-CN').format(Math.max(0, Number(value) || 0))
}
function formatMetricNumber(value: number): string {
  if (!Number.isFinite(value)) return '--'
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 1 }).format(value)
}
function formatWeightKey(key: string): string {
  return ({ quality: '质量', cost: '成本', success: '成功', ttft: '首 Token', latency: '延迟' }[key] ?? key)
}
function isCompletedProbe(status: string): boolean {
  return ['success', 'operational', 'ok', 'unavailable', 'model_unavailable', 'degraded'].includes(status)
}
function isFailedProbe(status: string): boolean {
  return ['failed', 'error'].includes(status)
}
function latencyBarHeight(value?: number | null): number {
  if (value == null || !Number.isFinite(value)) return 65
  const ratio = Math.log10(Math.max(100, value) / 100)
  return Math.round(Math.max(35, Math.min(100, 100 - ratio * 32.5)))
}
function errorMessage(reason: unknown, fallback: string): string {
  return reason instanceof Error && reason.message ? reason.message : fallback
}
function waitForPrioritySave(accountID: number, priority: number): Promise<void> {
  return new Promise((resolve, reject) => emit('updatePriority', accountID, priority, { resolve, reject }))
}
async function beginPriorityEdit() {
  draftPriority.value = String(displayedPriority.value)
  priorityError.value = ''
  editingPriority.value = true
  await nextTick()
  priorityInput.value?.focus()
  priorityInput.value?.select()
}
function cancelPriorityEdit() {
  if (savingPriority.value) return
  draftPriority.value = String(displayedPriority.value)
  priorityError.value = ''
  editingPriority.value = false
}
async function savePriority() {
  const priority = Number(draftPriority.value)
  if (!Number.isInteger(priority) || priority < 1) {
    priorityError.value = '请输入大于或等于 1 的整数'
    await nextTick()
    priorityInput.value?.focus()
    return
  }
  if (priority === displayedPriority.value) {
    editingPriority.value = false
    return
  }
  savingPriority.value = true
  priorityError.value = ''
  let shouldRefocus = false
  try {
    await waitForPrioritySave(props.account.account_id, priority)
    displayedPriority.value = priority
    editingPriority.value = false
  } catch (reason) {
    priorityError.value = errorMessage(reason, '保存全局优先级失败')
    shouldRefocus = true
  } finally {
    savingPriority.value = false
    if (shouldRefocus) {
      await nextTick()
      priorityInput.value?.focus()
    }
  }
}

const MetricCell = defineComponent({
  name: 'AccountMonitorMetricCell',
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    detail: { type: String, required: true },
    tone: { type: String, required: true },
  },
  setup(metricProps, { attrs }) {
    const toneClass: Record<string, string> = {
      success: 'border-emerald-200 bg-emerald-50 dark:border-emerald-900/50 dark:bg-emerald-950/20',
      ttft: 'border-blue-200 bg-blue-50 dark:border-blue-900/50 dark:bg-blue-950/20',
      latency: 'border-amber-200 bg-amber-50 dark:border-amber-900/50 dark:bg-amber-950/20',
    }
    return () => h('div', { ...attrs, class: ['min-h-[116px] min-w-0 rounded-lg border p-3 service-metric', toneClass[metricProps.tone], attrs.class] }, [
      h('div', { class: 'text-[11px] text-gray-500 dark:text-slate-400' }, metricProps.label),
      h('div', { class: 'mt-1 whitespace-nowrap font-mono text-lg font-semibold text-gray-900 dark:text-white', 'data-test': `${metricProps.tone}-metric-value` }, metricProps.value),
      h('p', { class: 'mt-1 text-[10px] leading-4 text-gray-400 dark:text-slate-500' }, metricProps.detail),
    ])
  },
})

const CostMetric = defineComponent({
  name: 'AccountMonitorCostMetric',
  props: { label: { type: String, required: true }, value: { type: String, required: true } },
  setup(metricProps) {
    return () => h('div', { class: 'rounded bg-gray-50 p-2 dark:bg-slate-900/70' }, [
      h('div', { class: 'text-[11px] text-gray-500 dark:text-slate-400' }, metricProps.label),
      h('div', { class: 'mt-1 break-words font-mono text-xs font-semibold text-gray-900 dark:text-white' }, metricProps.value),
    ])
  },
})
</script>
