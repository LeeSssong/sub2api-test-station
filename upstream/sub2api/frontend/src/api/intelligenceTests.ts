import { apiClient } from "./client";
export interface DrawingJudgment { status: "pending" | "passed" | "incorrect" | "unknown"; reason?: string; model_id?: string; deadline?: string; finished_at?: string }
export interface IntelligenceResult {
  drawing_judgment?: DrawingJudgment;
  source?: "quality_ops" | "intelligence_live";
  valid_until?: string;
  source_result_id?: number;
  source_template_id?: number;
  source_started_at?: string;
  source_finished_at?: string;
  id: number;
  group_id: number;
  model_id: string;
  kind: "candy" | "pelican";
  verdict: "passed" | "incorrect" | "abnormal" | "unknown";
  error_kind?: string;
  prompt?: string;
  expected_answer?: string;
  response_text?: string;
  reasoning_effort: string;
  latency_ms: number;
  attempts: number;
  started_at: string;
  action?: string;
  scene?: string;
  first_token_ms?: number;
  input_tokens?: number;
  output_tokens?: number;
}
export interface IntelligenceGroup {
  candy_cron_expression?: string;
  candy_running_started_at?: string;
  candy_model_ids?: string[];
  quality_template_id?: number;
  quality_source_status?: string;
  id: number;
  name: string;
  description: string;
  platform: string;
  rate_multiplier: number;
  model_id: string;
  reasoning_effort: string;
  expected_answer: string;
  next_run_at?: string;
  running_started_at?: string;
  results: IntelligenceResult[];
}
export interface IntelligenceDashboard {
  generated_at?: string;
  enabled: boolean;
  groups: IntelligenceGroup[];
}
export const intelligenceTestsAPI = {
  async dashboard(signal?: AbortSignal) {
    return (
      await apiClient.get<IntelligenceDashboard>("/intelligence-tests", {
        signal,
      })
    ).data;
  },
  async result(id: number) {
    return (
      await apiClient.get<IntelligenceResult>(
        `/intelligence-tests/results/${id}`,
      )
    ).data;
  },
};
