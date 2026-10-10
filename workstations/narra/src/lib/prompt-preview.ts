export function buildPromptPreview(prompt: string, maxLength = 160) {
  const normalized = prompt.replace(/\s+/g, " ").trim();
  if (normalized.length <= maxLength) {
    return normalized;
  }

  return `${normalized.slice(0, maxLength).trimEnd()}...`;
}

export function buildPromptClipboardText(
  prompt: string,
  negativePrompt?: string | null,
) {
  const sections = [`主提示词:\n${prompt}`];

  if (negativePrompt?.trim()) {
    sections.push(`负向提示词:\n${negativePrompt.trim()}`);
  }

  return sections.join("\n\n");
}
