<template>
  <article class="overflow-hidden rounded-lg border border-l-4 border-gray-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-950" :class="statusBorderClass" data-test="monitor-card">
    <header class="flex items-start justify-between gap-3 border-b border-gray-100 px-[18px] py-4 dark:border-slate-800 max-[430px]:px-[14px] max-[430px]:py-[14px]" :class="statusHeaderClass" data-test="monitor-card-header">
      <div class="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <h2 class="break-words text-base font-semibold text-gray-900 dark:text-white [overflow-wrap:anywhere]" data-test="account-identity">
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
        <div class="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[10px] leading-4 text-gray-500 dark:text-slate-400" data-test="account-metadata">
          <span>平台 {{ platformLabel }}</span>
          <span aria-hidden="true">/</span>
          <span class="min-w-0 break-words">当前分组 {{ currentGroupLabel }}</span>
          <span aria-hidden="true">/</span>
          <span>调度状态 {{ schedulableLabel }}</span>
          <template v-if="recommendation">
            <span aria-hidden="true">/</span>
            <HelpTooltip v-if="formalMigration" class="!ml-0" width-class="w-80" data-test="recommendation-tooltip-trigger">
              <template #trigger>
                <span class="inline-flex items-center gap-0.5 text-amber-600 dark:text-amber-300" data-test="group-recommendation">
                  <span>{{ recommendationLabel }}</span>
                  <button
                    class="inline-flex h-5 w-5 items-center justify-center transition-colors hover:text-amber-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-500 dark:hover:text-amber-200"
                    data-test="recommendation-warning"
                    type="button"
                    :title="recommendationTooltip"
                    :aria-label="recommendationTooltip"
                  >!</button>
                </span>
              </template>
              <div data-test="group-recommendation-tooltip">{{ recommendationTooltip }}</div>
            </HelpTooltip>
            <HelpTooltip v-else-if="isNotRecommended" class="!ml-0" trigger="hover-click" :reset-key="recommendationResetKey" data-test="recommendation-reason-trigger">
              <template #trigger>
                <button
                  class="inline-flex items-center gap-0.5 text-red-600 transition-colors hover:text-red-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-red-500 dark:text-red-300 dark:hover:text-red-200"
                  data-test="recommendation-reason-button"
                  type="button"
                  :title="`${recommendationLabel}，${recommendationReasonHint}`"
                  :aria-label="`${recommendationLabel}，${recommendationReasonHint}`"
                >
                  <span data-test="group-recommendation">{{ recommendationLabel }}</span>
                  <Icon name="infoCircle" size="xs" />
                </button>
              </template>
              <div data-test="group-recommendation-reason" class="line-clamp-2 max-w-[min(16rem,calc(100vw-1.5rem))] whitespace-normal break-words leading-5">{{ recommendationReasonHint }}</div>
            </HelpTooltip>
            <span v-else data-test="group-recommendation" :class="recommendationTextClass">{{ recommendationLabel }}</span>
          </template>
        </div>
      </div>
      <div class="flex shrink-0 flex-wrap items-center justify-end gap-1">
        <button
          class="rounded px-1.5 py-1 text-[11px] text-gray-500 transition-colors hover:bg-white/70 hover:text-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-slate-400 dark:hover:bg-slate-900/70 dark:hover:text-slate-100"
          data-test="account-info"
          type="button"
          title="查看账号信息"
          aria-label="查看账号信息"
          @click="emit('accountInfo', account)"
        >账号信息</button>
        <button
          class="rounded px-1.5 py-1 text-[11px] text-gray-500 transition-colors hover:bg-white/70 hover:text-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-slate-400 dark:hover:bg-slate-900/70 dark:hover:text-slate-100"
          data-test="account-edit"
          type="button"
          title="编辑账号"
          aria-label="编辑账号"
          @click="emit('accountEdit', account)"
        >编辑</button>
        <button
          class="rounded px-1.5 py-1 text-[11px] text-gray-500 transition-colors hover:bg-white/70 hover:text-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-slate-400 dark:hover:bg-slate-900/70 dark:hover:text-slate-100"
          data-test="account-delete"
          type="button"
          title="删除账号"
          aria-label="删除账号"
          @click="emit('accountDelete', account)"
        >删除</button>
        <button
          class="rounded px-1.5 py-1 text-[11px] text-gray-500 transition-colors hover:bg-white/70 hover:text-gray-800 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-slate-400 dark:hover:bg-slate-900/70 dark:hover:text-slate-100"
          data-test="account-more"
          type="button"
          title="更多账号操作"
          aria-label="更多账号操作"
          @click="emit('accountMore', account, $event)"
        >更多</button>
        <span class="rounded-full px-2 py-1 text-xs font-semibold" :class="statusBadgeClass" data-test="status-badge">
          {{ statusLabel }}
        </span>
      </div>
    </header>

    <div class="px-[18px] pb-0 pt-4 max-[430px]:px-[14px]">
      <section class="grid grid-cols-3 overflow-hidden rounded-lg border border-gray-200 bg-gray-50 divide-x divide-gray-200 dark:divide-slate-800 dark:border-slate-800 dark:bg-slate-900/50" aria-label="评分排名与优先级">
        <div class="min-h-[121px] min-w-0 p-[14px] max-[430px]:min-h-[114px] max-[430px]:px-2 max-[430px]:py-[11px]" data-test="score-metric" :title="scoreTooltip" :aria-label="scoreTooltip">
          <HelpTooltip class="!ml-0 w-full" trigger="click" width-class="w-80" data-test="score-tooltip-trigger">
            <template #trigger>
              <button class="w-full cursor-help text-left" type="button" :title="scoreTooltip" :aria-label="scoreTooltip">
                <div class="text-[11px] text-gray-500 dark:text-slate-400">{{ scoreTitle }}</div>
                <div class="mt-1 flex items-baseline gap-1.5"><strong class="font-mono text-2xl font-semibold text-gray-900 dark:text-white">{{ scoreLabel }}</strong><span class="text-xs font-semibold text-gray-500 dark:text-slate-400">/ 100</span></div>
                <p class="mt-2 text-[10px] text-gray-500 dark:text-slate-400">{{ evidenceDetail }}</p>
              </button>
            </template>
            <div data-test="score-breakdown-tooltip">{{ scoreTooltip }}</div>
          </HelpTooltip>
        </div>
        <div class="min-h-[121px] min-w-0 p-[14px] max-[430px]:min-h-[114px] max-[430px]:px-2 max-[430px]:py-[11px]" data-test="rank-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">{{ rankTitle }}</div>
          <div class="mt-1 flex min-h-8 items-baseline gap-1.5"><strong class="truncate font-mono text-2xl font-semibold text-gray-900 dark:text-white">{{ rankLabel }}</strong><span v-if="ranked" class="shrink-0 text-xs font-semibold text-gray-500 dark:text-slate-400">/ {{ rankedAccountCount }}</span></div>
          <p class="mt-1 text-[10px] text-gray-400 dark:text-slate-500">正常可用账号继续参与排名</p>
        </div>
        <div class="min-h-[121px] min-w-0 p-[14px] max-[430px]:min-h-[114px] max-[430px]:px-2 max-[430px]:py-[11px]" data-test="priority-control">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">全局优先级</div>
          <template v-if="editingPriority">
            <div class="mt-1 flex h-8 items-center gap-1">
              <label class="sr-only" :for="`account-priority-${account.account_id}`">全局优先级</label>
              <input
                :id="`account-priority-${account.account_id}`"
                ref="priorityInput"
                v-model="draftPriority"
                class="input h-8 min-w-0 flex-1 px-2 py-1 font-mono text-sm"
                data-test="priority-input"
                inputmode="numeric"
                min="1"
                step="1"
                type="number"
                :disabled="savingPriority"
                @keyup.enter="savePriority"
                @keyup.esc="cancelPriorityEdit"
              >
              <button class="icon-button h-8 w-8 shrink-0" data-test="save-priority" type="button" title="保存全局优先级" aria-label="保存全局优先级" :disabled="savingPriority" @click="savePriority"><Icon name="check" size="xs" /></button>
              <button class="icon-button h-8 w-8 shrink-0" data-test="cancel-priority" type="button" title="取消编辑全局优先级" aria-label="取消编辑全局优先级" :disabled="savingPriority" @click="cancelPriorityEdit"><Icon name="x" size="xs" /></button>
            </div>
            <p v-if="priorityError" class="mt-1 text-[11px] text-red-600 dark:text-red-400" data-test="priority-error" role="alert">{{ priorityError }}</p>
          </template>
          <template v-else>
            <div class="mt-1 flex h-8 items-center gap-1">
              <strong class="font-mono text-2xl text-gray-900 dark:text-white">{{ displayedPriority }}</strong>
              <button class="icon-button h-8 w-8 shrink-0" data-test="edit-priority" type="button" title="编辑全局优先级" aria-label="编辑全局优先级" @click="beginPriorityEdit"><Icon name="edit" size="xs" /></button>
            </div>
            <p class="mt-1 text-[10px] text-gray-400 dark:text-slate-500">数值越小，调度优先级越高</p>
          </template>
        </div>
      </section>

      <section class="mt-4 grid grid-cols-1 gap-2 sm:grid-cols-2 xl:grid-cols-6" aria-label="账号服务指标">
        <MetricCell data-test="success-rate-metric" tone="success" label="探测成功率" :value="formatPercent(probeSuccessRate)" :detail="`${formatNumber(probeSampleCount)} 次探测样本，${formatNumber(probeFailureCount)} 次失败`" />
        <MetricCell data-test="ttft-metric" tone="ttft" label="首 Token 延迟 P50" :value="formatMs(probeTTFTP50MS)" :detail="sampleDetail(probeSuccessCount)" />
        <MetricCell data-test="latency-metric" tone="latency" label="完整响应耗时 P95" :value="formatMs(probeLatencyP95MS)" :detail="sampleDetail(probeSuccessCount)" />
        <div class="min-h-[116px] min-w-0 rounded-lg border border-violet-200 bg-violet-50 p-3 service-metric dark:border-violet-900/50 dark:bg-violet-950/20" data-test="cost-metric">
          <div class="flex items-center justify-between gap-2 text-[11px] text-gray-500 dark:text-slate-400">
            <HelpTooltip class="!ml-0" trigger="click" width-class="w-72" data-test="cost-tooltip-trigger">
              <template #trigger>
                <button class="cursor-help text-left" type="button" :title="costSourceTooltip" :aria-label="costSourceTooltip">账号成本<span v-if="manualCost" class="ml-1 font-bold text-amber-500 dark:text-amber-300" data-test="manual-cost-warning">!</span></button>
              </template>
              <div data-test="cost-source-tooltip">{{ costSourceTooltip }}</div>
            </HelpTooltip>
          </div>
          <div class="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white">{{ costValue }}</div>
          <p class="mt-1 text-[10px] leading-4 text-gray-400 dark:text-slate-500" data-test="cost-detail">{{ costDetail }}</p>
          <div class="mt-2 flex items-center gap-1" data-test="cost-actions">
            <button class="icon-button h-8 w-8" data-test="edit-cost" type="button" title="编辑账号成本" aria-label="编辑账号成本" @click="emit('editCost', account)"><Icon name="edit" size="xs" /></button>
          </div>
        </div>
        <div v-if="isOpenAIAPIKey" class="min-h-[116px] min-w-0 rounded-lg border border-cyan-200 bg-cyan-50 p-3 service-metric dark:border-cyan-900/50 dark:bg-cyan-950/20" data-test="balance-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">上游余额</div>
          <div class="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white">{{ balanceValue }}</div>
          <p class="mt-1 text-[10px] leading-4 text-gray-400 dark:text-slate-500">{{ balanceDetail }}</p>
        </div>
        <div class="min-h-[116px] min-w-0 rounded-lg border border-gray-200 bg-gray-50 p-3 service-metric dark:border-slate-800 dark:bg-slate-900/50" data-test="concurrency-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">当前并发</div>
          <div class="mt-1 font-mono text-lg font-semibold text-gray-900 dark:text-white">{{ concurrencyValue }}</div>
          <p class="mt-1 text-[10px] text-gray-400 dark:text-slate-500">{{ concurrency?.delayed ? '数据延迟' : '近实时运维快照' }}</p>
        </div>
      </section>

      <section class="mt-4" data-test="equivalent-cost-multiplier">
        <CostMetric label="成本折合本站倍率" :value="formatMultiplier(account.equivalent_site_multiplier)" />
      </section>

      <section class="mt-4 border-t border-gray-100 py-4 dark:border-slate-800" aria-label="近期探测" data-test="probe-section">
        <div class="flex items-center justify-between gap-3">
          <div class="flex items-center gap-2">
            <h3 class="text-sm font-semibold text-gray-800 dark:text-slate-100">近期探测</h3>
            <button type="button" class="text-[11px] text-primary-600 hover:underline dark:text-primary-300" data-test="edit-connection-probe-model" @click="openModelDetectionDialog">{{ t('admin.accounts.modelDetection.editConnectionProbeModel') }}</button>
          </div>
          <span class="text-[11px] text-gray-500 dark:text-slate-400" data-test="probe-summary">{{ probeSummary }}</span>
        </div>
        <div class="mt-3 grid h-9 grid-cols-[repeat(24,minmax(3px,1fr))] items-end gap-1" role="img" :aria-label="timelineAriaLabel">
          <span
            v-for="(bar, index) in probeBars"
            :key="`${account.account_id}-${index}`"
            class="min-w-0 rounded-sm transition-[height,background-color] duration-200 motion-reduce:transition-none"
            :class="bar.colorClass"
            :style="{ height: `${bar.height}%` }"
            :title="bar.title"
            data-test="probe-bar"
            aria-hidden="true"
          />
        </div>
        <div class="mt-1 flex justify-between text-[10px] text-gray-400 dark:text-slate-500"><span>较早</span><span>最近</span></div>
      </section>

      <section class="border-t border-gray-100 py-3 dark:border-slate-800" data-test="model-detection-section">
        <button type="button" class="flex min-h-10 w-full items-center gap-2 text-left text-xs" data-test="model-detection-status-row" :aria-expanded="modelDetectionDialogOpen" @click="openModelDetectionDialog">
          <span class="font-semibold text-gray-700 dark:text-slate-200">{{ t('admin.accounts.modelDetection.section') }}</span>
          <span class="rounded-full px-2 py-0.5" :class="modelDetectionStatusClass">{{ modelDetectionStatusLabel }}</span>
          <span class="min-w-0 flex-1 truncate text-gray-500 dark:text-slate-400">{{ modelDetectionStatusHint }}</span>
          <Icon name="chevronDown" size="xs" />
        </button>
      </section>

      <section class="border-t border-gray-100 dark:border-slate-800" data-test="calls-disclosure">
        <button
          class="flex min-h-12 w-full items-center gap-2 text-left text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500"
          data-test="calls-toggle"
          type="button"
          :aria-controls="callsPanelID"
          :aria-expanded="callsExpanded"
          @click="callsExpanded = !callsExpanded"
        >
          <span class="font-semibold text-gray-800 dark:text-slate-100">{{ callsTitle }}</span>
          <span class="text-[11px] text-gray-500 dark:text-slate-400">{{ callsSummary }}</span>
          <Icon name="chevronDown" size="xs" class="ml-auto transition-transform motion-reduce:transition-none" :class="{ 'rotate-180': callsExpanded }" />
        </button>
        <div v-if="callsExpanded" :id="callsPanelID" class="grid grid-cols-2 gap-2 border-t border-gray-100 pb-3 pt-3 dark:border-slate-800">
          <div class="rounded-md bg-gray-50 p-2.5 dark:bg-slate-900/50"><div class="text-[10px] text-gray-500 dark:text-slate-400">成功请求</div><div class="mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ successfulRequestCount }}</div></div>
          <div class="rounded-md bg-gray-50 p-2.5 dark:bg-slate-900/50"><div class="text-[10px] text-gray-500 dark:text-slate-400">失败请求</div><div class="mt-1 font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ account.error_count }}</div></div>
        </div>
      </section>

      <footer class="flex min-h-[52px] items-center justify-between gap-3 border-t border-gray-100 text-[11px] text-gray-500 dark:border-slate-800 dark:text-slate-400 max-[430px]:flex-col max-[430px]:items-start max-[430px]:gap-[3px] max-[430px]:py-[9px]" data-test="card-footer">
        <span>检查于 {{ checkedAtLabel }} · 统计截止 {{ statisticsCutoffLabel }}</span>
        <button class="icon-button shrink-0 max-[430px]:self-end" data-test="refresh-account" type="button" title="刷新当前账号" aria-label="刷新当前账号" :disabled="running" @click="emit('refresh', account.account_id)"><Icon name="refresh" size="sm" :class="{ 'animate-spin': running }" /></button>
      </footer>
      <AccountModelDetectionDialog
        :show="modelDetectionDialogOpen"
        :account="account"
        :models="modelDetectionModels"
        :saving="savingModelDetection"
        :detecting="detectingModelDetection"
        @close="modelDetectionDialogOpen = false"
        @save="emit('saveModelDetectionModels', account.account_id, $event)"
        @detect="emit('detectModelDetection', account.account_id)"
      />
    </div>
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
}>(), { concurrency: null, running: false, rankedAccountCount: 0, rankingScope: 'group', statisticsCutoff: null, selectedRange: '24h' })
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

const platformLabel = computed(() => props.account.platform || '--')
const currentGroupLabel = computed(() => props.account.group_names?.filter(Boolean).join('、') || '--')
const schedulableLabel = computed(() => props.account.status !== 'active' ? '暂停' : props.account.schedulable ? '可调度' : '不可调度')
const recommendation = computed<AccountMonitorGroupRecommendation | null>(() => props.account.group_recommendation ?? null)
const recommendationIdentityRevision = ref(0)
const isTestGroup = computed(() => props.account.group_names?.some((name) => name.trim().toLowerCase().replace(/ /g, '') === 'gpt-测试分组') ?? false)
const formalMigration = computed(() => recommendation.value?.action === 'migrate' && !isTestGroup.value)
const isNotRecommended = computed(() => recommendation.value?.status === 'not_recommended')
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
watch(recommendation, (next, previous) => {
  if (next === previous) return
  if (next?.status === 'not_recommended' || previous?.status === 'not_recommended') {
    recommendationIdentityRevision.value += 1
  }
})
const recommendationResetKey = computed(() => isNotRecommended.value ? `${recommendationIdentityRevision.value}|${recommendation.value?.reason_codes?.[0] ?? ''}|${recommendation.value?.observed_at ?? ''}` : '')
const recommendationReason = computed(() => {
  const code = recommendation.value?.reason_codes?.[0]
  return ({
    codex_auth_default_pro: 'Codex Auth 默认进入 Pro',
    sample_insufficient: '主动探测样本不足',
    profit_below_minimum: '成本超过该分组利润下限',
    auth_failed: '认证失败',
    balance_unavailable: '余额不可用',
    quota_unavailable: '配额不可用',
    model_unavailable: '模型不可用',
    success_rate_below_pro: '探测成功率未达到 Pro 门槛',
    success_rate_below_plus: '探测成功率未达到 Plus 门槛',
    success_rate_below_special: '探测成功率未达到特惠门槛',
    ttft_exceeds_target: '首 Token 延迟超过目标',
    latency_exceeds_limit: '完整响应耗时超过目标',
  }[code ?? ''] ?? '主动探测质量不满足目标')
})
const recommendationReasonHint = computed(() => isNotRecommended.value ? `原因：${recommendationReason.value}` : '')
const recommendationTooltip = computed(() => {
  const item = recommendation.value
  if (!item) return ''
  return `推荐迁移至 ${recommendationTargetLabel.value}。原因：${recommendationReason.value}。依据：固定 7d 主动探测 ${formatNumber(item.sample_count)} 次，观察于 ${formatDateTime(item.observed_at)}。`
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
const statusBorderClass = computed(() => ({
  'border-emerald-500': statusLabel.value === '正常',
  'border-amber-500': statusLabel.value === '待确认' || statusLabel.value === '异常',
  'border-gray-300 dark:border-slate-700': statusLabel.value === '暂停',
  'border-red-500': statusLabel.value === '不可用',
}))
const statusHeaderClass = computed(() => ({
  'border-emerald-100 bg-emerald-50 dark:border-emerald-900/50 dark:bg-emerald-950/20': statusLabel.value === '正常',
  'border-amber-100 bg-amber-50 dark:border-amber-900/50 dark:bg-amber-950/20': statusLabel.value === '待确认' || statusLabel.value === '异常',
  'bg-gray-50 dark:bg-slate-900/50': statusLabel.value === '暂停',
  'border-red-100 bg-red-50 dark:border-red-900/50 dark:bg-red-950/20': statusLabel.value === '不可用',
}))
const evidenceDetail = computed(() => {
  if (props.account.evidence_source === 'monitor_probe') return `基于 ${formatNumber(props.account.sample_count)} 次主动探测`
  if (props.account.evidence_source === 'stale') return '暂无有效主动探测证据'
  return '等待主动探测证据'
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
  const breakdown = props.account.score_breakdown
  if (!breakdown || props.account.quality_score == null) return '暂无足够主动探测证据，无法计算评分构成'
  return `评分构成：成本优势 ${formatScorePart(breakdown.cost)}，探测成功率 ${formatScorePart(breakdown.success)}，TTFT ${formatScorePart(breakdown.ttft)}，总耗时 ${formatScorePart(breakdown.latency)}，合计 ${scoreLabel.value} / 100`
})
const scoreEligible = computed(() => props.account.score_status
  ? ['eligible', 'capped'].includes(props.account.score_status)
  : props.account.eligible === true)
const ranked = computed(() => scoreEligible.value && props.account.group_rank != null)
const scoreLabel = computed(() => {
  if (!scoreEligible.value || props.account.quality_score == null || !Number.isFinite(props.account.quality_score)) return '--'
  const value = props.account.score_status === 'capped' ? Math.min(props.account.quality_score, 70) : props.account.quality_score
  return new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(value)
})
const rankLabel = computed(() => ranked.value ? `第 ${props.account.group_rank}` : '未排名')
const scoreTitle = computed(() => props.rankingScope === 'global' ? '账号服务评分' : '账号分组评分')
const rankTitle = computed(() => props.rankingScope === 'global' ? '全站排名' : '组内排名')
const concurrencyValue = computed(() => props.concurrency ? `${props.concurrency.current} / ${props.concurrency.limit}` : '-- / --')
const callsPanelID = computed(() => `account-calls-${props.account.account_id}`)
const callsTitle = computed(() => ({ '24h': '24 小时调用', '7d': '7 天调用', '30d': '30 天调用' }[props.selectedRange]))
const callsSummary = computed(() => `${formatNumber(props.account.request_count)} 次请求 · ${formatNumber(props.account.error_count)} 次失败`)
const successfulRequestCount = computed(() => Math.max(0, Number(props.account.request_count) - Number(props.account.error_count)))
const probeSampleCount = computed(() => props.account.probe_sample_count ?? props.account.sample_count ?? 0)
const probeSuccessCount = computed(() => props.account.probe_success_count ?? props.account.success_sample_count ?? 0)
const probeSuccessRate = computed(() => props.account.probe_success_rate ?? props.account.success_rate ?? 0)
const probeTTFTP50MS = computed(() => props.account.probe_ttft_p50_ms ?? props.account.ttft_p50_ms ?? null)
const probeLatencyP95MS = computed(() => props.account.probe_latency_p95_ms ?? props.account.latency_p95_ms ?? null)
const probeFailureCount = computed(() => Math.max(0, probeSampleCount.value - probeSuccessCount.value))
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
      bars.push({ colorClass: 'bg-red-500 dark:bg-red-400', height: 28, title: `${timestamp} · 探测失败` })
    } else {
      bars.push({ colorClass: 'bg-gray-200 dark:bg-slate-700', height: 15, title: `${timestamp} · 暂无结果` })
    }
  }
  return bars
})
const probeSummary = computed(() => {
  return `${formatNumber(probeSampleCount.value)} 次结果 · ${formatNumber(probeSuccessCount.value)} 成功 · ${formatNumber(probeFailureCount.value)} 失败`
})
const timelineAriaLabel = computed(() => `近期 ${probeSummary.value}探测`)
const modelDetectionStatus = computed(() => props.account.model_detection?.status ?? 'untested')
const modelDetectionStatusLabel = computed(() => t(`admin.accounts.modelDetection.status.${modelDetectionStatus.value}`))
const modelDetectionStatusHint = computed(() => {
  if (modelDetectionStatus.value === 'service_unconfigured') return t('admin.accounts.modelDetection.detectorUnconfigured')
  if (modelDetectionStatus.value === 'service_unavailable') return t('admin.accounts.modelDetection.detectorUnavailable')
  return modelDetectionStatus.value === 'abnormal' ? t('admin.accounts.modelDetection.observedAbnormal') : props.account.model_detection?.recent?.error_message ?? t('admin.accounts.modelDetection.viewRecent')
})
const modelDetectionStatusClass = computed(() => ({ 'bg-emerald-100 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300': modelDetectionStatus.value === 'normal', 'bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300': ['queued', 'running', 'abnormal', 'insufficient'].includes(modelDetectionStatus.value), 'bg-red-100 text-red-700 dark:bg-red-950/40 dark:text-red-300': ['failed', 'service_unavailable'].includes(modelDetectionStatus.value), 'bg-gray-100 text-gray-600 dark:bg-slate-800 dark:text-slate-300': ['untested', 'unsupported', 'service_unconfigured'].includes(modelDetectionStatus.value) }))
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
function formatScorePart(value?: number | null): string {
  return value == null || !Number.isFinite(value) ? '--' : value.toFixed(1)
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
function sampleDetail(count: number): string {
  return `基于 ${formatNumber(count)} 次有效响应`
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
