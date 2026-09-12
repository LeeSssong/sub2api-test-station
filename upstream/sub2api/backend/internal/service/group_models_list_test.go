package service

import "testing"

func TestGroupModelAllowlistUsesEnabledListAsAdmission(t *testing.T) {
	tests := []struct {
		name  string
		group *Group
		model string
		want  bool
	}{
		{name: "disabled list keeps native semantics", group: &Group{ModelAllowlist: GroupModelAllowlist{Models: []string{"gpt-5.4"}}}, model: "gpt-5.5", want: true},
		{name: "empty enabled list is closed", group: &Group{ModelAllowlist: GroupModelAllowlist{Enabled: true}}, model: "gpt-5.5", want: false},
		{name: "listed model is allowed after normalization", group: &Group{ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{" gpt-5.5 "}}}, model: " GPT-5.5 ", want: true},
		{name: "unlisted model is rejected", group: &Group{ModelAllowlist: GroupModelAllowlist{Enabled: true, Models: []string{"gpt-5.4"}}}, model: "gpt-5.5", want: false},
		{name: "nil group keeps native semantics", group: nil, model: "gpt-5.5", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.group == nil || !tt.group.ModelAllowlistEnabled() || tt.group.ModelAllowlist.Allows(tt.model); got != tt.want {
				t.Fatalf("GroupModelAllowlist admission = %v, want %v", got, tt.want)
			}
		})
	}
}
