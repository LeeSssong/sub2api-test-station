package service

import "strings"

func normalizeGroupModelsListConfig(cfg GroupModelsListConfig) GroupModelsListConfig {
	out := GroupModelsListConfig{Enabled: cfg.Enabled}
	if len(cfg.Models) == 0 {
		return out
	}

	seen := make(map[string]struct{}, len(cfg.Models))
	out.Models = make([]string, 0, len(cfg.Models))
	for _, model := range cfg.Models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out.Models = append(out.Models, model)
	}
	if len(out.Models) == 0 {
		out.Models = nil
	}
	return out
}

func (g *Group) CustomModelsListEnabled() bool {
	return g != nil && g.ModelsListConfig.Enabled && len(g.ModelsListConfig.Models) > 0
}

// GroupAllowsOpenAIModel applies the group's optional model list as a request
// admission check. A disabled or empty list remains an advertisement-only
// configuration so existing native account capability semantics are preserved.
func GroupAllowsOpenAIModel(g *Group, requestedModel string) bool {
	if g == nil || !g.CustomModelsListEnabled() {
		return true
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return false
	}
	for _, allowedModel := range g.ModelsListConfig.Models {
		if strings.EqualFold(strings.TrimSpace(allowedModel), requestedModel) {
			return true
		}
	}
	return false
}
