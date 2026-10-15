<template>
  <AppLayout class="iq-layout">
    <div class="iq-page">
      <header class="iq-page-toolbar">
        <div class="iq-controls">
          <div role="group" aria-label="检测时间范围">
            <button
              :class="{ active: hours === 24 }"
              :aria-pressed="hours === 24"
              @click="hours = 24"
            >
              24h</button
            ><button
              :class="{ active: hours === 72 }"
              :aria-pressed="hours === 72"
              @click="hours = 72"
            >
              3d
            </button>
          </div>
          <button class="btn btn-secondary iq-refresh" :disabled="loading" @click="load">
            <Icon
              name="refresh"
              size="sm"
              :class="{ 'animate-spin': loading }"
            />
            {{ t('common.refresh') }}
          </button>
        </div>
      </header>
      <div class="iq-content">
        <div class="iq-meta">
          <p>对已配置检测规则的分组做定时能力检测。</p>
          <p>更新于 {{ updatedAt }} · 每 30 秒自动刷新</p>
        </div>
        <section class="iq-overview" aria-label="检测总览">
          <div class="iq-overview-copy">
            <div>
              <h2>模型真的是满血在跑吗？</h2>
              <p>
                逻辑题按分组配置定时真实调用各模型，绘图每半小时检测一次。点击色块查看题目、实际回复与判定。
              </p>
              <div class="iq-group-totals">
                <span class="passed">● {{ groupCounts.passed }} 智力正常</span
                ><span class="incorrect"
                  >● {{ groupCounts.incorrect }} 答题错误</span
                ><span v-if="groupCounts.abnormal" class="abnormal"
                  >● {{ groupCounts.abnormal }} 检测异常</span
                ><span v-if="groupCounts.empty" class="iq-muted"
                  >● {{ groupCounts.empty }} 尚无完整结果</span
                >
              </div>
            </div>
          </div>
          <div class="iq-overview-stat">
            <span
              >逻辑题检测通过率 · 最近 {{ hours === 24 ? "24 小时" : "3 天" }}</span
            ><strong
              >{{ summary.percentage
              }}<template v-if="summary.total">%</template></strong
            ><span class="iq-next-run"
              ><Icon name="clock" size="xs" />{{ nextRunLabel }}</span
            >
          </div>
        </section>
        <p v-if="loading && !view" class="iq-message" role="status">
          正在加载智商监测…
        </p>
        <div v-if="error" class="iq-message" role="alert">
          {{ error }} <button @click="load">重试</button>
        </div>
        <p v-if="view && !view.groups.length" class="iq-message">
          暂无检测分组，请在智能运维中创建检测规则。
        </p>
        <div class="iq-groups">
          <IntelligenceGroupCard
            v-for="group in windowGroups"
            :key="`${group.id}:${group.model_id}`"
            :group="group"
            :hours="hours"
            :now="now"
            @select="open(group, $event)"
          />
        </div>
      </div>
      <IntelligenceResultDialog
        v-if="selection"
        :key="selection.summary.id"
        :group="selection.group"
        :summary="selection.summary"
        :result="detail"
        :loading="detailLoading"
        :error="detailError"
        @close="
          selection = null;
          detailGeneration++;
        "
        @retry="loadDetail"
      />
    </div>
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from "vue";
import { useI18n } from "vue-i18n";
import AppLayout from "@/components/layout/AppLayout.vue";
import Icon from "@/components/icons/Icon.vue";
import {
  intelligenceTestsAPI,
  type IntelligenceDashboard,
  type IntelligenceGroup,
  type IntelligenceResult,
} from "@/api/intelligenceTests";
import {
  intelligenceWindow,
  intelligenceCandySamples,
  intelligenceStats,
  intelligenceGroupStatus,
} from "@/utils/intelligenceDashboard";
import { INTELLIGENCE_CANDY_MODELS } from "@/utils/intelligenceRules";
import IntelligenceGroupCard from "@/components/user/intelligence/IntelligenceGroupCard.vue";
import IntelligenceResultDialog from "@/components/user/intelligence/IntelligenceResultDialog.vue";
import "@/components/user/intelligence/intelligence.css";
const { t } = useI18n();
const view = ref<IntelligenceDashboard | null>(null),
  loading = ref(false),
  error = ref(""),
  now = ref(Date.now()),
  hours = ref<24 | 72>(24),
  updated = ref<number | null>(null);
const windowGroups = computed(
  () =>
    view.value?.groups.map((g) => ({
      ...g,
      results: intelligenceWindow(g.results, now.value, hours.value),
    })) || [],
);
const summary = computed(() =>
  intelligenceStats(
    windowGroups.value.flatMap((g) =>
      intelligenceCandySamples(g.results, g.candy_model_ids?.length ? g.candy_model_ids : INTELLIGENCE_CANDY_MODELS, now.value, hours.value),
    ),
  ),
);
const groupCounts = computed(() => {
  const counts = { passed: 0, incorrect: 0, abnormal: 0, empty: 0 };
  for (const g of windowGroups.value) {
    const state = intelligenceGroupStatus(g.results, g.candy_model_ids?.length ? g.candy_model_ids : INTELLIGENCE_CANDY_MODELS, now.value);
    if (state === "passed" || state === "incorrect" || state === "abnormal")
      counts[state]++;
    else counts.empty++;
  }
  return counts;
});
const updatedAt = computed(() =>
  updated.value
    ? new Date(updated.value).toLocaleTimeString("zh-CN", { hour12: false })
    : "—",
);
const nextRunLabel = computed(() => {
  const next = view.value?.groups
    .map((g) => Date.parse(g.next_run_at || ""))
    .filter((t) => Number.isFinite(t) && t > now.value)
    .sort((a, b) => a - b)[0];
  if (!next) return "暂无待执行检测";
  const s = Math.ceil((next - now.value) / 1000);
  return `距下次检测 ${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
});
const selection = ref<{
    group: IntelligenceGroup;
    summary: IntelligenceResult;
  } | null>(null),
  detail = ref<IntelligenceResult | null>(null),
  detailLoading = ref(false),
  detailError = ref("");
let detailGeneration = 0,
  timer: ReturnType<typeof setInterval>,
  clock: ReturnType<typeof setInterval>;
const controller = new AbortController();
async function load() {
  if (loading.value) return;
  loading.value = true;
  try {
    view.value = await intelligenceTestsAPI.dashboard(controller.signal);
    if (selection.value) {
      const fresh = view.value.groups.find(g => g.id === selection.value?.group.id)?.results.find(r => r.id === selection.value?.summary.id);
      if (fresh) {
        selection.value = { ...selection.value, summary: fresh };
        if (detail.value?.id === fresh.id) detail.value = { ...detail.value, verdict: fresh.verdict, error_kind: fresh.error_kind, drawing_judgment: fresh.drawing_judgment };
      }
    }
    updated.value = Date.now();
    error.value = "";
  } catch {
    if (!controller.signal.aborted) error.value = "检测数据加载失败";
  } finally {
    loading.value = false;
  }
}
function open(group: IntelligenceGroup, summary: IntelligenceResult) {
  selection.value = { group, summary };
  void loadDetail();
}
async function loadDetail() {
  if (!selection.value) return;
  const seq = ++detailGeneration;
  detail.value = null;
  detailError.value = "";
  detailLoading.value = true;
  try {
    const r = await intelligenceTestsAPI.result(selection.value.summary.id);
    if (seq === detailGeneration) detail.value = r;
  } catch {
    if (seq === detailGeneration)
      detailError.value = "检测详情加载失败，记录可能已过期";
  } finally {
    if (seq === detailGeneration) detailLoading.value = false;
  }
}
onMounted(() => {
  void load();
  timer = setInterval(() => {
    if (!document.hidden) void load();
  }, 30000);
  clock = setInterval(() => (now.value = Date.now()), 1000);
});
onBeforeUnmount(() => {
  controller.abort();
  clearInterval(timer);
  clearInterval(clock);
  detailGeneration++;
});
</script>
