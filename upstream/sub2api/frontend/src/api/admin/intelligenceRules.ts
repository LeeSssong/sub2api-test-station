import { apiClient } from "../client";
export interface IntelligenceJudge {
  group_id: number;
  model_id: string;
  prompt: string;
}
export interface IntelligenceRuleInput {
  name: string;
  group_ids: number[];
  model_id: string;
  reasoning_effort: string;
  cron_expression: string;
  enabled: boolean;
  candy_schedules: { group_id: number; cron_expression: string }[];
  candy_reasoning_effort: string;
  candy_models: string[];
  candy_prompt?: string;
  expected_answer?: string;
  judge?: IntelligenceJudge;
  drawing_judge?: IntelligenceJudge;
  drawing_prompt: string;
  actions: string[];
  scenes: string[];
}
export const intelligenceRulesAPI = {
  async save(input: IntelligenceRuleInput, id?: string) {
    return (
      await (id
        ? apiClient.put(`/admin/intelligence-rules/${id}`, input)
        : apiClient.post("/admin/intelligence-rules", input))
    ).data as { id: string };
  },
  async remove(id: string) {
    await apiClient.delete(`/admin/intelligence-rules/${id}`);
  },
};
