import {requireCurrentUserRecord} from '@/lib/server/current-user';
import {checkSiteKey} from '@/lib/workstation/site';
import {catalogue,type ImageFamily} from '@/lib/workstation/capabilities';
import {NextResponse} from 'next/server';
export async function POST(request:Request){try{
 await requireCurrentUserRecord();const {apiKey}=await request.json();if(typeof apiKey!=='string'||!apiKey.trim())throw new Error('请填写本站 Key');await checkSiteKey(apiKey.trim());
 const response=await fetch(`${process.env.SITE_INTERNAL_ORIGIN}/v1/models`,{headers:{Authorization:`Bearer ${apiKey.trim()}`},cache:'no-store',redirect:'error',signal:AbortSignal.timeout(15000)});
 if(!response.ok)throw new Error('Key 属于本站，但当前无法获取可用模型');const data=await response.json();
 const ids=(Array.isArray(data.data)?data.data:[]).map((m:{id?:unknown})=>m.id).filter((id:unknown):id is string=>typeof id==='string');
 const overrides=JSON.parse(process.env.WORKSTATION_MODEL_FAMILIES||'{}') as Record<string,ImageFamily>;
 return NextResponse.json({models:catalogue(ids,overrides),source:'sub2api-native',total:ids.length});
}catch(error){return NextResponse.json({error:error instanceof Error?error.message:'校验失败'},{status:400});}}
