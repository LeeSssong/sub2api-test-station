import { describe, expect, it } from "vitest";
import {
  intelligenceRuleDefaults,
  intelligenceRuleFromPlans,
} from "../intelligenceRules";
import type { PelicanGroupTestPlan } from "@/api/admin/pelicanTests";
describe("intelligence rules", () => {
  it("defaults to standalone candy configuration and two models", () => {
    const input = intelligenceRuleDefaults();
    expect(input.candy_schedules).toEqual([]);
    expect(input.candy_models).toEqual(["gpt-6-astra", "gpt-6.1-sol"]);
    expect(input.candy_prompt).toContain("允许");
    expect(input.expected_answer).toBe("21");
    expect(input.candy_reasoning_effort).toBe("high");
    expect(input.drawing_prompt).toContain("山姆奥特曼{动作}在{场景}");
    expect(input.group_ids).toEqual([]);
  });
  it("edits all groups of a rule without mutating saved settings", () => {
    const config = {
      prompt: "draw {动作} {场景}",
      reasoning_effort: "medium",
      parallel_count: 1,
      intelligence: {
        id: "r",
        drawing_judge: { group_id: 8, model_id: "vision", prompt: "" },
        name: "Rule",
        actions: ["a"],
        scenes: ["s"],
        candy: {
          prompt: "question",
          reasoning_effort: "medium",
          parallel_count: 1,
          quality: {
            expected_answer: "42",
            action: "observe_only",
            judge: { group_id: 4, model_id: "judge", prompt: "compare" },
          },
        },
      },
    };
    const plans = [
      {
        group_id: 1,
        model_id: "model",
        cron_expression: "*/30 * * * *",
        enabled: true,
        pelican_config: config,
      },
      { group_id: 2, pelican_config: config },
    ] as PelicanGroupTestPlan[];
    const draft = intelligenceRuleFromPlans(plans);
    expect(draft.group_ids).toEqual([1, 2]);
    expect(draft.candy_prompt).toBe("question");
    expect(draft.expected_answer).toBe("42");
    expect(draft.candy_schedules).toEqual([{ group_id: 1, cron_expression: "*/30 * * * *" }, { group_id: 2, cron_expression: "*/30 * * * *" }]);
    expect(draft.candy_models).toEqual(["gpt-6-astra", "gpt-6.1-sol"]);
    expect(draft.drawing_judge?.model_id).toBe("vision");
    draft.drawing_judge!.model_id="changed";
    expect(config.intelligence.drawing_judge.model_id).toBe("vision");
    draft.actions.push("b");
    expect(config.intelligence.actions).toEqual(["a"]);
  });
});
