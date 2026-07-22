export interface ClipboardWriter {
  writeText(value: string): Promise<void>
}

export async function copyText(
  value: string,
  clipboard: ClipboardWriter | null | undefined,
  ownerDocument: Document,
): Promise<'copied' | 'selected'> {
  if (clipboard) {
    try {
      await clipboard.writeText(value)
      return 'copied'
    } catch {
      // Continue with the selection fallback below.
    }
  }

  const input = ownerDocument.createElement('input')
  input.value = value
  input.readOnly = true
  input.dataset.copyFallback = 'true'
  input.setAttribute('aria-hidden', 'true')
  input.style.position = 'fixed'
  input.style.inset = '0 auto auto -9999px'
  ownerDocument.body.append(input)

  try {
    input.select()
    return 'selected'
  } finally {
    input.remove()
  }
}
