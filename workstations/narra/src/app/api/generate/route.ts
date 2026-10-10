import { GenerationStatus } from '@prisma/client';
import { db } from '@/lib/db';
import { requireCurrentUserRecord } from '@/lib/server/current-user';
import { parseGenerateRequest } from '@/lib/generation/parse-generate-request';
import { checkSiteKey } from '@/lib/workstation/site';
import { encryptProviderSecret } from '@/lib/providers/provider-secret';
import { persistGeneratedImage } from '@/lib/storage/persist-generated-image';
import { serializeGeneration,toPrismaGenerationType } from '@/lib/prisma-mappers';
import { jsonError,jsonOk } from '@/lib/server/http';
export async function POST(request:Request){try{
 const user=await requireCurrentUserRecord();if(Number(request.headers.get('content-length')||0)>32*1024*1024)throw new Error('上传总大小不能超过 32 MB');const body=await parseGenerateRequest(request);if(body.images.reduce((sum,file)=>sum+file.size,0)>32*1024*1024)throw new Error('参考图总大小不能超过 32 MB');
 if(!['text_to_image','image_to_image'].includes(body.generationType))throw new Error('仅支持图片生成');
 const key=body.customProvider?.apiKey?.trim();if(!key)throw new Error('请填写本站 Key');await checkSiteKey(key);
 if(body.imageUrls.length)throw new Error('请上传参考图，不接受外部图片地址');
 if(await db.generationJob.count({where:{userId:user.id,status:{in:['PENDING','PROCESSING']}}})>=3)throw new Error('最多同时排队 3 个任务');
 const urls:string[]=[];for(const image of body.images){urls.push(await persistGeneratedImage({buffer:Buffer.from(await image.arrayBuffer()),fileExtension:image.type==='image/jpeg'?'jpg':image.type==='image/webp'?'webp':'png',mimeType:image.type,userId:user.id}));}
 const job=await db.generationJob.create({data:{userId:user.id,prompt:body.prompt,model:body.model,count:body.count,size:body.size,quality:body.quality,outputFormat:body.outputFormat,outputCompression:body.outputCompression,moderation:body.moderation,generationType:toPrismaGenerationType(body.generationType),providerMode:'CUSTOM',providerBaseUrl:`${process.env.SITE_INTERNAL_ORIGIN}/v1`,providerApiKeyEncrypted:await encryptProviderSecret(key,process.env.AUTH_SECRET!),providerModels:[body.model],providerRemember:false,creditsSpent:0,contractVersion:1,handoffState:'NOT_STARTED',workerManaged:true,status:GenerationStatus.PENDING,sourceImageUrls:urls},include:{images:true,videos:true}});
 return jsonOk({generation:serializeGeneration(job)});
}catch(error){return jsonError(error instanceof Error?error.message:'创建任务失败',400)}}
