<template>
  <article class="iq-group">
    <header class="iq-heading">
      <h2 class="iq-group-name">{{ group.name }}</h2>
      <span class="iq-multiplier iq-mono">×{{ group.rate_multiplier }}</span
      ><span class="iq-status" :class="status">● {{ statusLabel }}</span>
    </header>
    <p class="iq-description">{{ group.description }}</p>
    <section class="iq-logic-region" aria-label="逻辑题测试">
      <div class="iq-test-heading">
        <span class="iq-test-name"><Icon name="grid" size="md" />逻辑题测试</span>
        <span class="iq-muted">分组真实检测 · {{ group.candy_cron_expression || '*/30 * * * *' }}</span>
      </div>
      <div class="iq-legend iq-logic-legend">
        <span><i class="passed" />通过</span><span><i class="incorrect" />失败</span><span><i class="abnormal" />异常</span><span><i class="unknown" />未知</span><span><i class="empty" />暂无数据</span>
        <span class="iq-legend-hint">每格 30 分钟，展示最近一次；点击查看时段内检测</span>
      </div>
      <IntelligenceTimeline v-for="model in candyModels" :key="model" kind="candy" :samples="candyFor(model)" expected-answer="" :model-id="model" compact :selected="selected?.id" :now="now" :hours="hours" :running-started-at="group.candy_running_started_at" @select="select" />
    </section>
    <section class="iq-drawing-region iq-group-grid" aria-label="绘图测试">
      <IntelligenceTimeline kind="pelican" :samples="drawing" expected-answer="" :model-id="group.model_id" :selected="selected?.id || artwork?.id" :now="now" :hours="hours" :running-started-at="group.running_started_at" @select="select" />
      <aside>
        <div class="iq-art-heading">
          <strong
            >{{
              artwork
                ? `${intelligenceDate(artwork.started_at)} 的画作`
                : "最新画作"
            }}<template v-if="artwork?.action">
              · {{ artwork.action }} · {{ artwork.scene }}</template
            ></strong
          ><span class="iq-mono">{{
            intelligenceDuration(artwork?.latency_ms)
          }}</span>
        </div>
        <div class="iq-art">
          <IntelligenceDrawingRibbon v-if="artwork" :judgment="artwork.drawing_judgment" />
          <PelicanArtworkPreview
            v-if="html"
            :html="html"
            :title="`${group.name} 检测画作`"
            :interactive="false"
          />
          <p v-else role="status">
            {{ artError || (artwork ? "正在加载画作…" : "暂无画作") }}
          </p>
          <button
            v-if="html && artwork"
            class="iq-zoom"
            @click="select(artwork)"
          >
            <Icon name="externalLink" size="sm" /> 放大
          </button>
        </div>
        <p class="iq-art-hint">
          点击任意色块可查看该次检测：题目、判定结果与模型回复。
        </p>
      </aside>
    </section>
  </article>
</template>
<script setup lang="ts">
import { computed, ref, watch } from "vue";
import Icon from "@/components/icons/Icon.vue";
import PelicanArtworkPreview from "@/components/user/pelican/PelicanArtworkPreview.vue";
import IntelligenceDrawingRibbon from "./IntelligenceDrawingRibbon.vue";
import IntelligenceTimeline from "./IntelligenceTimeline.vue";
import type {
  IntelligenceGroup,
  IntelligenceResult,
} from "@/api/intelligenceTests";
import { intelligenceTestsAPI } from "@/api/intelligenceTests";
import {
  intelligenceTimeline,
  intelligenceGroupStatus,
  intelligenceDate,
  intelligenceDuration,
} from "@/utils/intelligenceDashboard";
import { INTELLIGENCE_CANDY_MODELS } from "@/utils/intelligenceRules";
import { extractPelicanHtml } from "@/utils/pelicanHtml";
const props = defineProps<{
  group: IntelligenceGroup;
  now: number;
  hours: 24 | 72;
}>();
const emit = defineEmits<{ select: [result: IntelligenceResult] }>();
const candyModels = computed(() => props.group.candy_model_ids?.length ? props.group.candy_model_ids : INTELLIGENCE_CANDY_MODELS);
const candyFor = (model: string) => intelligenceTimeline(props.group.results, "candy", model);
const drawing = computed(() =>
  intelligenceTimeline(props.group.results, "pelican"),
);
const status = computed(() => intelligenceGroupStatus(props.group.results, candyModels.value, props.now));
const statusLabel = computed(
  () =>
    ({
      empty: "尚未检测",
      partial: "检测未齐",
      stale: "结果已过期",
      passed: "智力正常",
      incorrect: "答题错误",
      abnormal: "检测异常",
    })[status.value],
);
const selected = ref<IntelligenceResult | null>(null);
watch(
  () => props.group.results,
  (results) => {
    if (selected.value && !results.some((r) => r.id === selected.value?.id))
      selected.value = null;
  },
);
const artwork = computed(() =>
  selected.value?.kind === "pelican"
    ? drawing.value.find(r => r.id === selected.value?.id)
    : drawing.value.at(-1),
);
const html = ref("");
const artError = ref("");
let generation = 0;
watch(
  () => artwork.value?.id,
  async (id) => {
    const seq = ++generation;
    html.value = "";
    artError.value = "";
    if (!id) return;
    try {
      const r = await intelligenceTestsAPI.result(id);
      if (seq === generation) {
        html.value = extractPelicanHtml(r.response_text || "");
        if (!html.value) artError.value = "画作内容无法预览";
      }
    } catch {
      if (seq === generation) artError.value = "画作加载失败，请刷新重试";
    }
  },
  { immediate: true },
);
function select(result: IntelligenceResult) {
  selected.value = result;
  emit("select", result);
}
</script>
