import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { beforeEach, expect, it, vi } from 'vitest'
import MonitorSettingsPanel from '../MonitorSettingsPanel.vue'

const mocks = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn(), groups: vi.fn(), error: vi.fn() }))
vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return { ...actual, useI18n: () => ({ t: (key: string) => key, te: () => false }) }
})
vi.mock('@/utils/featureFlags', () => ({ isChannelMonitorV2Mode: () => true, getChannelMonitorMode: () => 'v2' }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.error, showSuccess: vi.fn(), cachedPublicSettings: null }) }))
vi.mock('@/api/admin', () => ({ adminAPI: { groups: { getAllIncludingInactive: mocks.groups } } }))
vi.mock('@/api/channelMonitorV2', () => ({ getConfig: mocks.get, updateConfig: mocks.update, MONITOR_ERROR_CATEGORIES: ['timeout'] }))

const factory = ['anthropic', 'openai', 'grok', 'kiro', 'gemini', 'antigravity']

function config() {
  return {
    version: 3,
    enabled: true,
    refresh_interval_seconds: 60,
    platforms: factory.map((platform) => ({ platform, enabled: true, models: [] })),
    group_ids: [],
    candy_probes: [],
    candy_history_limit: 100,
    ignored_error_categories: [],
    health_thresholds: {},
  }
}

beforeEach(() => {
  mocks.get.mockReset()
  mocks.update.mockReset()
  mocks.groups.mockReset()
  mocks.error.mockReset()
})

it('adds supported and custom platforms, reorders them, and deletes only a disabled custom platform', async () => {
  mocks.get.mockResolvedValue(config())
  mocks.groups.mockResolvedValue([
    { id: 9, name: 'DS', platform: 'deepseek' },
    { id: 11, name: 'Legacy', platform: 'custom_gw' },
  ])
  mocks.update.mockImplementation(async (payload) => ({ ...payload, version: 4 }))
  const wrapper = mount(MonitorSettingsPanel, { global: { stubs: { Toggle: true, Icon: true, RouterLink: true } } })
  await flushPromises()

  const optionValues = wrapper.get('[data-testid="monitor-v2-platform-select"]').findAll('option').map((option) => option.element.value)
  expect(optionValues).toEqual(expect.arrayContaining(['deepseek', 'kimi', 'typesafe', 'command_code', 'cline', 'custom_gw', 'composite']))
  expect(optionValues).not.toContain('openai')

  await wrapper.get('[data-testid="monitor-v2-platform-select"]').setValue('deepseek')
  await wrapper.get('[data-testid="monitor-v2-add-platform"]').trigger('click')
  await wrapper.get('[data-testid="monitor-v2-platform-custom"]').setValue('Custom_GW')
  await wrapper.get('[data-testid="monitor-v2-add-platform"]').trigger('click')
  await wrapper.get('[data-testid="monitor-v2-platform-custom"]').setValue('not a platform')
  await wrapper.get('[data-testid="monitor-v2-add-platform"]').trigger('click')
  expect(mocks.error).toHaveBeenCalledWith('channelMonitorV2.settings.platformInvalid')

  const rows = () => wrapper.findAll('[data-testid="monitor-v2-platform-row"]').map((row) => row.attributes('data-platform'))
  expect(rows().slice(-2)).toEqual(['deepseek', 'custom_gw'])
  expect(wrapper.find('[data-platform="openai"]').find('[data-testid="monitor-v2-delete-platform"]').exists()).toBe(false)
  expect(wrapper.get('[data-platform="custom_gw"]').get('[data-testid="monitor-v2-delete-platform"]').attributes('disabled')).toBeDefined()

  await wrapper.get('[data-platform="custom_gw"]').get('[data-testid="monitor-v2-move-up"]').trigger('click')
  expect(rows().slice(-2)).toEqual(['custom_gw', 'deepseek'])

  const draft = (wrapper.vm as unknown as { draft: { platforms: Array<{ platform: string; enabled: boolean }> } }).draft
  const custom = draft.platforms.find((item) => item.platform === 'custom_gw')
  expect(custom).toBeTruthy()
  custom!.enabled = false
  await nextTick()
  await wrapper.get('[data-platform="custom_gw"]').get('[data-testid="monitor-v2-delete-platform"]').trigger('click')
  expect(rows()).not.toContain('custom_gw')
  expect(rows()).toContain('deepseek')

  await wrapper.get('header button.btn-primary').trigger('click')
  await flushPromises()
  const saved = mocks.update.mock.calls[0][0].platforms.map((item: { platform: string }) => item.platform)
  expect(saved.slice(0, 6)).toEqual(factory)
  expect(saved.at(-1)).toBe('deepseek')
  expect(saved).not.toContain('custom_gw')
  wrapper.unmount()
})


it('defaults sync exclusion off, saves it and restores the saved value on reload', async () => {
  let saved = config() as ReturnType<typeof config> & { exclude_sync_requests?: boolean }
  mocks.get.mockImplementation(async () => saved)
  mocks.groups.mockResolvedValue([])
  mocks.update.mockImplementation(async (payload) => { saved = JSON.parse(JSON.stringify({ ...payload, version: 4 })); return saved })
  const options = { global: { stubs: { Toggle: true, Icon: true, RouterLink: true } } }
  const wrapper = mount(MonitorSettingsPanel, options)
  await flushPromises()
  expect((wrapper.get('[data-testid="monitor-v2-exclude-sync"]').element as HTMLInputElement).checked).toBe(false)
  await wrapper.get('[data-testid="monitor-v2-exclude-sync"]').setValue(true)
  await wrapper.get('header button.btn-primary').trigger('click')
  await flushPromises()
  expect(mocks.update).toHaveBeenCalledWith(expect.objectContaining({ exclude_sync_requests: true }))
  wrapper.unmount()
  const restored = mount(MonitorSettingsPanel, options)
  await flushPromises()
  expect((restored.get('[data-testid="monitor-v2-exclude-sync"]').element as HTMLInputElement).checked).toBe(true)
  restored.unmount()
})

it('removes display-model lists and duplicate candy editor while preserving legacy config on save',async()=>{
 const existing=config();existing.platforms[0].models=['old-model'];existing.candy_probes=[{group_id:7,enabled:true,model:'probe',reasoning_effort:'high',interval_minutes:1}] as never[]
 mocks.get.mockResolvedValue(existing);mocks.groups.mockResolvedValue([]);mocks.update.mockImplementation(async payload=>({...payload,version:4}))
 const w=mount(MonitorSettingsPanel,{global:{stubs:{Toggle:true,Icon:true,RouterLink:true}}});await flushPromises()
 expect(w.find('[data-platform="anthropic"] input[type="text"]').exists()).toBe(false)
 expect(w.find('[data-testid="monitor-candy-settings"]').exists()).toBe(false)
 const draft=(w.vm as unknown as {draft:{refresh_interval_seconds:number}}).draft;draft.refresh_interval_seconds=300;await nextTick();await w.get('header button.btn-primary').trigger('click');await flushPromises()
 expect(mocks.update.mock.calls[0][0]).toMatchObject({candy_probes:existing.candy_probes,platforms:expect.arrayContaining([expect.objectContaining({models:['old-model']})])});w.unmount()
})

it.each([{ status: 409 }, { response: { status: 409 } }])('keeps the monitor draft on a version conflict and requires explicit reload: %j', async (failure) => {
  const initial = config()
  initial.candy_probes = [{ group_id: 7, enabled: true, model: 'old-probe', reasoning_effort: 'high', interval_minutes: 5 }] as never[]
  mocks.get.mockResolvedValue(initial)
  mocks.groups.mockResolvedValue([])
  mocks.update.mockRejectedValueOnce(failure)
  const w = mount(MonitorSettingsPanel, { global: { stubs: { Toggle: true, Icon: true, RouterLink: true } } })
  try {
    await flushPromises()
    await w.get('[data-testid="monitor-v2-exclude-sync"]').setValue(true)
    await w.get('header button.btn-primary').trigger('click')
    await flushPromises()
    expect(mocks.get).toHaveBeenCalledTimes(1)
    expect((w.get('[data-testid="monitor-v2-exclude-sync"]').element as HTMLInputElement).checked).toBe(true)
    expect(w.get('[role="alert"]').text()).toContain('channelMonitorV2.settings.conflict')
    expect(w.get('header button.btn-primary').attributes('disabled')).toBeDefined()
    expect(mocks.update.mock.calls[0][0]).toMatchObject({ version: 3, candy_probes: initial.candy_probes })
    const fresh = { ...initial, version: 4, candy_probes: [{ group_id: 7, enabled: true, model: 'new-probe', reasoning_effort: 'high', interval_minutes: 5 }] }
    mocks.get.mockResolvedValue(fresh)
    await w.get('[data-testid="monitor-v2-settings-reload"]').trigger('click')
    await flushPromises()
    expect(w.find('[role="alert"]').exists()).toBe(false)
    expect((w.get('[data-testid="monitor-v2-exclude-sync"]').element as HTMLInputElement).checked).toBe(false)
    mocks.update.mockImplementation(async payload => ({ ...payload, version: 5 }))
    await w.get('[data-testid="monitor-v2-exclude-sync"]').setValue(true)
    await w.get('header button.btn-primary').trigger('click')
    await flushPromises()
    expect(mocks.update.mock.calls.at(-1)?.[0]).toMatchObject({ version: 4, candy_probes: fresh.candy_probes, exclude_sync_requests: true })
  } finally { w.unmount() }
})

it('preserves the draft and read version after a network failure so a retry can save it', async () => {
  mocks.get.mockResolvedValue(config())
  mocks.groups.mockResolvedValue([])
  mocks.update.mockRejectedValueOnce({ status: 0, message: 'offline' }).mockImplementationOnce(async payload => ({ ...payload, version: 4 }))
  const w = mount(MonitorSettingsPanel, { global: { stubs: { Toggle: true, Icon: true, RouterLink: true } } })
  try {
    await flushPromises()
    await w.get('[data-testid="monitor-v2-exclude-sync"]').setValue(true)
    await w.get('header button.btn-primary').trigger('click')
    await flushPromises()
    expect(mocks.get).toHaveBeenCalledTimes(1)
    expect((w.get('[data-testid="monitor-v2-exclude-sync"]').element as HTMLInputElement).checked).toBe(true)
    expect(w.get('header button.btn-primary').attributes('disabled')).toBeUndefined()
    await w.get('header button.btn-primary').trigger('click')
    await flushPromises()
    expect(mocks.update.mock.calls.at(-1)?.[0]).toMatchObject({ version: 3, exclude_sync_requests: true })
  } finally { w.unmount() }
})


it('saves the global timeline mode and restores it after remount', async () => {
  let saved: any = config()
  mocks.get.mockImplementation(async () => saved)
  mocks.groups.mockResolvedValue([])
  mocks.update.mockImplementation(async payload => { saved = JSON.parse(JSON.stringify({ ...payload, version: 4 })); return saved })
  const options = { global: { stubs: { Toggle: true, Icon: true, RouterLink: true } } }
  const w = mount(MonitorSettingsPanel, options)
  await flushPromises()
  expect(w.get('[data-testid="monitor-v2-mode-samples"]').attributes('aria-pressed')).toBe('true')
  await w.get('[data-testid="monitor-v2-mode-windows"]').trigger('click')
  await w.get('header button.btn-primary').trigger('click'); await flushPromises()
  expect(mocks.update.mock.calls[0][0]).toMatchObject({version:3,card_timeline_mode:'windows'})
  w.unmount()
  const restored = mount(MonitorSettingsPanel, options); await flushPromises()
  expect(restored.get('[data-testid="monitor-v2-mode-windows"]').attributes('aria-pressed')).toBe('true')
  restored.unmount()
})
