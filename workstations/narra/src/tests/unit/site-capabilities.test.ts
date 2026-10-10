import {describe,it,expect} from 'vitest';
import {imageCapability,catalogue,validateImageOptions} from '../../lib/workstation/capabilities';
import {sameOriginAllowed} from '../../lib/workstation/origin';
describe('native model capabilities',()=>{
 it('retains Grok imagine and only recognised image models from native catalogue',()=>{expect(catalogue(['gpt-image-2','grok-imagine','grok-4','flux-dev']).map(m=>m.id)).toEqual(['gpt-image-2','grok-imagine','flux-dev'])});
 it('does not invent GPT parameters for Grok',()=>{const c=imageCapability('grok-imagine')!;expect(c.qualities).toEqual([]);expect(c.formats).toEqual([]);expect(()=>validateImageOptions(c,{size:'1024x1024',quality:'high',outputFormat:'png',count:1,referenceCount:0})).toThrow();});
 it('supports administrator-declared aliases without changing route authority',()=>{expect(catalogue(['draw-any'],{'draw-any':'grok'})[0].family).toBe('grok');});
 it('blocks cross-origin requests while accepting same-origin deployment aliases',()=>{expect(sameOriginAllowed('https://test.example','test.example','https')).toBe(true);expect(sameOriginAllowed('https://evil.example','test.example','https')).toBe(false);expect(sameOriginAllowed(null,'test.example','https')).toBe(false)});
});
