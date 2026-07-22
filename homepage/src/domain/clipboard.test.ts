import { describe, expect, it, vi } from 'vitest'
import { copyText } from './clipboard'

describe('copyText', () => {
  it('uses the asynchronous clipboard API when available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)

    await expect(copyText('1080152144', { writeText }, document)).resolves.toBe('copied')
    expect(writeText).toHaveBeenCalledWith('1080152144')
    expect(document.querySelector('[data-copy-fallback]')).not.toBeInTheDocument()
  })

  it('selects a temporary readonly input when clipboard access is rejected', async () => {
    const writeText = vi.fn().mockRejectedValue(new DOMException('denied'))
    const select = vi.spyOn(HTMLInputElement.prototype, 'select')

    await expect(copyText('https://api.example.com', { writeText }, document)).resolves.toBe('selected')

    expect(select).toHaveBeenCalledOnce()
    expect(document.querySelector('[data-copy-fallback]')).not.toBeInTheDocument()
  })
})
