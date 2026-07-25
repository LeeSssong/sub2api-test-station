import { afterEach, describe, expect, it } from 'vitest'
import {
  resolveMarkdownSupportAction,
  rewriteRelativeImageSources,
  setMarkdownCopyState,
} from '../markdownSupportActions'

afterEach(() => {
  document.body.innerHTML = ''
})

describe('markdown support actions', () => {
  it('rewrites relative raw HTML image sources without touching external assets', () => {
    const html = `
      <img src="qq-group.png" alt="QQ群二维码">
      <img src="./nested/image name.png?size=large#preview" alt="嵌套图片">
      <img src="https://cdn.example.com/remote.png" alt="外部图片">
      <img src="/already-absolute.png" alt="绝对路径">
    `

    const result = rewriteRelativeImageSources(
      html,
      (src) => `/api/pages/support/images/${src.replace(' ', '%20')}`,
    )
    const document = new DOMParser().parseFromString(result, 'text/html')
    const sources = Array.from(document.querySelectorAll('img'), (image) =>
      image.getAttribute('src'),
    )

    expect(sources).toEqual([
      '/api/pages/support/images/qq-group.png',
      '/api/pages/support/images/./nested/image%20name.png?size=large#preview',
      'https://cdn.example.com/remote.png',
      '/already-absolute.png',
    ])
  })

  it('resolves a marked copy button from a nested label', () => {
    document.body.innerHTML = `
      <button type="button" data-copy-text="1080152144">
        <span data-copy-label>复制群号</span>
      </button>
    `

    const label = document.querySelector('[data-copy-label]')
    const action = resolveMarkdownSupportAction(label)

    expect(action).toMatchObject({
      kind: 'copy',
      text: '1080152144',
    })
    expect(action?.kind === 'copy' && action.button.tagName).toBe('BUTTON')
  })

  it('resolves a marked QR preview from its nested image', () => {
    document.body.innerHTML = `
      <button type="button" data-image-preview>
        <img src="/api/pages/support/images/qq.png" alt="QQ群二维码">
      </button>
    `

    const image = document.querySelector('img')

    expect(resolveMarkdownSupportAction(image)).toMatchObject({
      kind: 'preview',
      src: 'http://localhost:3000/api/pages/support/images/qq.png',
      alt: 'QQ群二维码',
    })
  })

  it('ignores unmarked elements and incomplete actions', () => {
    document.body.innerHTML = `
      <button type="button">普通按钮</button>
      <button type="button" data-copy-text=""></button>
      <button type="button" data-image-preview></button>
    `

    const buttons = document.querySelectorAll('button')

    expect(resolveMarkdownSupportAction(buttons[0])).toBeNull()
    expect(resolveMarkdownSupportAction(buttons[1])).toBeNull()
    expect(resolveMarkdownSupportAction(buttons[2])).toBeNull()
    expect(resolveMarkdownSupportAction(null)).toBeNull()
  })

  it('updates only the selected copy button state and label', () => {
    document.body.innerHTML = `
      <button id="first" type="button" data-copy-text="1080152144">
        <span data-copy-label>复制群号</span>
      </button>
      <button id="second" type="button" data-copy-text="other">
        <span data-copy-label>复制</span>
      </button>
    `
    const first = document.querySelector<HTMLButtonElement>('#first')!
    const second = document.querySelector<HTMLButtonElement>('#second')!
    const labels = {
      idle: '复制群号',
      copied: '已复制',
      failed: '复制失败',
    }

    setMarkdownCopyState(first, 'copied', labels)

    expect(first.dataset.copyState).toBe('copied')
    expect(first.textContent).toContain('已复制')
    expect(second.dataset.copyState).toBeUndefined()
    expect(second.textContent).toContain('复制')

    setMarkdownCopyState(first, 'failed', labels)
    expect(first.dataset.copyState).toBe('failed')
    expect(first.textContent).toContain('复制失败')
  })
})
