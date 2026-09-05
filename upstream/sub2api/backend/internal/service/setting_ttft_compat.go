package service

import "strings"

func normalizeOpenAITTFTMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), OpenAITTFTModeVisible) {
		return OpenAITTFTModeVisible
	}
	return OpenAITTFTModeSemantic
}
