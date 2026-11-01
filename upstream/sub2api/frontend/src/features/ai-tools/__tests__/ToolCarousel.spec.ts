import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ToolCarousel from '../ToolCarousel.vue'

afterEach(()=>vi.restoreAllMocks())
function make(visible=4){
 const w=mount(ToolCarousel,{props:{itemCount:5},slots:{default:'<article class="tool-card">Codex</article><article class="tool-card">Claude Code</article><article class="tool-card">Gemini</article><article class="tool-card">DeepSeek</article><article class="tool-card">Grok</article>'}})
 const viewport=w.get('.tool-grid').element as HTMLElement
 viewport.style.setProperty('--tool-visible',String(visible))
 const cardWidth=224,step=240
 Object.defineProperties(viewport,{clientWidth:{configurable:true,value:step*visible-16},scrollWidth:{configurable:true,get:()=>Math.ceil(5/Number(viewport.style.getPropertyValue('--tool-visible')))*step*Number(viewport.style.getPropertyValue('--tool-visible'))-16}})
 for(const card of Array.from(viewport.children)) vi.spyOn(card,'getBoundingClientRect').mockReturnValue({width:cardWidth} as DOMRect)
 const scroll=vi.fn((options:ScrollToOptions)=>{viewport.scrollLeft=options.left||0;viewport.dispatchEvent(new Event('scroll'))})
 viewport.scrollTo=scroll
 window.dispatchEvent(new Event('resize'))
 return {w,viewport,scroll}
}
describe('whole-page tool navigation',()=>{
 for(const visible of [4,2,1]) it('moves '+visible+' slots and pads only the incomplete last page',async()=>{
  const {w,viewport,scroll}=make(visible);await flushPromises()
  const prev=w.get('button[aria-label="查看前面的 AI 工具"]'),next=w.get('button[aria-label="查看后面的 AI 工具"]')
  expect(w.findAll('.tool-slot-empty')).toHaveLength((visible-5%visible)%visible)
  expect(w.findAll('.tool-card')).toHaveLength(5)
  expect(prev.attributes('disabled')).toBeDefined()
  await next.trigger('click');await flushPromises()
  expect(scroll).toHaveBeenLastCalledWith({left:240*visible,behavior:'auto'})
  const pages=Math.ceil(5/visible)
  for(let i=1;i<pages-1;i++) await next.trigger('click')
  await flushPromises();expect(viewport.scrollLeft).toBe((pages-1)*240*visible)
  expect(next.attributes('disabled')).toBeDefined()
  await prev.trigger('click');await flushPromises();expect(viewport.scrollLeft).toBe((pages-2)*240*visible)
  w.unmount()
 })
 it('keeps rapid page requests while smooth scrolling is in flight',async()=>{
  const {w,scroll}=make(2);await flushPromises();scroll.mockImplementation(()=>{})
  const next=w.get('button[aria-label="查看后面的 AI 工具"]')
  await next.trigger('click');await next.trigger('click')
  expect(scroll).toHaveBeenLastCalledWith({left:960,behavior:'auto'})
  w.unmount()
 })
 it('rounds manual scroll to a page before navigating and supports keyboard',async()=>{
  const {w,viewport,scroll}=make(2);await flushPromises()
  await w.get('.tool-grid').trigger('wheel');viewport.scrollLeft=300
  await w.get('.tool-grid').trigger('keydown',{key:'ArrowRight'})
  expect(scroll).toHaveBeenLastCalledWith({left:960,behavior:'auto'})
  await w.get('.tool-grid').trigger('keydown',{key:'ArrowLeft'});expect(viewport.scrollLeft).toBe(480)
  w.unmount()
 })
 it('reveals the page containing a focused card, including the padded final page',async()=>{
  const {w,scroll}=make(4);await flushPromises()
  const last=w.findAll('.tool-card')[4];await last.trigger('focusin')
  expect(scroll).toHaveBeenLastCalledWith({left:960,behavior:'auto'})
  w.unmount()
 })
 it('reflows to the page containing the previous first card on resize',async()=>{
  const {w,viewport}=make(4);await flushPromises();await w.get('button[aria-label="查看后面的 AI 工具"]').trigger('click')
  viewport.style.setProperty('--tool-visible','2');Object.defineProperty(viewport,'clientWidth',{value:464})
  window.dispatchEvent(new Event('resize'));await flushPromises()
  expect(viewport.scrollLeft).toBe(960);expect(w.findAll('.tool-slot-empty')).toHaveLength(1)
  w.unmount()
 })
})
