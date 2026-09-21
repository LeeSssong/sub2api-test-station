import { mount, flushPromises } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import BaseDialog from '../BaseDialog.vue'
afterEach(()=>{document.body.innerHTML='';document.body.classList.remove('modal-open')})
describe('teleported user dialog semantics',()=>{
 it('keeps unique accessible titles for two instances',async()=>{const root=mount(defineComponent({components:{BaseDialog},template:'<div><BaseDialog :show="true" title="线路详情"/><BaseDialog :show="true" title="创建线路密钥"/></div>'}),{attachTo:document.body,global:{provide:{'starbridge-user':ref(true)}}});await flushPromises();const dialogs=[...document.querySelectorAll('[role="dialog"]')];expect(new Set(dialogs.map(d=>d.getAttribute('aria-labelledby'))).size).toBe(2);expect(dialogs.every(d=>d.classList.contains('xq-dialog'))).toBe(true);root.unmount()})
 it('traps keyboard focus in a user dialog',async()=>{const w=mount(BaseDialog,{attachTo:document.body,props:{show:true,title:'线路详情'},slots:{default:'<button id="last-action">关联密钥</button>'},global:{provide:{'starbridge-user':ref(true)}}});await flushPromises();const last=document.querySelector<HTMLButtonElement>('#last-action')!;last.focus();last.dispatchEvent(new KeyboardEvent('keydown',{key:'Tab',bubbles:true,cancelable:true}));expect(document.activeElement?.getAttribute('aria-label')).toBe('关闭');w.unmount()})
})
