import { afterEach, describe, expect, it } from 'vitest'
import { enableAutoUnmount, mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'
import BaseDialog from '../BaseDialog.vue'

enableAutoUnmount(afterEach)

function openDialog(title: string) {
  return mount(BaseDialog, { props: { show: true, title }, global: { stubs: { Icon: true } } })
}

describe('dialog accessible titles', () => {
  it('associates simultaneous dialogs with their own title', async () => {
    openDialog('Edit account')
    openDialog('Confirm deletion')
    await nextTick()
    const dialogs = Array.from(document.body.querySelectorAll('[role="dialog"]'))
    expect(dialogs).toHaveLength(2)
    const titles = dialogs.map(dialog => document.getElementById(dialog.getAttribute('aria-labelledby')!)?.textContent)
    expect(titles).toEqual(['Edit account', 'Confirm deletion'])
  })

  it('keeps the title association stable when a dialog is reopened', async () => {
    const wrapper = openDialog('Details')
    await nextTick()
    const id = document.body.querySelector('[role="dialog"]')!.getAttribute('aria-labelledby')!
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true, title: 'Updated details' })
    expect(document.body.querySelector('[role="dialog"]')!.getAttribute('aria-labelledby')).toBe(id)
    expect(document.getElementById(id)?.textContent).toBe('Updated details')
  })

  it('uses a custom title in the dialog accessible name while keeping its label association', async () => {
    mount(BaseDialog, {
      props: { show: true, title: 'Codex 线路详情' },
      slots: { title: () => [h('img', { src: '/openai.svg', alt: '' }), 'Codex 线路详情'] },
      global: { stubs: { Icon: true } }
    })
    await nextTick()
    const dialog = document.body.querySelector('[role="dialog"]')!
    const title = document.getElementById(dialog.getAttribute('aria-labelledby')!)!
    expect(title.textContent?.trim()).toBe('Codex 线路详情')
    expect(title.querySelector('img')?.getAttribute('alt')).toBe('')
  })

  it('keeps optional helper text separate from the title and removes its ARIA reference when cleared', async () => {
    const wrapper = mount(BaseDialog, {
      props: { show: true, title: 'Codex 线路详情', description: '基于真实请求的缓存率、可用率与首 Token 表现。' },
      global: { stubs: { Icon: true } }
    })
    await nextTick()
    const dialog = document.body.querySelector('[role="dialog"]')!
    const title = document.getElementById(dialog.getAttribute('aria-labelledby')!)!
    const description = document.getElementById(dialog.getAttribute('aria-describedby')!)!
    expect(title.textContent?.trim()).toBe('Codex 线路详情')
    expect(description?.textContent).toBe('基于真实请求的缓存率、可用率与首 Token 表现。')
    expect(description.parentElement).toBe(title.parentElement)
    expect(title.contains(description)).toBe(false)
    await wrapper.setProps({ description: '' })
    expect(dialog.hasAttribute('aria-describedby')).toBe(false)
    expect(document.getElementById(description.id)).toBeNull()
  })

})
