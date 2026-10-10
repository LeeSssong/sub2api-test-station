import pg from 'pg';
import {readdir,rm,stat} from 'node:fs/promises';
import {join} from 'node:path';
const pool=new pg.Pool({connectionString:process.env.DATABASE_URL});const root=process.env.LOCAL_MEDIA_ROOT||'/data/media';
async function clean(){const client=await pool.connect();try{const lock=await client.query("SELECT pg_try_advisory_lock(92661010) AS locked");if(!lock.rows[0].locked)return;
 await client.query(`UPDATE "GenerationJob" SET status='FAILED',"completedAt"=NOW(),"providerApiKeyEncrypted"=NULL,"errorMessage"='会话已过期',"cancelRequestedAt"=NOW() WHERE status='PENDING' AND "userId" IN (SELECT id FROM "User" WHERE "createdAt"<NOW()-INTERVAL '6 hours')`);
 await client.query(`UPDATE "GenerationJob" SET "providerApiKeyEncrypted"=NULL WHERE status IN ('SUCCEEDED','FAILED')`);
 const expired=await client.query(`SELECT id FROM "User" WHERE "createdAt"<NOW()-INTERVAL '6 hours' AND id LIKE 'site_%'`);for(const {id} of expired.rows)if(/^site_\d+_[a-f0-9-]{36}$/.test(id))await rm(join(root,id),{recursive:true,force:true});
 const dirs=await readdir(root).catch(()=>[]);for(const id of dirs){if(!/^site_\d+_[a-f0-9-]{36}$/.test(id))continue;const user=await client.query('SELECT 1 FROM "User" WHERE id=$1',[id]);if(!user.rowCount&&(await stat(join(root,id))).mtimeMs<Date.now()-21600000)await rm(join(root,id),{recursive:true,force:true})}
 await client.query(`DELETE FROM "User" WHERE "createdAt"<NOW()-INTERVAL '24 hours' AND id LIKE 'site_%'`);
 }finally{await client.query('SELECT pg_advisory_unlock(92661010)').catch(()=>{});client.release()}}
for(;;){await clean().catch(()=>console.error('site media cleanup failed'));await new Promise(resolve=>setTimeout(resolve,60000))}
