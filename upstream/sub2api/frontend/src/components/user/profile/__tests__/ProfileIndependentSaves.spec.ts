import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfileEditForm from '../ProfileEditForm.vue'
import ProfileBalanceNotifyCard from '../ProfileBalanceNotifyCard.vue'

const { updateProfile, showError, showSuccess, authUser } = vi.hoisted(() => ({
  updateProfile: vi.fn(), showError: vi.fn(), showSuccess: vi.fn(), authUser: { user: null as unknown },
}))
vi.mock('@/api', () => ({ userAPI: { updateProfile } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError, showSuccess }) }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => authUser }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

describe('independent profile settings', () => {
  beforeEach(() => { updateProfile.mockReset(); showError.mockReset(); showSuccess.mockReset() })

  it('guards the profile save locally and retains its input on failure', async () => {
    let rejectRequest!: (error: Error) => void
    updateProfile.mockImplementationOnce(() => new Promise((_, reject) => { rejectRequest = reject }))
    const wrapper = mount(ProfileEditForm, { props: { initialUsername: 'original' } })
    await wrapper.get('#username').setValue('updated')
    await wrapper.get('form').trigger('submit')
    await wrapper.get('form').trigger('submit')
    expect(updateProfile).toHaveBeenCalledTimes(1)
    rejectRequest(new Error('offline'))
    await flushPromises()
    expect((wrapper.get('#username').element as HTMLInputElement).value).toBe('updated')
  })

  it('guards threshold save locally while leaving its input intact after failure', async () => {
    let rejectRequest!: (error: Error) => void
    updateProfile.mockImplementationOnce(() => new Promise((_, reject) => { rejectRequest = reject }))
    const wrapper = mount(ProfileBalanceNotifyCard, { props: {
      enabled: true, threshold: 10, extraEmails: [], systemDefaultThreshold: 5, userEmail: '',
    } })
    const input = wrapper.get('input[type="number"]')
    await input.setValue('12.3456')
    const save = wrapper.findAll('button').find(button => button.text() === 'common.save')!
    await save.trigger('click')
    await save.trigger('click')
    expect(updateProfile).toHaveBeenCalledTimes(1)
    rejectRequest(new Error('offline'))
    await flushPromises()
    expect((input.element as HTMLInputElement).value).toBe('12.3456')
  })
})
