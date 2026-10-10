import {mount,flushPromises} from '@vue/test-utils'
import {describe,it,expect,vi,beforeEach} from 'vitest'
import ImageWorkstationView from '../ImageWorkstationView.vue'
const mocks=vi.hoisted(()=>({refreshUser:vi.fn(),token:'old-token',fresh:'fresh-token'}))
vi.mock('@/stores/auth',()=>({useAuthStore:()=>({token:mocks.token,refreshUser:mocks.refreshUser})}))
vi.mock('@/api/auth',()=>({getAuthToken:()=>mocks.fresh}))
vi.mock('@/components/layout/AppLayout.vue',()=>({default:{template:'<div><slot /></div>'}}))
beforeEach(()=>{vi.restoreAllMocks();mocks.refreshUser.mockResolvedValue({id:1})})
describe('workstation entry',()=>{
 it('uses the refreshed native token before exchanging the workspace session',async()=>{
  const fetch=vi.fn().mockResolvedValue({ok:true});vi.stubGlobal('fetch',fetch)
  const wrapper=mount(ImageWorkstationView);await flushPromises()
  expect(mocks.refreshUser).toHaveBeenCalled();expect(JSON.parse(fetch.mock.calls[0][1].body).token).toBe('fresh-token')
  expect(wrapper.find('iframe').attributes('src')).toContain('theme=')
  wrapper.unmount()
 })
 it('shows the actual entry error and a retry action instead of a blank frame',async()=>{
  vi.stubGlobal('fetch',vi.fn().mockResolvedValue({ok:false,status:403,json:async()=>({error:'入口来源校验失败'})}))
  const wrapper=mount(ImageWorkstationView);await flushPromises()
  expect(wrapper.get('[role="alert"]').text()).toContain('入口来源校验失败');expect(wrapper.find('iframe').exists()).toBe(false)
  expect(wrapper.get('button').text()).toBe('重新打开');wrapper.unmount()
 })
})
