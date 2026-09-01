package service

import "testing"

func TestGroupAllowsOpenAIModelUsesEnabledNonEmptyListAsAdmission(t *testing.T) {
	tests := []struct {
		name  string
		group *Group
		model string
		want  bool
	}{
		{name: "disabled list keeps native semantics", group: &Group{ModelsListConfig: GroupModelsListConfig{Models: []string{"gpt-5.4"}}}, model: "gpt-5.5", want: true},
		{name: "empty enabled list keeps native semantics", group: &Group{ModelsListConfig: GroupModelsListConfig{Enabled: true}}, model: "gpt-5.5", want: true},
		{name: "listed model is allowed after normalization", group: &Group{ModelsListConfig: GroupModelsListConfig{Enabled: true, Models: []string{" gpt-5.5 "}}}, model: " GPT-5.5 ", want: true},
		{name: "unlisted model is rejected", group: &Group{ModelsListConfig: GroupModelsListConfig{Enabled: true, Models: []string{"gpt-5.4"}}}, model: "gpt-5.5", want: false},
		{name: "nil group keeps native semantics", group: nil, model: "gpt-5.5", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GroupAllowsOpenAIModel(tt.group, tt.model); got != tt.want {
				t.Fatalf("GroupAllowsOpenAIModel() = %v, want %v", got, tt.want)
			}
		})
	}
}
