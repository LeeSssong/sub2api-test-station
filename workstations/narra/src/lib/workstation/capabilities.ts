export type ImageFamily='gpt-image'|'grok'|'dalle'|'compatible';
export type ImageCapability={id:string;family:ImageFamily;sizes:string[];qualities:string[];formats:string[];customSize:boolean;maxCount:number;maxReferences:number;edit:boolean};
export function imageCapability(id:string,declared?:ImageFamily):ImageCapability|null{
 const name=id.toLowerCase();const family=declared||(/^gpt-image-|\/gpt-image-/.test(name)?'gpt-image':/^grok-imagine(?!.*video)/.test(name)?'grok':/dall-e/.test(name)?'dalle':/flux|seedream|imagen|image/.test(name)&&!/video/.test(name)?'compatible':null);
 if(!family)return null;
 const base={id,family,sizes:['auto'],qualities:[] as string[],formats:[] as string[],customSize:false,maxCount:1,maxReferences:0,edit:false};
 if(family==='gpt-image')return {...base,sizes:['auto','1024x1024','1536x1024','1024x1536',...(/gpt-image-2/.test(name)?['2048x2048','4096x4096']:[])],qualities:['auto','low','medium','high'],formats:['png','jpeg','webp'],customSize:/gpt-image-2/.test(name),maxCount:4,maxReferences:16,edit:true};
 if(family==='grok')return {...base,sizes:['auto','1024x1024','1024x576','576x1024','2048x2048','2048x1152','1152x2048'],maxCount:4,maxReferences:1,edit:true};
 if(family==='dalle')return {...base,sizes:['1024x1024',...(/dall-e-3/.test(name)?['1792x1024','1024x1792']:['512x512'])],edit:/dall-e-2/.test(name),maxReferences:/dall-e-2/.test(name)?1:0};
 return base;
}
export function catalogue(ids:string[],overrides:Record<string,ImageFamily>={}){return [...new Set(ids)].map(id=>imageCapability(id,overrides[id])).filter((v):v is ImageCapability=>v!==null)}
export function validateImageOptions(c:ImageCapability,o:{size:string;quality:string;outputFormat:string;count:number;referenceCount:number}){
 if(!c.sizes.includes(o.size)&&!(c.customSize&&/^\d+x\d+$/.test(o.size)))throw new Error('当前模型不支持该尺寸');
 if(!(c.qualities.length?c.qualities:['auto']).includes(o.quality))throw new Error('当前模型不支持该质量参数');
 if(!(c.formats.length?c.formats:['png']).includes(o.outputFormat))throw new Error('当前模型不支持该输出格式');
 if(o.count<1||o.count>(o.referenceCount?1:c.maxCount))throw new Error('当前模型不支持该生成张数');
 if(o.referenceCount&&(!c.edit||o.referenceCount>c.maxReferences))throw new Error('当前模型不支持该参考图数量');
}
