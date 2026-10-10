import {readFile} from 'node:fs/promises';
export async function GET(){try{return new Response(new Uint8Array(await readFile('/app/source.tar.gz')),{headers:{'Content-Type':'application/gzip','Content-Disposition':'attachment; filename="xingqiao-narra-source.tar.gz"'}})}catch{return new Response('Source is available in the deployed release repository',{status:503})}}
