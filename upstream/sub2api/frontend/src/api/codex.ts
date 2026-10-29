import { sanitizeCodexDiagnostic } from '@/utils/codexCatalogError'

export interface CodexModelsManifestResult {
  content: string
  modelCount: number
  responseBytes: number
}

const DEFAULT_CODEX_CLIENT_VERSION = '0.158.0'

function normalizeCodexBaseUrl(baseUrl: string): string {
  const fallback = typeof window !== 'undefined' ? window.location.origin : ''
  const value = (baseUrl || fallback).trim().replace(/\/+$/, '')
  if (!value) return '/v1'
  return /\/v1$/i.test(value) ? value : `${value}/v1`
}

export function buildCodexModelsManifestUrl(
  baseUrl: string,
  clientVersion = DEFAULT_CODEX_CLIENT_VERSION
): string {
  const params = new URLSearchParams({ client_version: clientVersion })
  return `${buildCodexModelCatalogUrl(baseUrl)}?${params.toString()}`
}

export function buildCodexModelCatalogUrl(baseUrl: string): string {
  return `${normalizeCodexBaseUrl(baseUrl)}/models`
}

function isCodexModelsManifest(value: unknown): value is { models: unknown[] } {
  return typeof value === 'object' && value !== null && Array.isArray((value as { models?: unknown }).models)
}

export class CodexCatalogRequestError extends Error {
  constructor(message: string, readonly status?: number, readonly code = '', readonly requestId = '') {
    super(message)
    this.name = 'CodexCatalogRequestError'
  }
}

export async function fetchCodexModelsManifest(
  baseUrl: string,
  apiKey: string,
  signal?: AbortSignal
): Promise<CodexModelsManifestResult> {
  const controller = new AbortController()
  const abort = () => controller.abort()
  if (signal?.aborted) controller.abort()
  signal?.addEventListener('abort', abort, { once: true })
  let timedOut = false
  const timeout = setTimeout(() => { timedOut = true; controller.abort() }, 15000)
  try {
    const response = await fetch(buildCodexModelsManifestUrl(baseUrl), {
      method: 'GET',
      headers: { Accept: 'application/json', Authorization: `Bearer ${apiKey}` },
      cache: 'no-store', signal: controller.signal
    })
    const text = await response.text()
    let payload: unknown
    try { payload = JSON.parse(text) } catch { /* A proxy may return HTML. Never display it. */ }
    if (!response.ok) {
      const envelope = payload && typeof payload === 'object' ? payload as Record<string, unknown> : {}
      const nested = envelope.error && typeof envelope.error === 'object' ? envelope.error as Record<string, unknown> : envelope
      const message = sanitizeCodexDiagnostic(nested.message, apiKey) || `HTTP ${response.status}`
      const code = sanitizeCodexDiagnostic(nested.code || nested.type, apiKey)
      const requestId = sanitizeCodexDiagnostic(response.headers?.get('x-request-id') || envelope.request_id, apiKey)
      throw new CodexCatalogRequestError(message, response.status, code, requestId)
    }
    if (!isCodexModelsManifest(payload)) {
      throw new CodexCatalogRequestError('Codex models response is not a valid manifest', response.status, 'INVALID_MANIFEST')
    }
    return {
      content: JSON.stringify(payload, null, 2), modelCount: payload.models.length,
      responseBytes: new TextEncoder().encode(text).byteLength
    }
  } catch (error) {
    if (timedOut) throw new CodexCatalogRequestError('Model catalog request timed out', undefined, 'TIMEOUT')
    throw error
  } finally {
    clearTimeout(timeout)
    signal?.removeEventListener('abort', abort)
  }
}
