import { mkdir, writeFile, readdir, stat, statfs, readFile } from 'node:fs/promises';
import { join } from 'node:path';
import { randomBytes } from 'node:crypto';
export const fileRoot=()=>process.env.LOCAL_MEDIA_ROOT||'/data/media';
export function mediaParts(userId:string,file:string){if(!/^site_\d+_[a-f0-9-]{36}$/.test(userId)||!/^[a-f0-9]{32}\.(png|jpg|webp|gif)$/.test(file))throw new Error('无效文件');return join(fileRoot(),userId,file)}
export async function writeMedia(userId:string,body:Buffer,extension:string){
 if(!/^site_\d+_[a-f0-9-]{36}$/.test(userId)||!['png','jpg','webp','gif'].includes(extension)||body.length>50*1024*1024)throw new Error('图片大小或格式不支持');
 await mkdir(fileRoot(),{recursive:true,mode:0o700});let total=0;for(const u of await readdir(fileRoot())){for(const f of await readdir(join(fileRoot(),u))){total+=(await stat(join(fileRoot(),u,f))).size}}
 const fs=await statfs(fileRoot());if(total+body.length>512*1024*1024||fs.bavail*fs.bsize<768*1024*1024+body.length)throw new Error('临时图片空间不足，请稍后重试');
 const name=`${randomBytes(16).toString("hex")}.${extension}`;await mkdir(join(fileRoot(),userId),{recursive:true,mode:0o700});await writeFile(mediaParts(userId,name),body,{mode:0o600,flag:'wx'});return `/image-workstation/api/workstation/media/${userId}/${name}`;
}
export async function readMedia(userId:string,file:string){return readFile(mediaParts(userId,file))}
