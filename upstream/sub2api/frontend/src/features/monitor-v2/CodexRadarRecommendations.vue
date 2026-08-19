<template>
  <section class="mt-6 min-w-0 overflow-hidden rounded-xl border border-slate-700 bg-slate-950 p-4 text-slate-100 shadow-sm sm:p-5" aria-labelledby="codexradar-title">
    <header class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <div class="flex items-center gap-2">
          <span class="inline-flex h-7 w-7 items-center justify-center rounded-full border border-slate-600 bg-slate-900 text-sm">◎</span>
          <h2 id="codexradar-title" class="text-lg font-bold tracking-tight">站长推荐</h2>
          <span v-if="insights?.stale" class="rounded-full border border-amber-400/40 bg-amber-400/10 px-2 py-0.5 text-[11px] font-medium text-amber-300">最近成功数据</span>
        </div>
        <p class="mt-1 text-xs text-slate-400">根据 CodexRadar 公开实测数据，按任务场景自动推荐。</p>
      </div>
      <p v-if="insights" class="text-xs font-medium text-slate-400">{{ formatUpdatedAt(insights.source_updated_at) }} 更新</p>
    </header>

    <div v-if="loading" class="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-busy="true">
      <div v-for="index in 4" :key="index" class="h-36 animate-pulse rounded-lg border border-slate-700 bg-slate-900 motion-reduce:animate-none" />
    </div>
    <p v-else-if="failed" class="mt-4 rounded-lg border border-slate-700 bg-slate-900 px-4 py-5 text-sm text-slate-300">站长推荐暂时不可用</p>
    <div v-else-if="insights" class="mt-4 grid min-w-0 grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <article
        v-for="recommendation in insights.recommendations"
        :key="recommendation.key"
        data-radar-category
        class="min-w-0 overflow-hidden rounded-lg border bg-slate-900/90"
        :class="categoryClass(recommendation.key)"
      >
        <div class="border-b border-current/25 px-3 py-2.5">
          <h3 class="text-sm font-bold">{{ recommendation.title }}</h3>
          <p class="mt-1 line-clamp-3 break-words text-[11px] leading-4 text-slate-400" :title="recommendation.rule">{{ recommendation.rule }}</p>
        </div>
        <div class="divide-y divide-slate-700/80">
          <div v-for="item in recommendation.items" :key="`${item.model}-${item.effort}`" class="grid grid-cols-[minmax(0,1fr)_auto] gap-3 px-3 py-2.5">
            <div class="min-w-0">
              <p class="truncate text-xs font-bold text-slate-100">{{ modelLabel(item.model) }} {{ item.effort }}</p>
              <p class="mt-1 text-[11px] text-slate-400">IQ {{ Math.round(item.iq) }} · {{ Math.round(item.average_duration_minutes) }} 分钟</p>
            </div>
            <p class="self-center text-sm font-extrabold text-emerald-400">${{ item.average_cost_usd.toFixed(2) }}</p>
          </div>
        </div>
      </article>
    </div>
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { getCodexRadarInsights, type CodexRadarInsights, type CodexRadarKey } from './codexRadar'

const insights = ref<CodexRadarInsights | null>(null)
const loading = ref(true)
const failed = ref(false)
let controller: AbortController | null = null

function categoryClass(key: CodexRadarKey) {
  return {
    daily_development: 'border-emerald-400/55 text-emerald-400 shadow-[inset_4px_0_0_#34d399]',
    hard_problems: 'border-amber-400/55 text-amber-400 shadow-[inset_4px_0_0_#fbbf24]',
    background_automation: 'border-violet-400/55 text-violet-400 shadow-[inset_4px_0_0_#a78bfa]',
    lobster_tasks: 'border-blue-400/55 text-blue-400 shadow-[inset_4px_0_0_#60a5fa]',
  }[key]
}

function modelLabel(model: string) {
  const normalized = model.replace(/^gpt-/i, '')
  if (normalized.endsWith('-sol')) return 'Sol'
  if (normalized.endsWith('-terra')) return 'Terra'
  if (normalized.endsWith('-luna')) return 'Luna'
  return normalized
}

function formatUpdatedAt(value: string) {
  return new Intl.DateTimeFormat(undefined, { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

onMounted(() => {
  controller = new AbortController()
  void Promise.resolve()
    .then(() => getCodexRadarInsights(controller?.signal))
    .then((value) => {
      if (value) insights.value = value
      else failed.value = true
    })
    .catch((error: unknown) => {
      if ((error as { name?: string })?.name !== 'AbortError') failed.value = true
    })
    .finally(() => { loading.value = false })
})

onBeforeUnmount(() => controller?.abort())
</script>
