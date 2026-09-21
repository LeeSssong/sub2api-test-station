import { apiClient } from '@/api/client'
export interface LineCheck { group_id: number; status: 'success'|'timeout'|'failed'|'disabled'; ttft_ms: number|null; checked_at: string }
export async function checkLines(groupIDs:number[], signal?:AbortSignal):Promise<LineCheck[]> {
  const {data}=await apiClient.post<{results:LineCheck[]}>('/monitor-v4/check',{group_ids:groupIDs},{signal,timeout:120000})
  return data.results
}
