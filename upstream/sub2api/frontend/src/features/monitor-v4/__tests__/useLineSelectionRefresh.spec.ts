import { afterEach, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import { useLineSelectionRefresh } from '../useLineSelectionRefresh'
afterEach(()=>vi.useRealTimers())
it('refreshes only open selectors on a timer/focus and stops on unmount',async()=>{
 vi.useFakeTimers()
 const active=ref(false),reload=vi.fn().mockResolvedValue(undefined)
 const wrapper=mount({setup(){useLineSelectionRefresh(()=>active.value,reload);return()=>null}})
 await vi.advanceTimersByTimeAsync(30000)
 expect(reload).not.toHaveBeenCalled()
 active.value=true
 await vi.advanceTimersByTimeAsync(30000)
 expect(reload).toHaveBeenCalledTimes(1)
 window.dispatchEvent(new Event('focus'));await Promise.resolve()
 expect(reload).toHaveBeenCalledTimes(2)
 active.value=false
 window.dispatchEvent(new Event('focus'));await Promise.resolve()
 expect(reload).toHaveBeenCalledTimes(2)
 wrapper.unmount();active.value=true
 await vi.advanceTimersByTimeAsync(30000)
 window.dispatchEvent(new Event('focus'))
 expect(reload).toHaveBeenCalledTimes(2)
})
