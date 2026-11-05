<template>
  <AppLayout>
    <div class="intelligence-admin">
      <SmartOpsNav />
      <header class="admin-heading">
        <div>
          <p class="eyebrow">智能运维</p>
          <h2>智商监测</h2>
          <p>配置糖果题与画图规则，选择生效分组并查看检测记录。</p>
        </div>
        <div class="admin-actions">
          <RouterLink to="/intelligence-test" class="btn btn-secondary"
            >查看检测页面</RouterLink
          ><button class="btn btn-secondary" :disabled="loading" @click="load">
            刷新</button
          ><button
            class="btn btn-primary"
            data-testid="pelican-tests-create"
            @click="rulesPanel?.open()"
          >
            创建规则
          </button>
        </div>
      </header>
      <p v-if="error" role="alert" class="text-red-500">{{ error }}</p>
      <IntelligenceRulesPanel
        ref="rulesPanel"
        :plans="plans"
        :show-create="false"
        @changed="load"
      />
      <OriginalCandySettings />
      <section class="admin-history" data-testid="pelican-test-history">
        <header>
          <h3>检测记录</h3>
          <p>查看每次检测的题目、结果、耗时与模型回复。</p>
        </header>
        <div class="history-scroll">
          <table>
            <thead>
              <tr>
                <th>检测时间</th>
                <th>分组</th>
                <th>模型</th>
                <th>题型</th>
                <th>作答账号</th>
                <th>结果</th>
                <th>耗时</th>
                <th>消耗（USD）</th>
                <th />
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="result in results"
                :key="result.id"
                :data-testid="`pelican-result-${result.id}`"
              >
                <td>{{ intelligenceDate(result.started_at) }}</td>
                <td>{{ result.group_name }}</td>
                <td>{{ result.pelican_config?.model_id || "—" }}</td>
                <td>
                  {{
                    result.pelican_config?.question_kind === "candy"
                      ? "糖果题"
                      : "绘图"
                  }}
                </td>
                <td>{{ result.account_name || "未选到账号" }}</td>
                <td>
                  <span
                    :class="
                      result.status === 'success'
                        ? 'text-emerald-500'
                        : result.error_message.startsWith('answer_mismatch')
                          ? 'text-red-500'
                          : 'text-amber-500'
                    "
                    >{{
                      result.status === "success"
                        ? "通过"
                        : result.error_message.startsWith("answer_mismatch")
                          ? "答案错误"
                          : "请求异常"
                    }}</span
                  ><small v-if="result.error_message">{{
                    result.error_message
                  }}</small>
                </td>
                <td>{{ intelligenceDuration(result.latency_ms) }}</td>
                <td>
                  {{
                    result.cost_usd == null
                      ? "未记录"
                      : `$${result.cost_usd.toFixed(6)}${result.cost_incomplete ? "（部分）" : ""}`
                  }}
                </td>
                <td>
                  <button
                    class="text-primary-500"
                    :data-testid="`pelican-result-view-${result.id}`"
                    @click="openResult(result)"
                  >
                    详情
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p v-if="!results.length" class="history-empty">
          {{ loading ? "正在加载…" : "暂无检测记录" }}
        </p>
        <Pagination
          v-if="total > 0"
          :total="total"
          :page="page"
          :page-size="pageSize"
          @update:page="changePage"
          @update:page-size="changeSize"
        />
      </section>
      <p class="menu-note">
        用户页面入口请在系统设置 → 自定义菜单中添加“智商监测”。
      </p>
    </div>
    <BaseDialog
      :show="!!detail"
      title="检测详情"
      width="extra-wide"
      @close="
        detail = null;
        detailGeneration++;
      "
      ><template v-if="detail"
        ><p v-if="detailLoading" role="status">正在加载…</p>
        <p v-else-if="detailError" role="alert">{{ detailError }}</p>
        <template v-else
          ><p>
            {{ detail.group_name }} · {{ detail.pelican_config?.model_id }} ·
            {{ intelligenceDate(detail.started_at) }}
          </p>
          <p v-if="detail.error_message" class="my-3 text-amber-500">
            {{ detail.error_message }}
          </p>
          <h4 class="my-3 font-medium">发送给模型的题目</h4>
          <pre class="detail-text">{{ detail.pelican_config?.prompt }}</pre>
          <div v-if="artwork" class="detail-art">
            <PelicanArtworkPreview :html="artwork" title="检测画作" />
          </div>
          <h4 class="my-3 font-medium">模型回复</h4>
          <pre class="detail-text">{{
            detail.response_text || "未收到模型回复。"
          }}</pre>
        </template></template
      ></BaseDialog
    >
  </AppLayout>
</template>
<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from "vue";
import AppLayout from "@/components/layout/AppLayout.vue";
import SmartOpsNav from "@/components/admin/operations/SmartOpsNav.vue";
import OriginalCandySettings from "@/features/channel-monitor-v2/OriginalCandySettings.vue";
import IntelligenceRulesPanel from "@/components/admin/operations/IntelligenceRulesPanel.vue";
import BaseDialog from "@/components/common/BaseDialog.vue";
import Pagination from "@/components/common/Pagination.vue";
import PelicanArtworkPreview from "@/components/user/pelican/PelicanArtworkPreview.vue";
import {
  pelicanTestsAPI,
  type PelicanGroupTestPlan,
  type PelicanGroupTestResult,
} from "@/api/admin/pelicanTests";
import {
  intelligenceDate,
  intelligenceDuration,
} from "@/utils/intelligenceDashboard";
import { extractPelicanHtml } from "@/utils/pelicanHtml";
const plans = ref<PelicanGroupTestPlan[]>([]),
  results = ref<PelicanGroupTestResult[]>([]),
  total = ref(0),
  page = ref(1),
  pageSize = ref(20),
  loading = ref(false),
  error = ref("");
const rulesPanel = ref<InstanceType<typeof IntelligenceRulesPanel> | null>(
    null,
  ),
  detail = ref<PelicanGroupTestResult | null>(null),
  detailLoading = ref(false),
  detailError = ref("");
let detailGeneration = 0,
  generation = 0,
  alive = true,
  timer: ReturnType<typeof setInterval>;
const artwork = computed(() =>
  detail.value?.status === "success" &&
  detail.value?.pelican_config?.question_kind !== "candy"
    ? extractPelicanHtml(detail.value?.response_text || "")
    : "",
);
async function load() {
  const seq = ++generation;
  loading.value = true;
  try {
    const [rules, history] = await Promise.all([
      pelicanTestsAPI.listPlans(),
      pelicanTestsAPI.listResults(page.value, pageSize.value),
    ]);
    if (!alive || seq !== generation) return;
    plans.value = rules;
    results.value = history.items;
    total.value = history.total;
    error.value = "";
    if (page.value > Math.max(1, Math.ceil(total.value / pageSize.value))) {
      page.value = Math.max(1, Math.ceil(total.value / pageSize.value));
      void load();
    }
  } catch {
    if (alive && seq === generation)
      error.value = "加载检测规则或记录失败，请重试";
  } finally {
    if (seq === generation) loading.value = false;
  }
}
function changePage(value: number) {
  page.value = value;
  void load();
}
function changeSize(value: number) {
  pageSize.value = value;
  page.value = 1;
  void load();
}
async function openResult(result: PelicanGroupTestResult) {
  const seq = ++detailGeneration;
  detail.value = result;
  detailLoading.value = true;
  detailError.value = "";
  try {
    const full = await pelicanTestsAPI.getResult(result.id);
    if (alive && seq === detailGeneration) detail.value = full;
  } catch {
    if (alive && seq === detailGeneration)
      detailError.value = "加载检测详情失败";
  } finally {
    if (seq === detailGeneration) detailLoading.value = false;
  }
}
onMounted(() => {
  void load();
  timer = setInterval(() => {
    if (!document.hidden && !loading.value) void load();
  }, 15000);
});
onBeforeUnmount(() => {
  alive = false;
  generation++;
  detailGeneration++;
  clearInterval(timer);
});
</script>
<style scoped>
.intelligence-admin {
  max-width: 1600px;
  margin: auto;
}
.admin-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 20px;
  margin: 24px 0;
}
.eyebrow {
  font-size: 12px;
  color: #0eaa98;
}
.admin-heading h2 {
  font-size: 26px;
  font-weight: 600;
  margin: 6px 0;
}
.admin-heading p:not(.eyebrow),
.admin-history header p,
.menu-note {
  font-size: 14px;
  color: #8993a3;
}
.admin-actions {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}
.admin-history {
  border: 1px solid rgb(148 163 184 / 0.25);
  border-radius: 16px;
  overflow: hidden;
}
.admin-history header {
  padding: 22px;
}
.admin-history h3 {
  font-size: 18px;
  font-weight: 600;
}
.history-scroll {
  overflow: auto;
}
table {
  width: 100%;
  text-align: left;
  font-size: 13px;
}
th,
td {
  padding: 14px 18px;
  border-top: 1px solid rgb(148 163 184 / 0.15);
}
td small {
  display: block;
  max-width: 360px;
  overflow-wrap: anywhere;
  color: #b79247;
  margin-top: 5px;
}
.history-empty {
  padding: 30px;
  text-align: center;
  color: #8993a3;
}
.menu-note {
  margin-top: 20px;
}
.detail-text {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  padding: 16px;
  background: rgb(148 163 184 / 0.08);
  font-size: 14px;
}
.detail-art {
  height: 450px;
  margin: 18px 0;
}
@media (max-width: 850px) {
  .admin-heading {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
