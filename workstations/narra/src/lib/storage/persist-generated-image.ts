import 'server-only';
import { writeMedia } from '@/lib/workstation/files';
type Input={userId:string;buffer:Buffer;fileExtension?:string;mimeType?:string}|{userId:string;b64Json:string;mimeType?:string}|{userId:string;url:string};
export async function persistGeneratedImage(input:Input):Promise<string>{if('url'in input)throw new Error('不接受外部图片地址');return writeMedia(input.userId,'buffer'in input?input.buffer:Buffer.from(input.b64Json,'base64'),'fileExtension'in input?input.fileExtension||'png':'png')}
