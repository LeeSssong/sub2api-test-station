import { describe, expect, it, vi } from 'vitest'
import { apiClient } from '@/api/client'
import { getLineSelectionMetrics, validateLineSelectionMetrics } from '../lineSelection'
vi.mock('@/api/client',()=>({apiClient:{get:vi.fn()}}))
const sample=()=>({contract_version:'1',window:'24h',window_start:'2026-11-02T10:00:00Z',window_end:'2026-11-03T10:00:00Z',generated_at:'2026-11-03T10:01:00Z',groups:[{id:1,sla_request_count:2,sla_success_count:1,ttft_p50_ms:0,ttft_sample_count:1}]})
it('reads only the independent real 24-hour endpoint',async()=>{
 vi.mocked(apiClient.get).mockResolvedValue({data:sample()})
 const result=await getLineSelectionMetrics()
 expect(apiClient.get).toHaveBeenCalledWith('/monitor-v4/line-selection',{signal:undefined})
 expect(result.groups[0].ttft_p50_ms).toBe(0)
})
it('keeps no samples null and rejects partial or misleading contracts',()=>{
 const empty=sample();empty.groups=[]
 expect(validateLineSelectionMetrics(empty).groups).toEqual([])
 const m=sample();m.groups[0]={...m.groups[0],ttft_p50_ms:null as unknown as number,ttft_sample_count:0}
 expect(validateLineSelectionMetrics(m).groups[0].ttft_p50_ms).toBeNull()
 for(const invalid of [{...sample(),window:'1h'},{...sample(),window_start:'2026-11-02T09:00:00Z'},{...sample(),groups:[...sample().groups,...sample().groups]}, {...sample(),groups:[{...sample().groups[0],sla_success_count:3}]},{...sample(),groups:[{...sample().groups[0],ttft_sample_count:0}]}]) expect(()=>validateLineSelectionMetrics(invalid)).toThrow()
})
