export type MarkdownCopyState = 'idle' | 'copied' | 'failed'

export interface MarkdownCopyLabels {
  idle: string
  copied: string
  failed: string
}

export type MarkdownSupportAction =
  | {
      kind: 'copy'
      text: string
      button: HTMLButtonElement
    }
  | {
      kind: 'preview'
      src: string
      alt: string
      trigger: HTMLButtonElement
    }

export function resolveMarkdownSupportAction(
  target: EventTarget | null,
): MarkdownSupportAction | null {
  if (!(target instanceof Element)) return null

  const copyButton = target.closest<HTMLButtonElement>('button[data-copy-text]')
  if (copyButton) {
    const text = copyButton.dataset.copyText?.trim() ?? ''
    return text ? { kind: 'copy', text, button: copyButton } : null
  }

  const previewTrigger = target.closest<HTMLButtonElement>('button[data-image-preview]')
  if (!previewTrigger) return null

  const image = previewTrigger.querySelector<HTMLImageElement>('img[src]')
  if (!image?.src) return null

  return {
    kind: 'preview',
    src: image.src,
    alt: image.alt,
    trigger: previewTrigger,
  }
}

export function setMarkdownCopyState(
  button: HTMLButtonElement,
  state: MarkdownCopyState,
  labels: MarkdownCopyLabels,
) {
  button.dataset.copyState = state
  const label = button.querySelector<HTMLElement>('[data-copy-label]')
  if (label) label.textContent = labels[state]
}
