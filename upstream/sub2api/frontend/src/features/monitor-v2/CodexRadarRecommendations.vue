<template>
  <section data-test="codexradar-panel" class="mt-7 min-w-0 overflow-hidden rounded-2xl border border-gray-200 bg-white p-5 text-gray-950 shadow-lg shadow-gray-200/70 sm:p-6 xl:p-7 dark:border-dark-700 dark:bg-dark-950 dark:text-white dark:shadow-black/20" aria-labelledby="codexradar-title">
    <header class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <div class="flex items-center gap-2">
          <span class="inline-flex h-9 w-9 items-center justify-center rounded-full border border-gray-200 bg-gray-50 text-base text-gray-700 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-200">◎</span>
          <h2 id="codexradar-title" class="text-xl font-black tracking-tight text-gray-950 dark:text-white sm:text-2xl">站长推荐</h2>
          <span v-if="insights?.stale" class="rounded-full border border-amber-300 bg-amber-50 px-2 py-0.5 text-[11px] font-medium text-amber-700 dark:border-amber-400/40 dark:bg-amber-400/10 dark:text-amber-300">最近成功数据</span>
        </div>
        <p class="mt-1.5 text-sm text-gray-600 dark:text-gray-400">根据 CodexRadar 公开实测数据，按任务场景自动推荐。</p>
      </div>
      <p v-if="insights" class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ formatUpdatedAt(insights.source_updated_at) }} 更新</p>
    </header>

    <div v-if="loading" class="mt-4 grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-4" aria-busy="true">
      <div v-for="index in 4" :key="index" class="h-36 animate-pulse rounded-lg border border-gray-200 bg-gray-100 motion-reduce:animate-none dark:border-dark-700 dark:bg-dark-900" />
    </div>
    <div v-else-if="failed" class="mt-4 flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-gray-50 px-4 py-5 text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300">
      <span>站长推荐暂时不可用</span>
      <button type="button" class="shrink-0 rounded-md border border-gray-300 px-2.5 py-1 text-xs font-bold hover:bg-white dark:border-dark-600 dark:hover:bg-dark-800" @click="load">重试</button>
    </div>
    <div v-else-if="insights" class="mt-5 grid min-w-0 grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <article
        v-for="recommendation in insights.recommendations"
        :key="recommendation.key"
        data-radar-category
        class="min-w-0 overflow-hidden rounded-lg border bg-white dark:bg-dark-900/90"
        :class="categoryClass(recommendation.key)"
      >
        <div class="border-b border-current/25 px-4 py-3.5">
          <h3 class="text-base font-black">{{ recommendation.title }}</h3>
          <p class="mt-1.5 line-clamp-3 break-words text-xs leading-5 text-gray-600 dark:text-gray-400" :title="recommendation.rule">{{ recommendation.rule }}</p>
        </div>
        <div v-if="recommendation.status === 'empty'" class="px-4 py-7 text-sm text-gray-500 dark:text-gray-400">当前暂无推荐</div>
        <div v-else class="divide-y divide-gray-200 dark:divide-dark-700/80">
          <div v-for="item in recommendation.items" :key="`${item.model}-${item.effort}`" class="grid grid-cols-[minmax(0,1fr)_auto] gap-3 px-4 py-3.5">
            <div class="min-w-0">
              <p class="truncate text-sm font-bold text-gray-900 dark:text-gray-100">{{ modelLabel(item.model) }} {{ item.effort }}</p>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">IQ {{ Math.round(item.iq) }} · {{ Math.round(item.average_duration_minutes) }} 分钟</p>
            </div>
            <p class="self-center text-base font-black text-emerald-600 dark:text-emerald-400">${{ item.average_cost_usd.toFixed(2) }}</p>
          </div>
        </div>
      </article>
    </div>
    <CodexRadarCommunityMatrix />
  </section>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { getCodexRadarInsights, type CodexRadarInsights, type CodexRadarKey } from './codexRadar'
import CodexRadarCommunityMatrix from './CodexRadarCommunityMatrix.vue'

const insights = ref<CodexRadarInsights | null>(null)
const loading = ref(true)
const failed = ref(false)
let controller: AbortController | null = null

function categoryClass(key: CodexRadarKey) {
  return {
    daily_development: 'border-emerald-300 bg-emerald-50/70 text-emerald-700 shadow-[inset_4px_0_0_#34d399] dark:border-emerald-400/55 dark:bg-dark-900/90 dark:text-emerald-300',
    hard_problems: 'border-amber-300 bg-amber-50/70 text-amber-700 shadow-[inset_4px_0_0_#fbbf24] dark:border-amber-400/55 dark:bg-dark-900/90 dark:text-amber-300',
    background_automation: 'border-violet-300 bg-violet-50/70 text-violet-700 shadow-[inset_4px_0_0_#a78bfa] dark:border-violet-400/55 dark:bg-dark-900/90 dark:text-violet-300',
    lobster_tasks: 'border-blue-300 bg-blue-50/70 text-blue-700 shadow-[inset_4px_0_0_#60a5fa] dark:border-blue-400/55 dark:bg-dark-900/90 dark:text-blue-300',
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

async function load() {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  failed.value = false
  try {
    const value = await getCodexRadarInsights(controller.signal)
    if (!value) throw new Error('empty CodexRadar response')
    insights.value = value
  } catch (error: unknown) {
    if ((error as { name?: string })?.name !== 'AbortError') {
      insights.value = null
      failed.value = true
    }
  } finally {
    loading.value = false
  }
}

onMounted(() => { void load() })

onBeforeUnmount(() => controller?.abort())
</script>
