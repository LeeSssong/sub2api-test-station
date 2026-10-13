<template>
  <dialog
    ref="dialog"
    class="iq-dialog"
    aria-label="检测详情"
    @close="$emit('close')"
    @click="closeBackdrop"
  >
    <header class="iq-dialog-header">
      <button class="iq-close" aria-label="关闭" @click="dialog?.close()">
        ×
      </button>
      <h2>
        <span class="iq-group-name">{{ group.name }}</span
        ><span class="iq-mono">{{ result?.model_id || group.model_id }}</span
        ><span class="iq-muted"
          >· {{ summary.kind === "candy" ? "逻辑题测试" : "绘图测试" }}</span
        >
      </h2>
      <p>
        检测于 {{ intelligenceDate(summary.started_at) }} · 耗时
        {{ intelligenceDuration(summary.latency_ms) }} · 记录 #{{ summary.id }}
      </p>
    </header>
    <div class="iq-dialog-body">
      <p v-if="loading" role="status">正在加载检测详情…</p>
      <p v-else-if="error" role="alert">
        {{ error }} <button @click="$emit('retry')">重试</button>
      </p>
      <template v-else-if="result">
        <p v-if="result.source === 'quality_ops'" class="iq-source-detail">
          质量运维抽样 · 规则 #{{ result.source_template_id ?? '—' }} · 来源记录 #{{ result.source_result_id ?? '—' }}<br />
          原检测时间 {{ result.source_started_at ? intelligenceDate(result.source_started_at) : '—' }}
          <template v-if="result.source_finished_at"> → {{ intelligenceDate(result.source_finished_at) }}</template>
          <br />展示时段 {{ intelligenceDate(result.started_at) }} · 保留来源判定
        </p>
        <p v-if="result.source === 'intelligence_live'" class="iq-source-detail">分组真实检测 · 使用本轮智商监测配置</p>
        <div v-if="html" class="iq-detail-art">
          <PelicanArtworkPreview :html="html" title="检测画作" />
          <IntelligenceDrawingRibbon :judgment="result.drawing_judgment" />
        </div>
        <div class="iq-result-line">
          <span class="iq-status" :class="result.verdict"
            >● {{ result.kind === "pelican" && result.verdict === "passed" ? "生成成功" : intelligenceVerdictLabel[result.verdict]
            }}<template v-if="result.error_kind === 'no_available_account'">
              · 没有可用账号</template
            ><template v-if="result.error_kind === 'judge_inconclusive'">
              · 无法判定</template
            ></span
          ><span class="iq-muted">{{
            result.kind === "candy"
              ? `正确答案 ${result.expected_answer || "—"}`
              : [result.action, result.scene].filter(Boolean).join(" · ")
          }}</span>
        </div>
        <p v-if="result.drawing_judgment?.reason" class="iq-question">图片判定：{{ result.drawing_judgment.reason }}</p>
        <h3>发送给模型的题目</h3>
        <div class="iq-question">{{ result.prompt }}</div>
        <div class="iq-metrics">
          <div>
            延迟<strong>{{ intelligenceDuration(result.latency_ms) }}</strong>
          </div>
          <div>
            首字<strong>{{
              intelligenceDuration(result.first_token_ms)
            }}</strong>
          </div>
          <div>
            TOKEN<strong
              >{{ result.input_tokens ?? "—" }} /
              {{ result.output_tokens ?? "—" }}</strong
            >
          </div>
          <div>
            尝试次数<strong>{{ result.attempts }}</strong>
          </div>
          <div>
            推理强度<strong>{{ result.reasoning_effort || "—" }}</strong>
          </div>
        </div>
        <template v-if="result.kind === 'candy'"
          ><h3>模型回复</h3>
          <div class="iq-answer" v-html="answer"
        /></template>
        <details v-else>
          <summary>
            展开模型回复（{{ result.response_text?.length || 0 }} 字符）
          </summary>
          <pre class="iq-question">{{
            result.response_text || "未收到模型回复。"
          }}</pre>
        </details>
      </template>
    </div>
  </dialog>
</template>
<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import DOMPurify from "dompurify";
import { marked } from "marked";
import IntelligenceDrawingRibbon from "./IntelligenceDrawingRibbon.vue";
import PelicanArtworkPreview from "@/components/user/pelican/PelicanArtworkPreview.vue";
import type {
  IntelligenceGroup,
  IntelligenceResult,
} from "@/api/intelligenceTests";
import {
  intelligenceDate,
  intelligenceDuration,
  intelligenceVerdictLabel,
} from "@/utils/intelligenceDashboard";
import { extractPelicanHtml } from "@/utils/pelicanHtml";
const props = defineProps<{
  group: IntelligenceGroup;
  summary: IntelligenceResult;
  result: IntelligenceResult | null;
  loading: boolean;
  error: string;
}>();
defineEmits<{ close: []; retry: [] }>();
const dialog = ref<HTMLDialogElement | null>(null);
const html = computed(() =>
  props.result?.kind === "pelican"
    ? extractPelicanHtml(props.result.response_text || "")
    : "",
);
const answer = computed(() =>
  DOMPurify.sanitize(
    marked.parse(props.result?.response_text || "未收到模型回复。", {
      async: false,
    }) as string,
    {
      FORBID_TAGS: ["img", "video", "audio", "source", "iframe", "style"],
      FORBID_ATTR: ["style", "srcset"],
    },
  ),
);
onMounted(() => dialog.value?.showModal());
function closeBackdrop(e: MouseEvent) {
  if (!dialog.value || e.target !== dialog.value) return;
  const r = dialog.value.getBoundingClientRect();
  if (
    e.clientX < r.left ||
    e.clientX > r.right ||
    e.clientY < r.top ||
    e.clientY > r.bottom
  )
    dialog.value.close();
}
</script>
