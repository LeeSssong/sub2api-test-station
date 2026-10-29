import unixTemplate from './codexQuickConfig/switch.sh?raw'
import windowsTemplate from './codexQuickConfig/switch.ps1?raw'
import type { GroupPlatform } from '@/types'

export const CODEX_QUICK_CONFIG_PLATFORMS = [
  'openai',
  'anthropic',
  'gemini',
  'antigravity',
  'grok',
  'kimi',
  'zhipu',
  'deepseek',
  'minimax',
  'opencode_go',
  'composite'
] as const satisfies readonly GroupPlatform[]

export type CodexQuickConfigPlatform = typeof CODEX_QUICK_CONFIG_PLATFORMS[number]

const codexQuickConfigPlatforms = new Set<GroupPlatform>(CODEX_QUICK_CONFIG_PLATFORMS)

export function isCodexQuickConfigPlatform(
  platform: GroupPlatform | null | undefined
): platform is CodexQuickConfigPlatform {
  return platform != null && codexQuickConfigPlatforms.has(platform)
}

export interface CodexQuickConfigInput {
  apiKey: string
  baseUrl: string
  platform?: CodexQuickConfigPlatform | null
  modelCatalogContent?: string
  modelCatalogPath?: string
}

const preferredModels: Record<CodexQuickConfigPlatform, string> = {
  openai: 'gpt-5.5',
  anthropic: 'claude-sonnet-4-6',
  gemini: 'gemini-2.5-pro',
  antigravity: 'claude-sonnet-4-6',
  grok: 'grok-4.5',
  kimi: 'kimi-k2.5',
  zhipu: 'glm-4.7',
  deepseek: 'deepseek-v4-pro',
  minimax: 'MiniMax-M3',
  opencode_go: 'glm-5.3',
  composite: 'gpt-5.5'
}

const providerNames: Record<CodexQuickConfigPlatform, string> = {
  openai: 'OpenAI',
  anthropic: 'Sub2API Anthropic',
  gemini: 'Sub2API Gemini',
  antigravity: 'Sub2API Antigravity',
  grok: 'Sub2API Grok',
  kimi: 'Sub2API Kimi',
  zhipu: 'Sub2API Zhipu',
  deepseek: 'Sub2API DeepSeek',
  minimax: 'Sub2API MiniMax',
  opencode_go: 'Sub2API OpenCode',
  composite: 'Sub2API Composite'
}

function escapeTomlBasicString(value: string): string {
  return value
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"')
    // TOML basic strings forbid unescaped control characters.
    // eslint-disable-next-line no-control-regex
    .replace(/[\x00-\x1f\x7f]/g, (character) => `\\u${character.charCodeAt(0).toString(16).padStart(4, '0')}`)
}

export function normalizeCodexBaseUrl(baseUrl: string): string {
  const trimmed = baseUrl.trim().replace(/\/+$/, '')
  if (!trimmed) return ''
  return /\/v1$/i.test(trimmed) ? trimmed : `${trimmed}/v1`
}

export function encodeUtf8Base64(value: string): string {
  const bytes = new TextEncoder().encode(value)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

function encodeUtf16LeBase64(value: string): string {
  let binary = ''
  for (let index = 0; index < value.length; index += 1) {
    const codeUnit = value.charCodeAt(index)
    binary += String.fromCharCode(codeUnit & 0xff, codeUnit >> 8)
  }
  return btoa(binary)
}

export function buildCodexQuickConfigToml(input: CodexQuickConfigInput): string {
  const platform = input.platform || 'openai'
  const model = preferredModels[platform] || preferredModels.openai
  const provider = 'xingqiao'
  const providerName = providerNames[platform] || 'Sub2API'
  const catalogLine = input.modelCatalogContent
    ? `model_catalog_json = "${escapeTomlBasicString(input.modelCatalogPath || '~/.codex/codex-models.json')}"\n`
    : ''

  return `# Xingqiao connection settings
model_provider = "${provider}"
model = "${escapeTomlBasicString(model)}"
review_model = "${escapeTomlBasicString(model)}"
${catalogLine}
[model_providers.${provider}]
name = "${escapeTomlBasicString(providerName)}"
base_url = "${escapeTomlBasicString(normalizeCodexBaseUrl(input.baseUrl))}"
wire_api = "responses"
requires_openai_auth = false
experimental_bearer_token = "${escapeTomlBasicString(input.apiKey)}"
http_headers = { "x-openai-actor-authorization" = "local-image-extension" }
supports_websockets = false

`
}

export function buildMacLinuxCodexQuickConfigScript(input: CodexQuickConfigInput): string {
  return unixTemplate
    .replace('@@CONFIG@@', encodeUtf8Base64(buildCodexQuickConfigToml({ ...input, modelCatalogContent: undefined })))
    .replace('@@CATALOG@@', input.modelCatalogContent ? encodeUtf8Base64(input.modelCatalogContent) : '')
}

export function buildMacCodexQuickConfigScript(input: CodexQuickConfigInput): string {
  return buildMacLinuxCodexQuickConfigScript(input)
}

export function buildWindowsCmdCodexQuickConfigScript(input: CodexQuickConfigInput): string {
  const script = windowsTemplate
    .replace('@@CONFIG@@', encodeUtf8Base64(buildCodexQuickConfigToml({ ...input, modelCatalogContent: undefined })))
    .replace('@@CATALOG@@', input.modelCatalogContent ? encodeUtf8Base64(input.modelCatalogContent) : '')
    .replace('@@MESSAGES@@', encodeUtf8Base64(JSON.stringify({
      menu: '星桥AI · Codex 配置工具\n\n操作前请完全退出 Codex。\n1. 连接星桥AI\n2. 切回官方 Codex\n0. 退出',
      prompt: '请选择', success: '配置已保存，请完全退出并重新启动 Codex。',
      official: '已切回官方；如有需要，请在 Codex 中完成官方登录。',
      failure: '配置未完成，请查看具体错误。', backup: '切换前备份：', close: '按回车键关闭窗口。'
    })))
  const bootstrap = `$ErrorActionPreference = 'Stop'; try { $source = [IO.File]::ReadAllText($env:SUB2API_CODEX_SCRIPT); $marker = '# SUB2API_POWERSHELL'; & ([ScriptBlock]::Create($source.Substring($source.LastIndexOf($marker) + $marker.Length))) } catch { [Console]::Error.WriteLine($_.Exception.Message); exit 1 }`
  return `@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "ERRORLEVEL="
set "SUB2API_CODEX_SCRIPT=%~f0"
set "SUB2API_POWERSHELL=%SystemRoot%\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"
if exist "%SystemRoot%\\Sysnative\\WindowsPowerShell\\v1.0\\powershell.exe" set "SUB2API_POWERSHELL=%SystemRoot%\\Sysnative\\WindowsPowerShell\\v1.0\\powershell.exe"
if not exist "%SUB2API_POWERSHELL%" goto :missing_powershell
"%SUB2API_POWERSHELL%" -NoLogo -NoProfile -ExecutionPolicy Bypass -EncodedCommand ${encodeUtf16LeBase64(bootstrap)}
set "SUB2API_CODEX_EXIT=%ERRORLEVEL%"
if not "%SUB2API_CODEX_EXIT%"=="0" pause
endlocal & exit /b %SUB2API_CODEX_EXIT%
:missing_powershell
echo Windows PowerShell is unavailable. Enable it and retry.
pause
endlocal & exit /b 1
# SUB2API_POWERSHELL
${script}`.replace(/\r?\n/g, '\r\n')
}
