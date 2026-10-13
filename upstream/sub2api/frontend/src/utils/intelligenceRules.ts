import type { IntelligenceRuleInput } from "@/api/admin/intelligenceRules";
import type { PelicanGroupTestPlan } from "@/api/admin/pelicanTests";
import { CANDY_PROMPT } from "./intelligenceTest";
export const INTELLIGENCE_CANDY_PROMPT = CANDY_PROMPT + "\n允许靠手感按形状挑选糖果，但不能辨别口味；可以预先确定两种形状各取多少个。";
export const INTELLIGENCE_CANDY_MODELS = ["gpt-6-astra", "gpt-6.1-sol"];
export const INTELLIGENCE_DRAWING_PROMPT =
  "请直接返回完整的单文件HTML代码：使用内联SVG绘制山姆奥特曼{动作}在{场景}的2D动画。SVG必须设置viewBox，并用SVG或CSS实现动画；不要使用canvas、JavaScript、外部资源或Markdown代码块。你不需要任何测试";
export function intelligenceRuleDefaults(): IntelligenceRuleInput {
  return {
    name: "分组智商监测",
    group_ids: [],
    model_id: "",
    reasoning_effort: "medium",
    cron_expression: "*/30 * * * *",
    enabled: true,
    candy_schedules: [],
    candy_reasoning_effort: "high",
    candy_prompt: INTELLIGENCE_CANDY_PROMPT,
    expected_answer: "21",
    candy_models: [...INTELLIGENCE_CANDY_MODELS],
    drawing_prompt: INTELLIGENCE_DRAWING_PROMPT,
    actions: ["坐着雪橇", "骑着摩托车", "乘着热气球", "划着小船", "乘着巴士"],
    scenes: ["游乐园里", "竹林里", "沙漠里", "彩虹下", "海边"],
  };
}
export function intelligenceRuleFromPlans(
  plans: PelicanGroupTestPlan[],
): IntelligenceRuleInput {
  const first = plans[0];
  const rule = first.pelican_config.intelligence;
  if (!rule) throw new Error("不是双题型规则");
  return {
    name: rule.name,
    group_ids: plans.map((p) => p.group_id),
    model_id: first.model_id,
    reasoning_effort: first.pelican_config.reasoning_effort,
    cron_expression: "*/30 * * * *",
    enabled: first.enabled,
    candy_schedules: plans.map(p => ({ group_id: p.group_id, cron_expression: p.pelican_config.intelligence?.candy_cron_expression || "*/30 * * * *" })),
    candy_reasoning_effort: rule.candy?.reasoning_effort || "high",
    candy_prompt: rule.candy?.prompt || INTELLIGENCE_CANDY_PROMPT,
    expected_answer: rule.candy?.quality?.expected_answer || "21",
    judge: rule.candy?.quality?.judge ? {...rule.candy.quality.judge} : undefined,
    candy_models: [...(rule.candy_models?.length ? rule.candy_models : INTELLIGENCE_CANDY_MODELS)],
    drawing_judge: rule.drawing_judge ? { ...rule.drawing_judge } : undefined,
    drawing_prompt: first.pelican_config.prompt,
    actions: [...rule.actions],
    scenes: [...rule.scenes],
  };
}
