import { mount, flushPromises } from '@vue/test-utils';
import { describe, it, expect, vi } from 'vitest';
import Card from '../IntelligenceGroupCard.vue';
import { intelligenceTestsAPI, type IntelligenceGroup } from '@/api/intelligenceTests';
vi.mock('@/api/intelligenceTests', () => ({ intelligenceTestsAPI: { result: vi.fn().mockResolvedValue({ response_text: '<svg viewBox="0 0 100 100"><circle r="20"/></svg>' }) } }));
const now = Date.now();
const group = { id: 1, name: '分组', description: '', platform: 'openai', rate_multiplier: 1, model_id: 'draw', reasoning_effort: 'medium', expected_answer: '', results: [{id: 1, group_id: 1, model_id: 'draw', kind: 'pelican', verdict: 'passed', reasoning_effort: 'medium', latency_ms: 1000, attempts: 1, started_at: new Date(now).toISOString(), drawing_judgment: { status: 'incorrect', reason: '人物与车身分离' }}] } as IntelligenceGroup;
describe('drawing ribbon', () => {
 it('shows failed artwork and matching ribbon', async () => {
  const w=mount(Card,{props:{group,now,hours:24}});await flushPromises();
  expect(w.find('iframe').exists()).toBe(true);expect(w.find('.iq-drawing-ribbon').text()).toBe('未通过');
  expect(w.find('.iq-drawing-ribbon').attributes('title')).toContain('人物与车身分离');w.unmount();
 });
 it('updates pending judgment without refetching artwork', async () => {
  const pending=structuredClone(group);pending.results[0]!.drawing_judgment={status:'pending'};
  vi.mocked(intelligenceTestsAPI.result).mockClear();
  const w=mount(Card,{props:{group:pending,now,hours:24}});await flushPromises();expect(w.find('.iq-drawing-ribbon').text()).toBe('待判定');
  await w.setProps({group});await flushPromises();expect(w.find('.iq-drawing-ribbon').text()).toBe('未通过');expect(intelligenceTestsAPI.result).toHaveBeenCalledTimes(1);w.unmount();
 });
 it('does not confuse generation success with visual pass', async () => {
  const old=structuredClone(group);old.results[0]!.verdict='passed';delete old.results[0]!.drawing_judgment;
  const w=mount(Card,{props:{group:old,now,hours:24}});await flushPromises();expect(w.find('.iq-drawing-ribbon').text()).toBe('未判定');w.unmount();
 });
});

it.each([
 ['unknown', '人物：无法确认身份', '无法判断'],
 ['error', '视觉判题请求失败', '判定异常'],
] as const)('shows %s separately from generation success', async (status, reason, label) => {
 const sample=structuredClone(group);
 sample.results[0]!.drawing_judgment={status,reason};
 const w=mount(Card,{props:{group:sample,now,hours:24}});
 await flushPromises();
 expect(w.find('.iq-drawing-ribbon').text()).toBe(label);
 expect(w.find('.iq-drawing-ribbon').attributes('title')).toBe(reason);
 expect(w.find('iframe').exists()).toBe(true);
 w.unmount();
});
