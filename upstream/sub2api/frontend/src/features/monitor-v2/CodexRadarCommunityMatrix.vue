<template>
  <section class="mt-5 min-w-0 border-t border-gray-200 pt-5 dark:border-dark-700/80" aria-labelledby="codexradar-community-title">
    <div class="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
      <div class="min-w-0">
        <div id="codexradar-community-title" class="flex flex-wrap gap-2" role="tablist" aria-label="CodexRadar 社区测试类型">
          <button
            v-for="tab in tabs"
            :key="tab.key"
            type="button"
            role="tab"
            :aria-selected="activeKey === tab.key"
            :data-community-tab="tab.key"
            class="rounded-full border px-3 py-1.5 text-xs font-bold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400"
            :class="activeKey === tab.key ? 'border-blue-500/70 bg-blue-50 text-blue-700 dark:border-blue-400/70 dark:bg-blue-400/15 dark:text-blue-100' : 'border-gray-300 bg-gray-50 text-gray-600 hover:border-gray-400 hover:text-gray-950 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-300 dark:hover:border-dark-500 dark:hover:text-white'"
            @click="activeKey = tab.key"
          >{{ tab.icon }} {{ tab.title }}</button>
        </div>
        <p class="mt-2 text-xs font-medium text-gray-600 dark:text-gray-400">社区众测数据 · 每多一份贡献，结果就更准确。</p>
        <p v-if="activeKey === 'comprehensive'" class="mt-1 text-[11px] leading-4 text-gray-500 dark:text-gray-500">综合 IQ 为软件工程能力 IQ 与视觉空间推理 IQ 的等权几何平均，仅纳入两维均有有效成绩的档位。</p>
      </div>
      <div v-if="activeTab" class="flex shrink-0 items-center gap-2 text-xs font-medium text-gray-500 dark:text-gray-400">
        <span v-if="community?.stale" class="rounded-full border border-amber-300 bg-amber-50 px-2 py-0.5 text-[11px] text-amber-700 dark:border-amber-400/40 dark:bg-amber-400/10 dark:text-amber-300">最近成功数据</span>
        <span>{{ formatUpdatedAt(activeTab.source_updated_at) }} 更新</span>
      </div>
    </div>

    <div v-if="loading" class="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-6" aria-busy="true">
      <div v-for="index in 6" :key="index" class="h-28 animate-pulse rounded-lg border border-gray-200 bg-gray-100 motion-reduce:animate-none dark:border-dark-700 dark:bg-dark-900" />
    </div>
    <p v-else-if="failed" class="mt-4 rounded-lg border border-gray-200 bg-gray-50 px-4 py-5 text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300">社区测试数据暂时不可用</p>
    <div v-else-if="activeTab" data-community-scroll class="mt-5 min-w-0 max-w-full overflow-x-auto overscroll-x-contain pb-2" tabindex="0" aria-label="可横向滚动查看全部模型档位">
      <div data-community-grid class="min-w-[1120px] space-y-3">
        <div v-for="family in groupedPoints" :key="family.model" :data-community-family="family.model" class="grid grid-cols-6 gap-2.5">
          <article
            v-for="point in family.points"
            :key="`${point.model}-${point.effort}`"
            data-community-card
            :data-effort="point.effort"
            class="relative min-h-[116px] min-w-0 overflow-hidden rounded-xl border border-gray-200 bg-white px-4 py-3.5 shadow-[inset_4px_0_0_var(--family-color)] transition-colors hover:border-gray-300 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-900/95 dark:hover:border-dark-500 dark:hover:bg-dark-800/95"
            :style="{ '--family-color': familyColor(point.model), gridColumnStart: effortColumn(point.effort) }"
          >
            <div class="flex items-start justify-between gap-2">
              <p class="min-w-0 truncate text-sm font-bold text-gray-900 dark:text-gray-100">{{ modelLabel(point.model) }} {{ point.effort }}</p>
              <p class="shrink-0 text-xl font-black leading-none text-gray-950 dark:text-white">IQ {{ Math.round(point.iq) }}</p>
            </div>
            <p class="mt-1 text-[10px] font-semibold uppercase tracking-wider text-gray-500 dark:text-slate-500">社区实测</p>
            <div class="mt-3 grid grid-cols-3 gap-1 border-t border-gray-200 pt-2.5 text-center dark:border-dark-700/80">
              <div><strong class="block text-xs text-gray-800 dark:text-slate-200">{{ point.samples }}</strong><span class="text-[10px] text-gray-500 dark:text-slate-500">份样本</span></div>
              <div><strong class="block text-xs text-emerald-600 dark:text-emerald-300">{{ point.average_cost_usd == null ? '–' : `$${point.average_cost_usd.toFixed(2)}` }}</strong><span class="text-[10px] text-gray-500 dark:text-slate-500">平均费用</span></div>
              <div><strong class="block text-xs text-blue-600 dark:text-blue-300">{{ point.average_duration_minutes == null ? '–' : point.average_duration_minutes.toFixed(1) }}</strong><span class="text-[10px] text-gray-500 dark:text-slate-500">分钟</span></div>
            </div>
            <p class="sr-only">{{ point.samples }} 份样本 · {{ point.average_cost_usd == null ? '费用暂无' : `$${point.average_cost_usd.toFixed(2)}` }} · {{ point.average_duration_minutes == null ? '耗时暂无' : `${point.average_duration_minutes.toFixed(1)} 分钟` }}</p>
          </article>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { getCodexRadarCommunity, type CodexRadarCommunity, type CodexRadarCommunityKey, type CodexRadarCommunityPoint } from './codexRadarCommunity'

const tabs: Array<{ key: CodexRadarCommunityKey; icon: string; title: string }> = [
  { key: 'comprehensive', icon: '🧠', title: '综合智能' },
  { key: 'software', icon: '💻', title: '软件工程能力' },
  { key: 'visual', icon: '🧩', title: '视觉空间推理' },
]

const community = ref<CodexRadarCommunity | null>(null)
const activeKey = ref<CodexRadarCommunityKey>('comprehensive')
const loading = ref(true)
const failed = ref(false)
let controller: AbortController | null = null

const activeTab = computed(() => community.value?.tabs.find((tab) => tab.key === activeKey.value) ?? null)
const groupedPoints = computed(() => {
  const groups: Array<{ model: string; points: CodexRadarCommunityPoint[] }> = []
  for (const point of activeTab.value?.points ?? []) {
    let group = groups.find((item) => item.model === point.model)
    if (!group) {
      group = { model: point.model, points: [] }
      groups.push(group)
    }
    group.points.push(point)
  }
  for (const group of groups) group.points.sort((a, b) => effortRank(a.effort) - effortRank(b.effort))
  return groups.sort((a, b) => familyRank(a.model) - familyRank(b.model))
})

const effortOrder = ['ultra', 'max', 'xhigh', 'high', 'medium', 'low'] as const

function effortRank(effort: string) {
  const index = effortOrder.indexOf(effort.toLowerCase() as (typeof effortOrder)[number])
  return index === -1 ? effortOrder.length : index
}

function effortColumn(effort: string) {
  return Math.min(effortRank(effort) + 1, effortOrder.length)
}

function familyRank(model: string) {
  const normalized = model.toLowerCase()
  if (normalized.includes('sol')) return 0
  if (normalized.includes('terra')) return 1
  if (normalized.includes('luna')) return 2
  if (normalized.includes('5.5')) return 3
  return 4
}

function modelLabel(model: string) {
  const normalized = model.replace(/^gpt-/i, '')
  if (normalized.endsWith('-sol')) return 'Sol'
  if (normalized.endsWith('-terra')) return 'Terra'
  if (normalized.endsWith('-luna')) return 'Luna'
  return normalized.replace(/(^|[-_])(\w)/g, (_, prefix: string, value: string) => `${prefix ? ' ' : ''}${value.toUpperCase()}`).trim()
}

function familyColor(model: string) {
  if (model.includes('sol')) return '#eab308'
  if (model.includes('terra')) return '#60a5fa'
  if (model.includes('luna')) return '#c084fc'
  if (model.includes('deepseek')) return '#34d399'
  return '#94a3b8'
}

function formatUpdatedAt(value: string) {
  return new Intl.DateTimeFormat(undefined, { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

onMounted(() => {
  controller = new AbortController()
  void getCodexRadarCommunity(controller.signal)
    .then((value) => { community.value = value })
    .catch((error: unknown) => {
      if ((error as { name?: string })?.name !== 'AbortError') failed.value = true
    })
    .finally(() => { loading.value = false })
})

onBeforeUnmount(() => controller?.abort())
</script>
