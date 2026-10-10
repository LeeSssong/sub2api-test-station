export function sameOriginAllowed(origin:string|null,host:string|null,protocol:string){
 if(!origin||!host)return false;
 try{const parsed=new URL(origin);return ['http:','https:'].includes(parsed.protocol)&&parsed.origin===new URL(`${protocol}://${host}`).origin;}catch{return false}
}
