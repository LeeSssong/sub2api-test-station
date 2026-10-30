/** Only short, sanitized diagnostics may enter the UI or copied support details. */
export function sanitizeCodexDiagnostic(value: unknown, apiKey = ''): string {
  let text = typeof value === 'string' ? value : ''
  if (apiKey) text = text.split(apiKey).join('[REDACTED]')
  return text
    .replace(/\bBearer\s+[^\s,;"'}]+/gi, 'Bearer [REDACTED]')
    .replace(/\bsk-[A-Za-z0-9_-]+/g, '[REDACTED]')
    .replace(/\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+/g, '[REDACTED]')
    .replace(/([?&](?:key|api_key|token|access_token)=)[^\s&#]*/gi, '$1[REDACTED]')
    // Strip terminal/control codes from untrusted diagnostics.
    // eslint-disable-next-line no-control-regex
    .replace(/[\u0000-\u001f\u007f]/g, ' ')
    .slice(0, 500)
}

export function codexCatalogErrorDetails(error: unknown, apiKey: string) {
  const data = error && typeof error === 'object' ? error as Record<string, unknown> : {}
  const message = sanitizeCodexDiagnostic(data.message, apiKey)
  const code = sanitizeCodexDiagnostic(data.code, apiKey)
  const status = typeof data.status === 'number' ? data.status : undefined
  const requestId = sanitizeCodexDiagnostic(data.requestId, apiKey)
  const balance = /INSUFFICIENT_BALANCE|insufficient user balance/i.test(`${code} ${message}`)
  return { message, code, status, requestId, balance }
}
