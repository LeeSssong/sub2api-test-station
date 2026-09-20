<template>
  <AppLayout>
    <section class="user-page ai-tools-workspace" data-testid="ai-tools-workspace" aria-labelledby="ai-tools-title">
      <header class="ai-page-head">
        <div>
          <h1 id="ai-tools-title">请选择你的 AI 工具</h1>
          <p>比较可用线路、稳定性和价格倍率</p>
        </div>
        <button type="button" class="btn btn-secondary ai-refresh" :disabled="loading" @click="loadWorkspace">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          刷新
        </button>
      </header>

      <div class="tool-grid" :aria-busy="loading">
        <article v-for="tool in toolCards" :key="tool.id" class="tool-card">
          <div class="tool-card-heading">
            <div class="tool-name">
              <img :src="tool.icon" alt="" />
              <h2>{{ tool.label }}</h2>
            </div>
            <span>{{ tool.type }}</span>
          </div>
          <div class="tool-status" :class="`is-${tool.statusKind}`">
            <span class="status-dot" aria-hidden="true"></span>
            <span>{{ tool.statusText }}</span>
            <button type="button" class="tool-info" :aria-label="`${tool.label} 线路详情`" @click="selectedTool = tool">
              <Icon name="infoCircle" size="sm" />
            </button>
          </div>
          <div class="tool-best">
            <span>最佳线路</span>
            <strong :class="{ muted: !tool.bestGroup }">{{ tool.bestGroup?.name || '暂无可用线路' }}</strong>
            <small v-if="tool.bestGroup">{{ rateLabel(tool.bestGroup) }} · {{ successLabel(tool.bestGroup) }}</small>
          </div>
          <div class="tool-card-footer">
            <span>已关联 {{ tool.linkedKeys }} 把密钥</span>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="!tool.groups.length" @click="openKeys(tool)">
              {{ tool.linkedKeys ? '管理密钥' : '关联密钥' }}
            </button>
          </div>
        </article>
      </div>

      <div v-if="workspaceError" class="workspace-error" role="alert">
        <span>{{ workspaceError }}</span>
        <button type="button" @click="loadWorkspace">重试</button>
      </div>

      <section class="routes-panel" aria-labelledby="routes-title">
        <div class="routes-panel-head">
          <div>
            <h2 id="routes-title">我的 AI 线路</h2>
            <p>数据来自当前可用分组与近 1 小时真实请求观测</p>
          </div>
          <span v-if="updatedAt">更新于 {{ updatedAt }}</span>
        </div>

        <div v-if="loading && !routeRows.length" class="routes-empty">正在读取线路…</div>
        <div v-else-if="!routeRows.length" class="routes-empty">还没有可用线路，请联系管理员开通分组权限。</div>
        <div v-else class="route-table-scroll">
          <div class="route-table" role="table" aria-label="我的 AI 线路">
            <div class="route-row route-header" role="row">
              <span>线路</span><span>近 1 小时成功率</span><span>最近观测</span><span>关联密钥</span><span>操作</span>
            </div>
            <div v-for="row in routeRows" :key="row.group.id" class="route-row" role="row">
              <div class="route-identity">
                <span class="status-dot" :class="row.operational ? 'is-success' : 'is-danger'" aria-hidden="true"></span>
                <img :src="providerIcon(row.group.platform)" alt="" />
                <div>
                  <strong>{{ row.group.name }}</strong>
                  <small>{{ platformLabel(row.group.platform) }} · {{ rateLabel(row.group) }}</small>
                </div>
              </div>
              <span :class="row.successRate === null ? 'muted' : row.operational ? 'success' : 'danger'">{{ row.successRate === null ? '近 1 小时无请求' : `${row.successRate.toFixed(1)}%` }}</span>
              <span class="route-observation">{{ observationLabel(row) }}</span>
              <span>{{ row.linkedKeys }} 把</span>
              <button type="button" class="route-action" @click="openGroupKeys(row.group.id)">查看关联密钥</button>
            </div>
          </div>
        </div>
      </section>

      <BaseDialog :show="!!selectedTool" :title="selectedTool ? `${selectedTool.label} 线路详情` : '线路详情'" @close="selectedTool = null">
        <div v-if="selectedTool" class="tool-dialog-list">
          <div v-for="group in selectedTool.groups" :key="group.id" class="tool-dialog-row">
            <div class="route-identity">
              <img :src="providerIcon(group.platform)" alt="" />
              <div><strong>{{ group.name }}</strong><small>{{ rateLabel(group) }}</small></div>
            </div>
            <span>{{ successLabel(group) }}</span>
          </div>
          <div v-if="!selectedTool.groups.length" class="routes-empty">暂无可用线路</div>
        </div>
        <template #footer>
          <button type="button" class="btn btn-secondary" @click="selectedTool = null">关闭</button>
          <button v-if="selectedTool?.groups.length" type="button" class="btn btn-primary" @click="openKeys(selectedTool)">管理密钥</button>
        </template>
      </BaseDialog>
    </section>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import userGroupsAPI from '@/api/groups'
import keysAPI from '@/api/keys'
import { getHybridPerformanceSnapshot } from '@/features/monitor-v4/api'
import type { ApiKey, Group } from '@/types'
import type { MonitorV4Group } from '@/features/monitor-v4/types'

interface ToolCard {
  id: string
  label: string
  type: string
  platforms: string[]
  icon: string
  groups: Group[]
  bestGroup: Group | null
  linkedKeys: number
  statusKind: 'success' | 'danger' | 'muted'
  statusText: string
}

const router = useRouter()
const groups = ref<Group[]>([])
const keys = ref<ApiKey[]>([])
const userRates = ref<Record<number, number>>({})
const monitorGroups = ref<MonitorV4Group[]>([])
const loading = ref(true)
const workspaceError = ref('')
const generatedAt = ref('')
const selectedTool = ref<ToolCard | null>(null)
let controller: AbortController | null = null

const toolDefinitions = [
  { id: 'codex', label: 'Codex', type: 'AI 编程', platforms: ['openai'], icon: '/xingqiao/providers/openai.svg' },
  { id: 'claude-code', label: 'Claude Code', type: 'AI 编程', platforms: ['anthropic'], icon: '/xingqiao/providers/anthropic.svg' },
  { id: 'grok', label: 'Grok', type: '通用 AI', platforms: ['grok'], icon: '/xingqiao/providers/grok.svg' },
  { id: 'deepseek', label: 'DeepSeek', type: '通用 AI', platforms: ['deepseek'], icon: '/xingqiao/providers/deepseek.svg' },
]

const monitorById = computed(() => new Map(monitorGroups.value.map(item => [item.id, item])))
const linkedKeyCounts = computed(() => {
  const counts = new Map<number, number>()
  keys.value.filter(key => key.status === 'active' && key.group_id).forEach(key => counts.set(key.group_id!, (counts.get(key.group_id!) || 0) + 1))
  return counts
})

function groupOperational(group: Group): boolean {
  const monitor = monitorById.value.get(group.id)
  return group.status === 'active' && (monitor ? monitor.current_operational : true)
}
function groupSuccess(group: Group): number | null { return monitorById.value.get(group.id)?.success_rate ?? null }
function effectiveRate(group: Group): number { return userRates.value[group.id] ?? group.rate_multiplier }
function bestGroupFor(items: Group[]): Group | null {
  return [...items].filter(groupOperational).sort((a, b) => {
    const successDiff = (groupSuccess(b) ?? -1) - (groupSuccess(a) ?? -1)
    return successDiff || effectiveRate(a) - effectiveRate(b) || a.name.localeCompare(b.name, 'zh-CN')
  })[0] || null
}

const toolCards = computed<ToolCard[]>(() => toolDefinitions.map(definition => {
  const matching = groups.value.filter(group => definition.platforms.includes(group.platform))
  const available = matching.filter(groupOperational)
  const linkedKeys = matching.reduce((sum, group) => sum + (linkedKeyCounts.value.get(group.id) || 0), 0)
  return {
    ...definition,
    groups: matching,
    bestGroup: bestGroupFor(matching),
    linkedKeys,
    statusKind: matching.length === 0 ? 'muted' : available.length ? 'success' : 'danger',
    statusText: matching.length === 0 ? '暂无可用线路' : available.length ? `可用 · ${available.length}/${matching.length} 条` : `不可用 · 0/${matching.length} 条`,
  }
}))

const routeRows = computed(() => groups.value.map(group => {
  const monitor = monitorById.value.get(group.id)
  return {
    group,
    operational: groupOperational(group),
    successRate: monitor?.success_rate ?? null,
    ttftMs: monitor?.ttft_p95_ms ?? null,
    latencyMs: monitor?.latency_p95_ms ?? null,
    linkedKeys: linkedKeyCounts.value.get(group.id) || 0,
  }
}).sort((a, b) => Number(b.operational) - Number(a.operational) || (b.successRate ?? -1) - (a.successRate ?? -1) || effectiveRate(a.group) - effectiveRate(b.group)))

const updatedAt = computed(() => generatedAt.value ? new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric', hour: '2-digit', minute: '2-digit' }).format(new Date(generatedAt.value)) : '')

async function loadAllKeys(signal: AbortSignal): Promise<ApiKey[]> {
  const first = await keysAPI.list(1, 100, undefined, { signal })
  const items = [...first.items]
  const pages = Math.min(20, Math.ceil(first.total / 100))
  for (let page = 2; page <= pages; page += 1) {
    const next = await keysAPI.list(page, 100, undefined, { signal })
    items.push(...next.items)
  }
  return items
}

async function loadWorkspace() {
  controller?.abort()
  const nextController = new AbortController()
  controller = nextController
  loading.value = true
  workspaceError.value = ''
  const results = await Promise.allSettled([
    userGroupsAPI.getAvailable(),
    userGroupsAPI.getUserGroupRates(),
    getHybridPerformanceSnapshot('1h', nextController.signal),
    loadAllKeys(nextController.signal),
  ])
  if (nextController.signal.aborted || controller !== nextController) return
  if (results[0].status === 'fulfilled') groups.value = results[0].value
  if (results[1].status === 'fulfilled') userRates.value = results[1].value
  if (results[2].status === 'fulfilled') {
    monitorGroups.value = results[2].value.groups
    generatedAt.value = results[2].value.generated_at
  }
  if (results[3].status === 'fulfilled') keys.value = results[3].value
  const failed = results.filter(result => result.status === 'rejected').length
  if (failed) workspaceError.value = failed === results.length ? 'AI 工具数据暂时不可用，请稍后重试。' : '部分线路数据暂时不可用，当前已展示可读取的信息。'
  loading.value = false
  controller = null
}

function rateLabel(group: Group): string { return `${effectiveRate(group).toFixed(2).replace(/\.00$/, '')}× 倍率` }
function successLabel(group: Group): string {
  const success = groupSuccess(group)
  return success === null ? '近 1 小时无请求' : `近 1 小时 ${success.toFixed(1)}%`
}
function platformLabel(platform: string): string {
  return ({ openai: 'OpenAI', anthropic: 'Anthropic', grok: 'xAI', deepseek: 'DeepSeek' } as Record<string, string>)[platform] || platform
}
function providerIcon(platform: string): string {
  const known = new Set(['openai', 'anthropic', 'grok', 'deepseek'])
  return known.has(platform) ? `/xingqiao/providers/${platform}.svg` : '/logo.svg'
}
function observationLabel(row: { operational: boolean; ttftMs: number | null; latencyMs: number | null }): string {
  if (!row.operational) return '当前不可用'
  if (row.ttftMs !== null) return `首字 P95 ${(row.ttftMs / 1000).toFixed(2)}s`
  if (row.latencyMs !== null) return `延迟 P95 ${(row.latencyMs / 1000).toFixed(2)}s`
  return '等待观测'
}
function openKeys(tool: ToolCard) {
  selectedTool.value = null
  const groupId = tool.bestGroup?.id || tool.groups[0]?.id
  void router.push({ path: '/keys', query: groupId ? { group_id: String(groupId) } : undefined })
}
function openGroupKeys(groupId: number) { void router.push({ path: '/keys', query: { group_id: String(groupId) } }) }

onMounted(loadWorkspace)
onBeforeUnmount(() => controller?.abort())
</script>

<style scoped>
.ai-tools-workspace { display: flex; flex-direction: column; gap: 24px; }
.ai-page-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; }
.ai-page-head h1 { margin: 0; color: var(--xq-text); font-size: clamp(24px, 2.2vw, 32px); font-weight: 680; letter-spacing: -.03em; }
.ai-page-head p { margin: 7px 0 0; color: var(--xq-secondary); font-size: 14px; }
.ai-refresh { flex: 0 0 auto; }
.tool-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 16px; }
.tool-card { min-width: 0; padding: 20px; border: 1px solid var(--xq-border); border-radius: 12px; background: var(--xq-surface); }
.tool-card-heading { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.tool-name { display: flex; min-width: 0; align-items: center; gap: 10px; }
.tool-name img { width: 24px; height: 24px; object-fit: contain; }
.tool-name h2 { margin: 0; color: var(--xq-text); font-size: 17px; font-weight: 650; }
.tool-card-heading > span { color: var(--xq-muted); font-size: 11px; }
.tool-status { display: flex; align-items: center; gap: 7px; margin-top: 22px; color: var(--xq-secondary); font-size: 12px; }
.status-dot { width: 7px; height: 7px; flex: 0 0 auto; border-radius: 50%; background: var(--xq-muted); }
.is-success .status-dot, .status-dot.is-success { background: var(--xq-success); box-shadow: 0 0 0 3px rgb(140 223 196 / .09); }
.is-danger .status-dot, .status-dot.is-danger { background: var(--xq-danger); }
.tool-info { display: grid; width: 28px; height: 28px; margin-left: auto; place-items: center; border: 0; border-radius: 7px; background: transparent; color: var(--xq-muted); }
.tool-info:hover { background: var(--xq-raised); color: var(--xq-accent); }
.tool-best { display: flex; min-height: 92px; flex-direction: column; justify-content: center; gap: 5px; margin-top: 16px; padding: 16px 0; border-top: 1px solid var(--xq-line); border-bottom: 1px solid var(--xq-line); }
.tool-best span, .tool-best small { color: var(--xq-muted); font-size: 11px; }
.tool-best strong { overflow: hidden; color: var(--xq-text); font-size: 15px; text-overflow: ellipsis; white-space: nowrap; }
.tool-best strong.muted { color: var(--xq-muted); font-weight: 500; }
.tool-card-footer { display: flex; align-items: center; justify-content: space-between; gap: 10px; padding-top: 16px; color: var(--xq-secondary); font-size: 12px; }
.workspace-error { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 12px 16px; border: 1px solid rgb(212 126 126 / .45); border-radius: 10px; background: rgb(212 126 126 / .08); color: var(--xq-danger); font-size: 13px; }
.workspace-error button { color: inherit; font-weight: 650; text-decoration: underline; }
.routes-panel { overflow: hidden; border: 1px solid var(--xq-border); border-radius: 12px; background: var(--xq-surface); }
.routes-panel-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 16px; padding: 19px 20px; border-bottom: 1px solid var(--xq-line); }
.routes-panel-head h2 { margin: 0; color: var(--xq-text); font-size: 17px; font-weight: 650; }
.routes-panel-head p { margin: 5px 0 0; color: var(--xq-muted); font-size: 11px; }
.routes-panel-head > span { color: var(--xq-muted); font-size: 11px; white-space: nowrap; }
.route-table-scroll { overflow-x: auto; }
.route-table { min-width: 900px; }
.route-row { display: grid; grid-template-columns: minmax(220px, 1.5fr) minmax(150px, .9fr) minmax(150px, .9fr) 110px 130px; min-height: 68px; align-items: center; gap: 16px; padding: 10px 20px; border-top: 1px solid var(--xq-line); color: var(--xq-secondary); font-size: 12px; }
.route-row:first-child { border-top: 0; }
.route-header { min-height: 42px; background: var(--xq-depth); color: var(--xq-muted); font-size: 11px; font-weight: 650; }
.route-identity { display: flex; min-width: 0; align-items: center; gap: 10px; }
.route-identity img { width: 22px; height: 22px; object-fit: contain; }
.route-identity div { min-width: 0; }
.route-identity strong, .route-identity small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.route-identity strong { color: var(--xq-text); font-size: 13px; }
.route-identity small { margin-top: 4px; color: var(--xq-muted); font-size: 10px; }
.success { color: var(--xq-success); }
.danger { color: var(--xq-danger); }
.muted { color: var(--xq-muted); }
.route-action { justify-self: start; border: 0; background: transparent; color: var(--xq-accent); font-size: 12px; }
.route-action:hover { color: var(--xq-core); text-decoration: underline; }
.routes-empty { padding: 48px 20px; color: var(--xq-muted); text-align: center; }
.tool-dialog-list { display: flex; flex-direction: column; gap: 8px; }
.tool-dialog-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 12px; border: 1px solid var(--xq-line); border-radius: 9px; background: var(--xq-depth); color: var(--xq-secondary); font-size: 12px; }
@media (max-width: 1350px) { .tool-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 700px) {
  .ai-page-head { align-items: stretch; flex-direction: column; }
  .ai-refresh { align-self: flex-start; }
  .tool-grid { grid-template-columns: 1fr; }
  .routes-panel-head { align-items: flex-start; flex-direction: column; }
}
</style>
