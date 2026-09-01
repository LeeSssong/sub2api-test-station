<template>
  <article class="monitor-card-shell" data-test="monitor-card" :data-account-id="account.account_id">
    <div class="monitor-card-layout" data-test="monitor-card-header">
      <header class="monitor-card-header">
        <section class="monitor-card-identity" data-test="identity-column" aria-label="账号身份与状态">
          <div class="monitor-card-eyeline">
            <span class="monitor-card-status-dot" :class="statusDotClass" aria-hidden="true" />
            <span>{{ statusLabel }} · 人工开关：{{ schedulableLabel }} · 有效调度：{{ effectiveSchedulableLabel }}<template v-if="effectiveUnschedulableReason">（{{ effectiveUnschedulableReason }}）</template> · {{ platformLabel }} · 目标组 {{ currentGroupLabel }}</span>
          </div>
          <h2 data-test="account-identity">
            <a v-if="account.homepage_url" :href="account.homepage_url" target="_blank" rel="noopener noreferrer" data-test="account-homepage-link">{{ account.name }}</a>
            <span v-else>{{ account.name }}</span>
            <span aria-hidden="true">{{ ' ' }}</span>
            <span class="account-id">#{{ account.account_id }}</span>
          </h2>
          <div class="monitor-card-meta" data-test="account-metadata">
            <span>{{ currentGroupLabel }}</span><span aria-hidden="true">·</span><span>{{ formatNumber(realEvidence?.request_count ?? account.request_count ?? 0) }} 次窗口真实请求 · 累计 {{ formatNumber(account.lifetime_real_request_count ?? 0) }} 次</span>
            <template v-if="recommendation">
              <span aria-hidden="true">·</span>
              <button v-if="formalMigration" class="monitor-card-recommendation" data-test="group-recommendation" type="button" :title="recommendationTooltip">{{ recommendationLabel }}<span data-test="recommendation-warning">!</span></button>
              <span v-else class="monitor-card-recommendation" data-test="group-recommendation" :class="recommendationTextClass">{{ recommendationLabel }}</span>
            </template>
          </div>
        </section>
        <section v-if="schedulerContext" class="monitor-card-scheduler" data-test="scheduler-column" :aria-label="schedulerRankTitle">
          <div class="scheduler-label">{{ schedulerRankTitle }}</div>
          <div class="scheduler-rank" data-test="scheduler-rank">{{ schedulerRankLabel }}<span v-if="schedulerRanked"> / {{ schedulerRankTotalLabel }}</span></div>
        </section>
      </header>
      <div class="monitor-card-body">
        <div class="monitor-card-main">
          <section class="monitor-card-metrics" data-test="key-metrics" aria-label="关键服务指标">
            <MetricCell data-test="success-rate-metric" tone="success" label="成功率" :value="realSuccessRate" />
            <MetricCell data-test="ttft-metric" tone="ttft" label="TTFT P95" :value="realTTFTP95" />
            <MetricCell data-test="profit-rate-metric" tone="profit" label="利润率" :value="profitRateLabel" />
            <MetricCell data-test="native-priority-metric" tone="native-priority" label="Sub 原生优先级" :value="String(account.priority ?? '--')" />
            <div class="service-metric multiplier-metric" data-test="upstream-multiplier-metric">
              <div class="metric-label">上游声明倍率</div>
              <button class="metric-value metric-link" type="button" title="编辑上游声明倍率" @click="emit('editCost', account)">{{ formatMultiplier(account.upstream_multiplier?.value ?? account.multiplier?.value) }}</button>
            </div>
          </section>
          <section class="monitor-card-model" data-test="model-detection-section">
            <button type="button" class="model-status" data-test="model-detection-status-row" :aria-expanded="modelDetectionDialogOpen" @click="openModelDetectionEntry"><span class="model-title">{{ t('admin.accounts.modelDetection.section') }}</span><span class="model-pill" :class="modelDetectionStatusClass">{{ modelDetectionStatusLabel }}</span><Icon name="chevronDown" size="xs" /></button>
            <button type="button" class="model-detect" data-test="detect-model-detection" :disabled="detectingModelDetection" @click="emit('detectModelDetection', account.account_id)">{{ detectingModelDetection ? t('admin.accounts.modelDetection.detecting') : t('admin.accounts.modelDetection.detectNow') }}</button>
          </section>
        </div>
        <section class="monitor-card-chart" data-test="timeline-section" aria-label="真实性能">
          <div class="chart-head">
            <h3>真实性能 · 真实请求</h3>
            <button type="button" class="chart-action" data-test="refresh-account" title="刷新主动探测状态，不生成真实请求样本" aria-label="刷新主动探测状态，不生成真实请求样本" :disabled="running" @click="emit('refresh', account.account_id)"><Icon name="refresh" size="xs" :class="{ 'animate-spin': running }" />刷新探测状态</button>
          </div>
          <div class="performance-bars" role="img" :aria-label="realTimelineAriaLabel">
            <span v-for="(bar, index) in realRequestBars" :key="`${account.account_id}-${index}`" tabindex="0" class="performance-bar" :class="bar.colorClass" :style="{ height: `${bar.height}%` }" :title="bar.title" data-test="real-request-bar" />
          </div>
        </section>
      </div>
      <footer class="monitor-card-footer" data-test="account-actions" aria-label="账号操作">
        <button class="footer-button primary" data-test="account-info" type="button" title="查看账号详情" aria-label="查看账号详情" @click="emit('accountInfo', account)"><Icon name="eye" size="xs" />账号详情</button>
        <button class="footer-button" data-test="account-more" type="button" title="账号操作" aria-label="账号操作" @click="emit('accountMore', account, $event)"><Icon name="more" size="xs" />账号操作</button>
        <button class="sr-only" data-test="account-edit" type="button" @click="emit('accountEdit', account)">编辑</button>
        <button class="sr-only" data-test="account-delete" type="button" @click="emit('accountDelete', account)">删除</button>
      </footer>
      <AccountModelDetectionDialog :show="modelDetectionDialogOpen" :account="account" :models="modelDetectionModels" :saving="savingModelDetection" :detecting="detectingModelDetection" @close="modelDetectionDialogOpen = false" @save="emit('saveModelDetectionModels', account.account_id, $event)" @detect="emit('detectModelDetection', account.account_id)" />
    </div>
    <template v-if="false">
      <section class="min-w-0" data-test="identity-column" aria-label="账号身份与状态">
        <div class="monitor-card-eyeline">
          <span class="monitor-card-status-dot h-2 w-2 shrink-0 rounded-full" :class="statusDotClass" aria-hidden="true" />
          <span>账号状态 · {{ schedulableLabel }} · {{ platformLabel }} · 目标组 {{ currentGroupLabel }}</span>
        </div>
        <div class="flex min-w-0 items-start gap-2">
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
              <span>当前分组：{{ currentGroupLabel }}</span><span aria-hidden="true">·</span><span>基于 {{ formatNumber(realEvidence?.request_count ?? account.request_count ?? 0) }} 次已持久化真实请求</span><span aria-hidden="true">·</span><span>数据累计至当前</span>
              <template v-if="recommendation">
                <span aria-hidden="true">·</span>
                <HelpTooltip v-if="formalMigration" class="!ml-0" width-class="w-80" data-test="recommendation-tooltip-trigger">
                  <template #trigger><button class="monitor-card-recommendation" data-test="group-recommendation" type="button" :title="recommendationTooltip">{{ recommendationLabel }}<span data-test="recommendation-warning">!</span></button></template>
                  <div data-test="group-recommendation-tooltip">{{ recommendationTooltip }}</div>
                </HelpTooltip>
                <span v-else class="monitor-card-recommendation" data-test="group-recommendation" :class="recommendationTextClass">{{ recommendationLabel }}</span>
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
          <p v-if="false" class="mt-1 text-[10px] text-gray-500 dark:text-slate-400" data-test="score-evidence-detail">{{ evidenceDetail }}</p>
        </div>
        <div class="mt-3" data-test="rank-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400">{{ qualityRankTitle }}</div>
          <div class="mt-0.5 flex min-w-0 items-baseline gap-1.5"><strong class="truncate font-mono text-lg font-semibold text-gray-900 dark:text-white" data-test="quality-rank">{{ qualityRankLabel }}<span v-if="qualityRanked" class="text-xs font-normal text-gray-500 dark:text-slate-400"> / {{ qualityRankTotalLabel }}</span></strong></div>
        </div>
      </section>

      <section v-if="schedulerContext" class="min-w-0 border-l border-gray-100 pl-4 dark:border-slate-800" data-test="scheduler-column" aria-label="调度优先级">
        <div class="text-[11px] text-gray-500 dark:text-slate-400">调度优先级</div>
        <div class="mt-1 flex min-w-0 items-baseline gap-1.5"><strong class="truncate font-mono text-lg font-semibold text-gray-900 dark:text-white" data-test="scheduler-rank">{{ schedulerRankLabel }}<span v-if="schedulerRanked" class="text-xs font-normal text-gray-500 dark:text-slate-400"> / {{ schedulerRankTotalLabel }}</span></strong></div>
        <p v-if="false" class="mt-1 break-words text-[10px] text-gray-500 dark:text-slate-400" data-test="scheduler-policy">{{ schedulerPolicyLabel }}</p>
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
        <MetricCell data-test="success-rate-metric" tone="success" label="成功率" :value="realSuccessRate" detail="真实请求" />
        <MetricCell data-test="ttft-metric" tone="ttft" label="TTFT P95" :value="`${realTTFTP95} · P50 ${formatMs(probeTTFTP50MS)}`" detail="真实成功请求" />
        <MetricCell data-test="latency-metric" tone="latency" label="完整响应 P95" :value="formatMs(probeLatencyP95MS)" detail="成功响应" />
        <div data-test="profit-rate-metric" class="metric-extra rounded-lg border border-violet-200 bg-violet-50 p-3 dark:border-violet-900/50 dark:bg-violet-950/20"><div class="text-[11px] text-gray-500 dark:text-slate-400">利润率</div><div class="mt-1 font-mono text-lg font-semibold">{{ profitRateLabel }}</div><p class="mt-1 text-[10px]">当前分组</p></div>
        <div data-test="native-priority-metric" class="metric-extra rounded-lg border border-slate-200 bg-slate-50 p-3 dark:border-slate-700 dark:bg-slate-900/50"><div class="text-[11px] text-gray-500 dark:text-slate-400">Sub 原生优先级</div><div class="mt-1 font-mono text-lg font-semibold">{{ account.priority ?? '--' }}</div></div>
        <div data-test="upstream-multiplier-metric" class="metric-extra rounded-lg border border-cyan-200 bg-cyan-50 p-3 dark:border-cyan-900/50 dark:bg-cyan-950/20"><div class="text-[11px] text-gray-500 dark:text-slate-400">上游声明倍率</div><div class="mt-1 font-mono text-lg font-semibold">{{ formatMultiplier(account.upstream_multiplier?.value ?? account.multiplier?.value) }}</div><p class="mt-1 text-[10px]">可编辑</p></div>
        <div class="service-metric min-w-0 border-l border-violet-200 bg-violet-50 px-2 pl-3 dark:border-violet-900/50 dark:bg-violet-950/20" data-test="cost-metric">
          <div class="text-[11px] text-gray-500 dark:text-slate-400"><HelpTooltip class="!ml-0" trigger="click" width-class="w-72" data-test="cost-tooltip-trigger"><template #trigger><button class="cursor-help text-left" type="button" :title="costSourceTooltip" :aria-label="costSourceTooltip">账号成本<span v-if="manualCost" class="ml-1 font-bold text-amber-500 dark:text-amber-300" data-test="manual-cost-warning">!</span></button></template><div data-test="cost-source-tooltip">{{ costSourceTooltip }}</div></HelpTooltip></div>
          <div class="mt-1 break-words font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ costValue }}</div>
          <p v-if="false" class="mt-1 break-words text-[10px] leading-4 text-gray-400 dark:text-slate-500" data-test="cost-detail">{{ costDetail }}</p>
          <div class="mt-1 flex items-center gap-1" data-test="cost-actions"><button class="icon-button h-7 w-7" data-test="edit-cost" type="button" title="编辑账号成本" aria-label="编辑账号成本" @click="emit('editCost', account)"><Icon name="edit" size="xs" /></button></div>
        </div>
        <div v-if="false && isOpenAIAPIKey" class="min-w-0 border-l border-cyan-200 bg-cyan-50/70 px-2 pl-3" data-test="balance-metric"><div>上游余额</div><div>{{ balanceValue }}</div></div>
        <div v-if="false" class="service-metric min-w-0" data-test="concurrency-metric"><div>当前并发</div><div>{{ concurrencyValue }}</div></div>
        <div v-if="false" class="col-span-2 min-w-0" data-test="equivalent-cost-multiplier"><CostMetric label="成本折合本站倍率" :value="formatMultiplier(account.equivalent_site_multiplier)" /></div>
      </section>

      <section class="min-w-0 border-l border-gray-100 pl-4 dark:border-slate-800" data-test="timeline-section" aria-label="近期表现">
        <div class="flex min-w-0 items-center justify-between gap-2"><h3 class="text-xs font-semibold text-gray-800 dark:text-slate-100">近期表现</h3><button type="button" class="shrink-0 text-[11px] text-primary-600 hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-primary-300" data-test="edit-connection-probe-model" @click="openModelDetectionDialog">{{ t('admin.accounts.modelDetection.editConnectionProbeModel') }}</button></div>
        <div class="mt-3 grid h-9 min-w-0 grid-cols-[repeat(24,minmax(3px,1fr))] items-end gap-1" role="img" :aria-label="realTimelineAriaLabel"><span v-for="(bar, index) in realRequestBars" :key="`${account.account_id}-${index}`" tabindex="0" class="min-w-0 rounded-sm focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500" :class="bar.colorClass" :style="{ height: `${bar.height}%` }" :title="bar.title" data-test="real-request-bar" /></div>
        <div v-if="false" class="mt-1 flex justify-between text-[10px] text-gray-400 dark:text-slate-500"><span>较早</span><span>最近</span></div>
      </section>

      <section class="flex min-w-0 flex-wrap items-start justify-end gap-1 2xl:flex-col 2xl:items-stretch" data-test="account-actions" aria-label="账号操作">
        <span class="sr-only">账号操作</span>
        <button class="icon-button h-8 w-8 2xl:w-full" data-test="account-info" type="button" title="查看账号详情" aria-label="查看账号详情" @click="emit('accountInfo', account)"><Icon name="eye" size="xs" /><span class="sr-only 2xl:not-sr-only 2xl:ml-1 2xl:text-[11px]">账号详情</span></button>
        <button class="icon-button h-8 w-8 2xl:w-full" data-test="account-edit" type="button" title="编辑账号" aria-label="编辑账号" @click="emit('accountEdit', account)"><Icon name="edit" size="xs" /><span class="sr-only 2xl:not-sr-only 2xl:ml-1 2xl:text-[11px]">编辑</span></button>
        <button class="icon-button h-8 w-8 2xl:w-full" data-test="account-delete" type="button" title="删除账号" aria-label="删除账号" @click="emit('accountDelete', account)"><Icon name="trash" size="xs" /><span class="sr-only 2xl:not-sr-only 2xl:ml-1 2xl:text-[11px]">删除</span></button>
        <button class="icon-button h-8 w-8 2xl:w-full" data-test="account-more" type="button" title="更多账号操作" aria-label="更多账号操作" @click="emit('accountMore', account, $event)"><Icon name="more" size="xs" /><span class="sr-only 2xl:not-sr-only 2xl:ml-1 2xl:text-[11px]">更多</span></button>
        <button class="icon-button h-8 w-8 2xl:w-full" data-test="refresh-account" type="button" title="刷新当前账号" aria-label="刷新当前账号" :disabled="running" @click="emit('refresh', account.account_id)"><Icon name="refresh" size="sm" :class="{ 'animate-spin': running }" /><span class="sr-only 2xl:not-sr-only 2xl:ml-1 2xl:text-[11px]">刷新</span></button>
      </section>

      <div v-if="false && schedulerContext" class="min-w-0 2xl:col-span-6">
        <div class="flex min-w-0 items-start gap-3 border-t border-gray-100 pt-3 dark:border-slate-800">
          <p v-if="rankingReason" class="min-w-0 flex-1 break-words text-[11px] leading-4 text-gray-500 dark:text-slate-400" data-test="ranking-reason">{{ rankingReason }}</p>
          <button class="inline-flex min-h-8 shrink-0 items-center gap-1 rounded-md px-2 text-xs font-medium text-gray-600 hover:bg-gray-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:text-slate-300 dark:hover:bg-slate-800" data-test="ranking-explanation-toggle" type="button" :aria-controls="rankingExplanationID" :aria-expanded="rankingExplanationExpanded" @click="rankingExplanationExpanded = !rankingExplanationExpanded"><Icon name="eye" size="xs" /><span>{{ rankingExplanationExpanded ? '收起依据' : '查看排名依据' }}</span><Icon name="chevronDown" size="xs" :class="{ 'rotate-180': rankingExplanationExpanded }" /></button>
        </div>
        <div v-if="rankingExplanationExpanded" :id="rankingExplanationID" class="grid min-w-0 gap-3 border-t border-gray-100 pt-3 text-xs dark:border-slate-800 sm:grid-cols-2 lg:grid-cols-4" data-test="ranking-explanation">
          <div class="min-w-0"><div class="font-semibold text-gray-700 dark:text-slate-200">质量构成</div><div class="mt-1 grid gap-1 text-gray-500 dark:text-slate-400"><span v-for="item in qualityBreakdownItems" :key="item.key">{{ item.label }} {{ item.score }} / {{ item.max }}</span><span v-if="!qualityBreakdownItems.length">暂无评分构成</span><span v-if="qualityExplanationSummary">{{ qualityExplanationSummary }}</span></div></div>
          <div class="min-w-0"><div class="font-semibold text-gray-700 dark:text-slate-200">调度事实</div><div class="mt-1 grid gap-1 break-words text-gray-500 dark:text-slate-400"><span>策略 {{ schedulerPolicyLabel }}</span><span>{{ schedulerEligibilityLabel }}</span><span v-if="schedulerCandidateLabel">{{ schedulerCandidateLabel }}</span><span v-if="schedulerScopeLabel">范围 {{ schedulerScopeLabel }}</span></div></div>
          <div class="min-w-0"><div class="font-semibold text-gray-700 dark:text-slate-200">快照与权重</div><div class="mt-1 grid gap-1 break-words text-gray-500 dark:text-slate-400"><span v-if="schedulerFactsLabel">权重 {{ schedulerFactsLabel }}</span><span v-if="schedulerModelQuotaParityLabel">{{ schedulerModelQuotaParityLabel }}</span><span v-if="schedulerSnapshotLabel">快照 {{ schedulerSnapshotLabel }}</span><span v-if="schedulerTieBreakLabel">并列处理 {{ schedulerTieBreakLabel }}</span><span v-if="!schedulerFactsLabel && !schedulerModelQuotaParityLabel && !schedulerSnapshotLabel && !schedulerTieBreakLabel">暂无额外调度事实</span></div></div>
          <div class="min-w-0"><div class="font-semibold text-gray-700 dark:text-slate-200">来源</div><div class="mt-1 grid gap-1 break-words text-gray-500 dark:text-slate-400"><span>评分来源 {{ qualitySourceLabel }}</span><span>评分样本 {{ qualitySampleLabel }}</span><span>评分时间 {{ qualityObservedAtLabel }}</span><span v-if="schedulerReasonLabel">原因 {{ schedulerReasonLabel }}</span></div></div>
        </div>
      </div>

      <section class="min-w-0 border-t border-gray-100 pt-3 2xl:col-span-6 dark:border-slate-800" data-test="model-detection-section">
        <button type="button" class="flex min-h-8 w-full min-w-0 items-center gap-2 text-left text-xs focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500" data-test="model-detection-status-row" :aria-expanded="modelDetectionDialogOpen" @click="openModelDetectionEntry"><span class="font-semibold text-gray-700 dark:text-slate-200">{{ t('admin.accounts.modelDetection.section') }}</span><span class="rounded-full px-2 py-0.5" :class="modelDetectionStatusClass">{{ modelDetectionStatusLabel }}</span><span class="min-w-0 flex-1 truncate text-gray-500 dark:text-slate-400">{{ modelDetectionStatusHint }}</span><Icon name="chevronDown" size="xs" /></button>
      </section>

      <section v-if="false" class="min-w-0 border-t border-gray-100 2xl:col-span-6 dark:border-slate-800" data-test="calls-disclosure">
        <button class="flex min-h-10 w-full min-w-0 items-center gap-2 text-left text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500" data-test="calls-toggle" type="button" :aria-controls="callsPanelID" :aria-expanded="callsExpanded" @click="callsExpanded = !callsExpanded"><span class="font-semibold text-gray-800 dark:text-slate-100">{{ callsTitle }}</span><span class="min-w-0 truncate text-[11px] text-gray-500 dark:text-slate-400">{{ callsSummary }}</span><Icon name="chevronDown" size="xs" class="ml-auto" :class="{ 'rotate-180': callsExpanded }" /></button>
        <div v-if="callsExpanded" :id="callsPanelID" class="grid grid-cols-2 gap-3 border-t border-gray-100 pb-1 pt-3 text-xs dark:border-slate-800"><div><div class="text-[10px] text-gray-500 dark:text-slate-400">成功请求</div><div class="mt-1 font-mono font-semibold text-gray-900 dark:text-white">{{ successfulRequestCount }}</div></div><div><div class="text-[10px] text-gray-500 dark:text-slate-400">失败请求</div><div class="mt-1 font-mono font-semibold text-gray-900 dark:text-white">{{ account.error_count }}</div></div></div>
      </section>

      <footer v-if="false" class="hidden" data-test="card-footer"></footer>
    </template>
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
const effectiveSchedulableLabel = computed(() => props.account.effective_schedulable ? '可调度' : '不可调度')
const effectiveUnschedulableReason = computed(() => ({
  inactive: '账号未激活',
  manual_disabled: '人工暂停',
  expired: '已过期',
  overload: '过载冷却',
  rate_limited: '限流冷却',
  temp_unschedulable: '临时不可调度',
  quota_exceeded: '额度耗尽',
})[props.account.effective_unschedulable_reason] ?? (props.account.effective_unschedulable_reason || ''))
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
const schedulerExplanation = computed(() => (props.account.scheduler_explanation ?? null) as SchedulerDetails | null)
const schedulerPlatformSupported = computed(() => ['openai', 'grok'].includes(props.account.platform.toLowerCase()))
const schedulerContext = computed(() => schedulerPlatformSupported.value && (schedulerExplanation.value != null || props.account.scheduler_rank != null || props.account.scheduler_unavailable === true))
const schedulerRankTitle = computed(() => props.rankingScope === 'global' ? `最佳组内调度排名${props.account.best_scheduler_group_name ? ` · ${props.account.best_scheduler_group_name}` : ''}` : '本组调度优先级')
const schedulerRanked = computed(() => schedulerContext.value && props.account.scheduler_rank != null)
const schedulerRankLabel = computed(() => schedulerRanked.value ? `第 ${props.account.scheduler_rank}` : '暂不可用')
const schedulerRankTotalLabel = computed(() => props.account.scheduler_rank_total ?? '--')
const schedulerPolicyLabel = computed(() => {
  if (!schedulerContext.value) return ''
  if (props.account.scheduler_unavailable === true) return '调度投影暂不可用'
  return schedulerExplanation.value?.policy_label || ''
})
const schedulerEligibilityLabel = computed(() => {
  if (!schedulerContext.value) return ''
  if (props.account.scheduler_unavailable === true) return '调度投影暂不可用'
  return schedulerExplanation.value ? (schedulerExplanation.value.eligible ? '符合调度条件' : '不符合调度条件') : ''
})
const schedulerCandidateLabel = computed(() => schedulerContext.value && schedulerExplanation.value?.candidate_total != null ? `候选数 ${formatNumber(schedulerExplanation.value.candidate_total)}` : '')
const schedulerScopeLabel = computed(() => schedulerContext.value ? schedulerExplanation.value?.candidate_scope || '' : '')
const schedulerSnapshotLabel = computed(() => schedulerContext.value && schedulerExplanation.value?.snapshot_at ? formatDateTime(schedulerExplanation.value.snapshot_at) : '')
const schedulerTieBreakLabel = computed(() => schedulerContext.value ? schedulerExplanation.value?.tie_break_reason || schedulerExplanation.value?.tie_break || '' : '')
const schedulerReasonLabel = computed(() => schedulerContext.value ? schedulerExplanation.value?.primary_reason_label || schedulerExplanation.value?.reason || '' : '')
const schedulerFactsLabel = computed(() => schedulerContext.value
  ? (schedulerExplanation.value?.effective_facts ?? []).map((fact) => `${fact.label} ${fact.value}`).join(' · ')
  : '')
const schedulerModelQuotaParityLabel = computed(() => {
  if (!schedulerContext.value || schedulerExplanation.value?.model_quota_parity !== 'unknown') return ''
  return '模型额度一致性未知（监控请求未指定模型）'
})
const rankingReason = computed(() => schedulerReasonLabel.value || (schedulerContext.value && props.account.scheduler_unavailable === true ? '调度投影暂不可用' : ''))
const rankingExplanationExpanded = ref(false)
const rankingExplanationID = computed(() => `account-ranking-explanation-${props.account.account_id}`)
const qualityBreakdownItems = computed<BreakdownItem[]>(() => {
  const breakdown = props.account.quality_explanation?.breakdown
  if (breakdown) {
    return [
      { key: 'cost', label: '成本', score: formatMetricNumber(breakdown.cost.score), max: formatMetricNumber(breakdown.cost.max) },
      { key: 'success', label: '成功', score: formatMetricNumber(breakdown.success.score), max: formatMetricNumber(breakdown.success.max) },
      { key: 'ttft', label: '首 Token', score: formatMetricNumber(breakdown.ttft.score), max: formatMetricNumber(breakdown.ttft.max) },
      { key: 'latency', label: props.account.quality_explanation?.experience_label || '生成体验', score: formatMetricNumber(breakdown.latency.score), max: formatMetricNumber(breakdown.latency.max) },
    ]
  }
  const legacyBreakdown = props.account.score_breakdown
  if (!legacyBreakdown) return []
  return [
    { key: 'cost', label: '成本', score: formatMetricNumber(legacyBreakdown.cost), max: '--' },
    { key: 'success', label: '成功', score: formatMetricNumber(legacyBreakdown.success), max: '--' },
    { key: 'ttft', label: '首 Token', score: formatMetricNumber(legacyBreakdown.ttft), max: '--' },
    { key: 'latency', label: props.account.quality_explanation?.experience_label || '生成体验', score: formatMetricNumber(legacyBreakdown.latency), max: '--' },
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
const concurrencyValue = computed(() => props.concurrency ? `${props.concurrency.current} / ${props.concurrency.limit}` : '-- / --')
const callsPanelID = computed(() => `account-calls-${props.account.account_id}`)
const callsTitle = computed(() => ({ '24h': '24 小时调用', '7d': '7 天调用', '30d': '30 天调用' }[props.selectedRange]))
const callsSummary = computed(() => `${formatNumber(props.account.request_count)} 次请求 · ${formatNumber(props.account.error_count)} 次失败`)
const successfulRequestCount = computed(() => Math.max(0, Number(props.account.request_count) - Number(props.account.error_count)))
const probeTTFTP50MS = computed(() => props.account.probe_ttft_p50_ms ?? props.account.ttft_p50_ms ?? null)
const probeLatencyP95MS = computed(() => props.account.probe_latency_p95_ms ?? props.account.latency_p95_ms ?? null)
const realEvidence = computed(() => props.account.real_request_evidence)
const realSuccessRate = computed(() => {
  if (realEvidence.value && realEvidence.value.request_count > 0) return formatPercent(realEvidence.value.success_rate)
  if (props.account.success_rate != null) return formatPercent(props.account.success_rate)
  return '--'
})
const realTTFTP95 = computed(() => formatMs(realEvidence.value?.ttft_p95_ms ?? props.account.ttft_p95_ms))
const profitRateLabel = computed(() => {
  if (props.rankingScope === 'global') return '按分组查看'
  const profit = props.account.group_profitability
  if (!profit || !['confirmed', 'estimated'].includes(profit.status) || profit.profit_rate == null) return profit?.status === 'no_real_request' ? '--' : '待确认'
  return formatPercent(profit.profit_rate)
})
const realRequestBars = computed(() => {
  const points = props.account.real_request_timeline ?? []
  if (!points.length) return Array.from({ length: 24 }, () => ({ colorClass: 'bg-gray-200 dark:bg-slate-700', height: 16, title: '暂无真实请求' }))
  return points.map((point) => {
    const slow = point.ttft_p95_ms != null && point.ttft_p95_ms > 5000
    const probeCount = point.probe_count ?? 0
    const source = point.source ?? (point.request_count > 0 ? 'real' : probeCount > 0 ? 'probe' : 'no_data')
    const requestCount = point.request_count + probeCount
    const colorClass = requestCount === 0 ? 'bg-gray-200 dark:bg-slate-700' : source === 'probe' ? (point.probe_failure_count && point.probe_failure_count > 0 ? 'bg-red-500' : 'bg-amber-400') : point.failure_count > 0 && point.success_count === 0 ? 'bg-red-500' : slow ? 'bg-amber-400' : 'bg-emerald-500'
    const sourceLabel = source === 'probe' ? '主动探测兜底' : source === 'no_data' ? '无数据' : source === 'mixed' ? '含真实请求/探测兜底' : '真实请求'
    return { colorClass, height: requestCount === 0 ? 16 : Math.max(28, Math.min(100, 28 + requestCount * 4)), title: `${formatShortTime(point.start_at)} · ${sourceLabel} · 真实请求 ${point.request_count} · 探测 ${probeCount} · 成功 ${point.success_count + (point.probe_success_count ?? 0)} · 失败 ${point.failure_count + (point.probe_failure_count ?? 0)} · TTFT P95 ${formatMs(point.ttft_p95_ms)}` }
  })
})
const realTimelineAriaLabel = computed(() => `真实性能，${realEvidence.value?.request_count ?? props.account.request_count ?? 0} 次真实请求`)
const checkedAtLabel = computed(() => formatDateTime(props.account.checked_at ?? props.account.latest?.checked_at ?? null))
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
    tone: { type: String, required: false, default: '' },
  },
  setup(metricProps, { attrs }) {
    return () => h('div', { ...attrs, class: ['service-metric', attrs.class] }, [
      h('div', { class: 'metric-label' }, metricProps.label),
      h('div', { class: 'metric-value', 'data-test': `${metricProps.tone || metricProps.label}-metric-value` }, metricProps.value),
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

<style scoped>
.monitor-card-shell {
  --monitor-bg: #08111f;
  --monitor-bg-deep: #07101c;
  --monitor-panel: #0a1626;
  --monitor-line: #1c2a3e;
  --monitor-line-strong: #24344b;
  --monitor-text: #dbe7f5;
  --monitor-muted: #91a5ba;
  --monitor-dim: #71849c;
  display: block;
  width: 100%;
  min-width: 0;
  overflow: hidden;
  border: 1px solid var(--monitor-line-strong);
  border-radius: 14px;
  background: var(--monitor-bg);
  color: var(--monitor-text);
  box-shadow: none;
}

.monitor-card-shell :deep(button) {
  font-family: inherit;
}

.monitor-card-shell > [data-test="monitor-card-header"] {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 18px;
  min-width: 0;
  padding: 22px 24px 18px;
  border-bottom: 1px solid var(--monitor-line);
  background: var(--monitor-bg);
}

.monitor-card-shell > [data-test="monitor-card-header"] > [data-test="identity-column"] {
  grid-column: 1;
  min-width: 0;
}

.monitor-card-shell > [data-test="monitor-card-header"] > [data-test="scheduler-column"] {
  grid-column: 2;
  min-width: 148px;
  border: 0;
  padding: 0;
  text-align: right;
}

.monitor-card-shell > [data-test="monitor-card-header"] > [data-test="key-metrics"] {
  grid-column: 1;
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 0;
  min-width: 0;
  padding: 18px 24px 22px;
  background: var(--monitor-bg);
}

.monitor-card-shell > [data-test="monitor-card-header"] > [data-test="timeline-section"] {
  grid-column: 2;
  min-width: 0;
  border-top: 0;
  border-left: 1px solid var(--monitor-line);
  padding: 18px 20px;
  background: var(--monitor-panel);
}

.monitor-card-shell > [data-test="monitor-card-header"] > [data-test="account-actions"] {
  grid-column: 1 / -1;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  min-width: 0;
  padding: 13px 24px;
  border-top: 1px solid var(--monitor-line);
  background: var(--monitor-bg-deep);
}

.monitor-card-shell > [data-test="monitor-card-header"] > [data-test="model-detection-section"],
.monitor-card-shell > [data-test="monitor-card-header"] > [data-test="calls-disclosure"],
.monitor-card-shell > [data-test="monitor-card-header"] > [data-test="card-footer"] {
  display: none;
}

.monitor-card-shell [data-test="identity-column"] > .flex:first-child {
  align-items: center;
  gap: 8px;
}

.monitor-card-shell [data-test="identity-column"] .mt-1\.5 {
  margin-top: 0;
}

.monitor-card-shell [data-test="identity-column"] [data-test="account-identity"] {
  margin: 8px 0 4px;
  color: #f7fbff;
  font-size: 21px;
  line-height: 1.2;
  letter-spacing: -0.02em;
}

.monitor-card-shell [data-test="identity-column"] [data-test="account-identity"] a,
.monitor-card-shell [data-test="identity-column"] [data-test="account-identity"] span {
  color: inherit;
}

.monitor-card-shell [data-test="identity-column"] [data-test="account-identity"] .font-mono {
  color: #8094ab;
  font-size: 13px;
  font-weight: 500;
}

.monitor-card-shell [data-test="account-metadata"] {
  margin-top: 0;
  color: #92a4b8;
  font-size: 13px;
  line-height: 1.55;
}

.monitor-card-shell [data-test="identity-column"] > .mt-2 {
  margin-top: 10px;
}

.monitor-card-shell [data-test="status-badge"] {
  border-radius: 999px;
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 650;
}

.monitor-card-shell [data-test="identity-column"] .h-2.w-2 {
  width: 8px;
  height: 8px;
  box-shadow: 0 0 0 4px #103a35;
}

.monitor-card-shell [data-test="scheduler-column"] > div:first-child,
.monitor-card-shell [data-test="scheduler-column"] > .monitor-card-scheduler-label {
  color: #8fa2b7;
  font-size: 12px;
}

.monitor-card-shell [data-test="scheduler-rank"] {
  display: block;
  margin-top: 4px;
  color: #f6fbff;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 34px;
  line-height: 1.1;
  letter-spacing: -0.06em;
}

.monitor-card-shell [data-test="scheduler-rank"] span,
.monitor-card-shell [data-test="scheduler-rank"] small {
  color: #90a4b9;
  font-size: 14px;
  letter-spacing: 0;
}

.monitor-card-shell [data-test="scheduler-column"] .mt-1.flex,
.monitor-card-shell [data-test="scheduler-column"] [data-test="priority-control"] {
  justify-content: flex-end;
  margin-top: 8px;
  color: var(--monitor-muted);
  font-size: 11px;
}

.monitor-card-shell [data-test="scheduler-column"] [data-test="priority-control"] strong {
  color: var(--monitor-text);
}

.monitor-card-shell [data-test="scheduler-column"] [data-test="priority-error"] {
  text-align: right;
}

.monitor-card-shell [data-test="key-metrics"] > .service-metric,
.monitor-card-shell [data-test="key-metrics"] > [data-test="profit-rate-metric"],
.monitor-card-shell [data-test="key-metrics"] > [data-test="native-priority-metric"],
.monitor-card-shell [data-test="key-metrics"] > [data-test="upstream-multiplier-metric"] {
  min-width: 0;
  min-height: 118px;
  padding: 0 12px 0 0;
  border: 0;
  border-right: 1px solid #1b293d;
  border-radius: 0;
  background: transparent;
}

.monitor-card-shell [data-test="key-metrics"] > :nth-child(5) {
  border-right: 0;
}

.monitor-card-shell [data-test="key-metrics"] > :not(.monitor-card-legacy) > div:first-child,
.monitor-card-shell [data-test="key-metrics"] > :not(.monitor-card-legacy) > span:first-child {
  display: block;
  color: #90a1b5;
  font-size: 12px;
}

.monitor-card-shell [data-test="key-metrics"] > :not(.monitor-card-legacy) strong {
  display: block;
  margin-top: 7px;
  color: #eaf2fb;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 16px;
  line-height: 1.35;
}

.monitor-card-shell [data-test="key-metrics"] > :not(.monitor-card-legacy) small,
.monitor-card-shell [data-test="key-metrics"] > :not(.monitor-card-legacy) p {
  display: block;
  margin-top: 4px;
  color: #71849c;
  font-size: 11px;
  line-height: 1.35;
}

.monitor-card-shell [data-test="key-metrics"] > [data-test="upstream-multiplier-metric"] .flex,
.monitor-card-shell [data-test="key-metrics"] > [data-test="upstream-multiplier-metric"] > div {
  display: flex;
  align-items: center;
  gap: 7px;
}

.monitor-card-shell [data-test="key-metrics"] > [data-test="upstream-multiplier-metric"] button {
  min-height: 24px;
  border: 1px solid #30445d;
  border-radius: 6px;
  background: #0d1b2d;
  color: #b8c9da;
  padding: 0 7px;
  font-size: 11px;
}

.monitor-card-shell [data-test="key-metrics"] > [data-test="cost-metric"],
.monitor-card-shell [data-test="key-metrics"] > [data-test="balance-metric"],
.monitor-card-shell [data-test="key-metrics"] > [data-test="concurrency-metric"] {
  display: none;
}

.monitor-card-shell [data-test="timeline-section"] h3 {
  margin: 0;
  color: #b8c9da;
  font-size: 12px;
  font-weight: 600;
}

.monitor-card-shell [data-test="timeline-section"] > div:first-child {
  align-items: flex-start;
}

.monitor-card-shell [data-test="timeline-section"] > div:first-child > div::after { display: none; content: none; }

.monitor-card-shell [data-test="edit-connection-probe-model"] {
  color: #57d8be;
  font-size: 11px;
  text-decoration: none;
}

.monitor-card-shell [data-test="timeline-section"] [role="img"] {
  display: flex;
  align-items: flex-end;
  height: 104px;
  gap: 4px;
  margin: 15px 0 7px;
}

.monitor-card-shell [data-test="real-request-bar"] {
  flex: 1;
  min-width: 3px;
  border-radius: 3px 3px 1px 1px;
}

.monitor-card-shell [data-test="timeline-section"]::after {
  display: block;
  color: #91a5ba;
  font-size: 11px;
  content: '● 正常    ● TTFT 变慢    ● 失败';
  white-space: pre;
}

.monitor-card-shell > [data-test="monitor-card-header"] > [data-test="account-actions"] .icon-button {
  display: inline-flex;
  width: auto;
  min-height: 32px;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: 1px solid #30445d;
  border-radius: 7px;
  background: #0d1b2d;
  color: #dce8f5;
  padding: 0 11px;
  font-size: 12px;
}

.monitor-card-shell > [data-test="monitor-card-header"] > [data-test="account-actions"] .icon-button:first-of-type {
  border-color: #167e70;
  background: #0e5c54;
  color: #edfffb;
}

.monitor-card-shell > [data-test="monitor-card-header"] > [data-test="account-actions"] .icon-button span {
  position: static;
  width: auto;
  height: auto;
  overflow: visible;
  clip: auto;
  margin: 0;
  padding: 0;
  white-space: nowrap;
}

@media (max-width: 960px) {
  .monitor-card-shell > [data-test="monitor-card-header"] {
    grid-template-columns: 1fr;
  }

  .monitor-card-shell > [data-test="monitor-card-header"] > [data-test="identity-column"],
  .monitor-card-shell > [data-test="monitor-card-header"] > [data-test="scheduler-column"],
  .monitor-card-shell > [data-test="monitor-card-header"] > [data-test="key-metrics"],
  .monitor-card-shell > [data-test="monitor-card-header"] > [data-test="timeline-section"] {
    grid-column: 1;
  }

  .monitor-card-shell > [data-test="monitor-card-header"] > [data-test="scheduler-column"] {
    justify-items: start;
    text-align: left;
  }

  .monitor-card-shell > [data-test="monitor-card-header"] > [data-test="key-metrics"] {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .monitor-card-shell > [data-test="monitor-card-header"] > [data-test="timeline-section"] {
    border-top: 1px solid var(--monitor-line);
    border-left: 0;
  }
}

@media (max-width: 560px) {
  .monitor-card-shell > [data-test="monitor-card-header"] {
    padding: 18px 16px 14px;
  }

  .monitor-card-shell > [data-test="monitor-card-header"] > [data-test="key-metrics"] {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    padding: 16px 0 18px;
  }

  .monitor-card-shell [data-test="key-metrics"] > .service-metric,
  .monitor-card-shell [data-test="key-metrics"] > [data-test="profit-rate-metric"],
  .monitor-card-shell [data-test="key-metrics"] > [data-test="native-priority-metric"],
  .monitor-card-shell [data-test="key-metrics"] > [data-test="upstream-multiplier-metric"] {
    padding: 0 10px 12px 0;
    border-top: 1px solid #1b293d;
    border-right: 1px solid #1b293d;
  }

  .monitor-card-shell [data-test="key-metrics"] > :nth-child(even) {
    border-right: 0;
    padding-left: 10px;
  }

  .monitor-card-shell > [data-test="monitor-card-header"] > [data-test="account-actions"] {
    align-items: flex-start;
    flex-wrap: wrap;
    justify-content: flex-start;
    padding: 12px 0;
  }
}

/* The live component keeps its existing interaction hooks, but the shell follows the approved design-grid. */
.monitor-card-shell > .monitor-card-layout {
  display: grid;
  grid-template-columns: minmax(0, 1.72fr) minmax(280px, .78fr);
  grid-template-areas:
    'identity scheduler'
    'metrics quality'
    'foot foot'
    'detection detection';
  gap: 0;
  padding: 0;
  background: var(--monitor-bg);
}

.monitor-card-shell > .monitor-card-layout > [data-test="identity-column"] { grid-area: identity; }
.monitor-card-shell > .monitor-card-layout > [data-test="quality-column"] { display: none; }
.monitor-card-shell > .monitor-card-layout > [data-test="scheduler-column"] { grid-area: scheduler; }
.monitor-card-shell > .monitor-card-layout > [data-test="key-metrics"] { grid-area: metrics; }
.monitor-card-shell > .monitor-card-layout > [data-test="timeline-section"] { grid-area: quality; }
.monitor-card-shell > .monitor-card-layout > [data-test="account-actions"] { grid-area: foot; }
.monitor-card-shell > .monitor-card-layout > [data-test="model-detection-section"] { grid-area: detection; }
.monitor-card-shell > .monitor-card-layout > [data-test="calls-disclosure"],
.monitor-card-shell > .monitor-card-layout > [data-test="card-footer"] { display: none; }

.monitor-card-shell > .monitor-card-layout > [data-test="identity-column"] {
  min-width: 0;
  padding: 22px 24px 18px;
}

.monitor-card-shell > .monitor-card-layout > [data-test="scheduler-column"] {
  min-width: 0;
  padding: 22px 24px 18px;
  border: 0;
  border-bottom: 1px solid var(--monitor-line);
  text-align: right;
}

.monitor-card-shell > .monitor-card-layout > [data-test="key-metrics"] {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 0;
  min-width: 0;
  padding: 18px 24px 22px;
  border-top: 1px solid var(--monitor-line);
  background: var(--monitor-bg);
}

.monitor-card-shell > .monitor-card-layout > [data-test="timeline-section"] {
  min-width: 0;
  border-top: 1px solid var(--monitor-line);
  border-left: 1px solid var(--monitor-line);
  padding: 18px 20px;
  background: var(--monitor-panel);
}

.monitor-card-shell > .monitor-card-layout > [data-test="account-actions"] {
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  min-width: 0;
  padding: 13px 24px;
  border-top: 1px solid var(--monitor-line);
  background: var(--monitor-bg-deep);
}

.monitor-card-shell > .monitor-card-layout > [data-test="account-actions"] .icon-button {
  display: inline-flex;
  width: auto;
  min-height: 32px;
  align-items: center;
  justify-content: center;
  gap: 6px;
  border: 1px solid #30445d;
  border-radius: 7px;
  background: #0d1b2d;
  color: #dce8f5;
  padding: 0 11px;
  font-size: 12px;
}

.monitor-card-shell > .monitor-card-layout > [data-test="account-actions"] .icon-button:first-of-type {
  border-color: #167e70;
  background: #0e5c54;
  color: #edfffb;
}

.monitor-card-shell > .monitor-card-layout > [data-test="account-actions"] .icon-button span {
  position: static;
  width: auto;
  height: auto;
  overflow: visible;
  clip: auto;
  margin: 0;
  padding: 0;
  white-space: nowrap;
}

.monitor-card-shell > .monitor-card-layout > [data-test="account-actions"]::before { display: none; content: none; }

.monitor-card-shell > .monitor-card-layout > [data-test="account-actions"] [data-test="account-edit"],
.monitor-card-shell > .monitor-card-layout > [data-test="account-actions"] [data-test="account-delete"],
.monitor-card-shell > .monitor-card-layout > [data-test="account-actions"] [data-test="refresh-account"] {
  display: none;
}

.monitor-card-shell > .monitor-card-layout > [data-test="model-detection-section"] {
  min-width: 0;
  padding: 0 24px 12px;
  border: 0;
  background: var(--monitor-bg-deep);
}

.monitor-card-shell > .monitor-card-layout > [data-test="model-detection-section"] button {
  display: flex;
  width: 100%;
  min-height: 28px;
  align-items: center;
  gap: 8px;
  color: var(--monitor-muted);
  font-size: 11px;
  text-align: left;
}

.monitor-card-shell > .monitor-card-layout > [data-test="model-detection-section"] button > span:nth-child(2) { border-radius: 999px; padding: 2px 8px; }
.monitor-card-shell > .monitor-card-layout > [data-test="model-detection-section"] button > span:nth-child(3) { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.monitor-card-shell > .monitor-card-layout > [data-test="model-detection-section"] button > svg { margin-left: auto; }

.monitor-card-shell > .monitor-card-layout > [data-test="key-metrics"] > .service-metric,
.monitor-card-shell > .monitor-card-layout > [data-test="key-metrics"] > [data-test="profit-rate-metric"],
.monitor-card-shell > .monitor-card-layout > [data-test="key-metrics"] > [data-test="native-priority-metric"],
.monitor-card-shell > .monitor-card-layout > [data-test="key-metrics"] > [data-test="upstream-multiplier-metric"] {
  min-width: 0;
  min-height: 118px;
  padding: 0 12px 0 0;
  border: 0;
  border-right: 1px solid #1b293d;
  border-radius: 0;
  background: transparent;
}

.monitor-card-shell > .monitor-card-layout > [data-test="key-metrics"] > [data-test="upstream-multiplier-metric"] { border-right: 0; }
.monitor-card-shell > .monitor-card-layout > [data-test="key-metrics"] > [data-test="latency-metric"],
.monitor-card-shell > .monitor-card-layout > [data-test="key-metrics"] > [data-test="cost-metric"] { display: none; }

@media (max-width: 960px) {
  .monitor-card-shell > .monitor-card-layout {
    grid-template-columns: 1fr;
    grid-template-areas: 'identity' 'scheduler' 'metrics' 'quality' 'foot' 'detection';
  }
  .monitor-card-shell > .monitor-card-layout > [data-test="scheduler-column"] { text-align: left; }
  .monitor-card-shell > .monitor-card-layout > [data-test="timeline-section"] { border-left: 0; }
  .monitor-card-shell > .monitor-card-layout > [data-test="key-metrics"] { grid-template-columns: repeat(3, minmax(0, 1fr)); }
}

@media (max-width: 560px) {
  .monitor-card-shell > .monitor-card-layout > [data-test="identity-column"],
  .monitor-card-shell > .monitor-card-layout > [data-test="scheduler-column"] { padding: 18px 16px 14px; }
  .monitor-card-shell > .monitor-card-layout > [data-test="key-metrics"] { grid-template-columns: repeat(2, minmax(0, 1fr)); padding: 16px 16px 18px; }
  .monitor-card-shell > .monitor-card-layout > [data-test="account-actions"] { align-items: flex-start; flex-wrap: wrap; justify-content: flex-start; padding: 12px 16px; }
  .monitor-card-shell > .monitor-card-layout > [data-test="model-detection-section"] { padding-inline: 16px; }
}
</style>

<style scoped>
/* Final R2 surface: one visual contract, with legacy utility overrides intentionally
   kept below the component so old class-based selectors cannot reshape the card. */
.monitor-card-shell {
  --monitor-bg: #0b1523;
  --monitor-bg-deep: #09121e;
  --monitor-line: #1e2d41;
  --monitor-line-strong: #26384f;
  --monitor-text: #dbe7f5;
  --monitor-muted: #9eb0c1;
  display: block !important;
  width: 100%;
  overflow: hidden;
  border: 1px solid var(--monitor-line-strong);
  border-radius: 14px;
  background: var(--monitor-bg);
  color: var(--monitor-text);
  box-shadow: none;
}

.monitor-card-layout {
  display: block !important;
  min-width: 0;
  padding: 0 !important;
  background: var(--monitor-bg);
}

.monitor-card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 18px;
  min-width: 0;
  padding: 20px 22px 16px;
  border-bottom: 1px solid var(--monitor-line);
}

.monitor-card-identity { min-width: 0; }
.monitor-card-eyeline {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: #c2d1df;
  font-size: 12px;
  line-height: 1.4;
}
.monitor-card-status-dot {
  width: 8px;
  height: 8px;
  flex: 0 0 auto;
  border-radius: 50%;
  box-shadow: 0 0 0 4px #103a35;
}
.monitor-card-identity h2 {
  margin: 8px 0 4px;
  color: #f7fbff;
  font-size: 22px;
  font-weight: 650;
  letter-spacing: -0.02em;
  line-height: 1.2;
  overflow-wrap: anywhere;
}
.monitor-card-identity h2 a,
.monitor-card-identity h2 a:hover { color: inherit; }
.monitor-card-identity .account-id {
  color: #8094ab;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
  font-weight: 500;
  letter-spacing: 0;
}
.monitor-card-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0 8px;
  min-width: 0;
  color: var(--monitor-muted);
  font-size: 13px;
  line-height: 1.55;
}
.monitor-card-recommendation {
  border: 0;
  background: transparent;
  padding: 0;
  color: #57d8be;
  font: inherit;
  cursor: pointer;
}

.monitor-card-scheduler {
  min-width: 148px;
  flex: 0 0 auto;
  text-align: right;
}
.scheduler-label {
  color: var(--monitor-muted);
  font-size: 11px;
}
.scheduler-rank {
  margin-top: 4px;
  color: #f6fbff;
  font: 600 30px/1.1 ui-monospace, SFMono-Regular, Menlo, monospace;
  letter-spacing: -0.06em;
  white-space: nowrap;
}
.scheduler-rank span {
  color: #8fa3b8;
  font-size: 13px;
  letter-spacing: 0;
}
.scheduler-hint {
  margin-top: 6px;
  color: #57d8be;
  font-size: 11px;
}
.priority-control {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  margin-top: 10px;
  color: var(--monitor-muted);
  font-size: 11px;
}
.priority-control strong { color: var(--monitor-text); font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
.priority-input { width: 64px; min-height: 28px; border: 1px solid #30445d; border-radius: 6px; background: #0d1b2d; color: #e7f0f8; padding: 0 7px; font: 12px ui-monospace, SFMono-Regular, Menlo, monospace; }
.priority-icon { display: inline-grid; width: 28px; height: 28px; place-items: center; border: 1px solid #30445d; border-radius: 6px; background: #0d1b2d; color: #dce8f5; }
.priority-error { margin: 5px 0 0; color: #f39aa4; font-size: 11px; }

.monitor-card-body {
  display: grid;
  grid-template-columns: minmax(0, 1.72fr) minmax(280px, .78fr);
  gap: 0;
  min-width: 0;
}
.monitor-card-main { min-width: 0; padding: 18px 22px 20px; }
.monitor-card-metrics {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 12px;
  min-width: 0;
}
.monitor-card-metrics :deep(.service-metric) {
  min-width: 0;
  min-height: 74px;
  padding: 0 12px 0 0;
  border: 0;
  border-right: 1px solid #203046;
  border-radius: 0;
  background: transparent;
}
.monitor-card-metrics :deep(.service-metric:last-child) { border-right: 0; padding-right: 0; }
.monitor-card-metrics :deep(.metric-link) { width: fit-content; border: 0; background: transparent; padding-inline: 0; text-align: left; cursor: pointer; }
.monitor-card-metrics :deep(.metric-label) { display: block; color: #9eb0c1; font-size: 11px; }
.monitor-card-metrics :deep(.metric-value) { display: block; margin-top: 7px; color: #edf5fc; font: 600 17px/1.35 ui-monospace, SFMono-Regular, Menlo, monospace; white-space: nowrap; }
.monitor-card-metrics :deep(.metric-detail) { margin-top: 4px; color: #71869e; font-size: 11px; line-height: 1.35; }

.monitor-card-chart {
  min-width: 0;
  padding: 18px 20px;
  border-left: 1px solid var(--monitor-line);
  background: #0a1626;
}
.chart-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.chart-head h3 { margin: 0; color: #d7e3ee; font-size: 12px; font-weight: 600; }
.chart-action {
  display: inline-flex;
  min-height: 28px;
  align-items: center;
  gap: 6px;
  border: 1px solid #2f4862;
  border-radius: 6px;
  background: #102237;
  color: #dce8f5;
  padding: 0 9px;
  font-size: 11px;
}
.chart-action:disabled { cursor: wait; opacity: .65; }
.performance-bars {
  display: flex;
  align-items: flex-end;
  height: 92px;
  gap: 4px;
  margin-top: 12px;
}
.performance-bar {
  flex: 1;
  min-width: 3px;
  border-radius: 3px 3px 1px 1px;
  outline: none;
}
.monitor-card-chart::after { display: none !important; content: none !important; }

.monitor-card-model {
  display: grid;
  grid-template-columns: auto auto 1fr auto;
  align-items: center;
  gap: 9px;
  min-width: 0;
  margin-top: 16px;
  padding-top: 13px;
  border-top: 1px solid var(--monitor-line);
}
.model-status {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  border: 0;
  background: transparent;
  color: #dce8f3;
  padding: 0;
  font-size: 12px;
}
.model-status > svg { color: #8fa3b8; }
.model-title { font-weight: 650; }
.model-pill { border-radius: 999px; padding: 3px 7px; color: #b9c8d8; font-size: 10px; }
.model-edit, .model-detect {
  min-height: 28px;
  border: 1px solid #2f4862;
  border-radius: 6px;
  background: #102237;
  color: #dce8f5;
  padding: 0 9px;
  font-size: 11px;
  white-space: nowrap;
}
.model-detect { border-color: #2f766a; background: #0d5d54; color: #effffb; }
.model-detect:disabled { cursor: wait; opacity: .65; }

.monitor-card-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  min-width: 0;
  padding: 13px 22px;
  border-top: 1px solid var(--monitor-line);
  background: var(--monitor-bg-deep);
}
.monitor-card-footer::before { display: none !important; content: none !important; }
.footer-button {
  display: inline-flex;
  min-height: 32px;
  align-items: center;
  gap: 6px;
  border: 1px solid #30455e;
  border-radius: 7px;
  background: #101f31;
  color: #dce8f5;
  padding: 0 12px;
  font-size: 12px;
}
.footer-button.primary { border-color: #167e70; background: #0d5d54; color: #edfffb; }

@media (max-width: 700px) {
  .monitor-card-header { flex-direction: column; }
  .monitor-card-scheduler { width: 100%; text-align: left; }
  .priority-control { justify-content: flex-start; }
}
@media (max-width: 900px) {
  .monitor-card-body { grid-template-columns: 1fr; }
  .monitor-card-chart { border-top: 1px solid var(--monitor-line); border-left: 0; }
}
@media (max-width: 900px) and (min-width: 561px) {
  .monitor-card-metrics { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .monitor-card-metrics :deep(.service-metric:nth-child(3)) { border-right: 0; }
  .monitor-card-metrics :deep(.service-metric:nth-child(n + 4)) { padding-top: 12px; border-top: 1px solid #203046; }
}
@media (max-width: 560px) {
  .monitor-card-header, .monitor-card-main { padding-inline: 16px; }
  .monitor-card-metrics { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .monitor-card-metrics :deep(.service-metric) { padding-top: 12px; padding-bottom: 12px; border-top: 1px solid #203046; }
  .monitor-card-metrics :deep(.service-metric:nth-child(2n)) { border-right: 0; padding-right: 0; padding-left: 10px; }
  .monitor-card-footer { flex-direction: column; padding-inline: 16px; }
  .footer-button { width: 100%; justify-content: center; }
  .monitor-card-model { grid-template-columns: 1fr auto; }
  .model-status { grid-column: 1 / -1; }
  .model-edit, .model-detect { width: 100%; }
}
</style>
