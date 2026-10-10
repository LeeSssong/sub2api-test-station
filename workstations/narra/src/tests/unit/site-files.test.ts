import {it,expect} from 'vitest';
import {mkdtemp,rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';import {join} from 'node:path';
import {writeMedia,readMedia,mediaParts} from '../../lib/workstation/files';
it('rejects traversal and returns protected local URLs instead of inline data',async()=>{const dir=await mkdtemp(join(tmpdir(),'narra-test-'));process.env.LOCAL_MEDIA_ROOT=dir;try{expect(()=>mediaParts('../escape','x.png')).toThrow();const user='site_1_12345678-1234-1234-1234-123456789abc';const url=await writeMedia(user,Buffer.from('image'),'png');expect(url).toMatch(/^\/image-workstation\/api\/workstation\/media\//);expect((await readMedia(user,url.split('/').pop()!)).toString()).toBe('image')}finally{await rm(dir,{recursive:true,force:true})}});
