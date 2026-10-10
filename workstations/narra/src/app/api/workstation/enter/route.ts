import { randomUUID } from 'node:crypto';
import { NextResponse } from 'next/server';
import { db } from '@/lib/db';
import { siteProfile } from '@/lib/workstation/site';
import { encryptProviderSecret } from '@/lib/providers/provider-secret';
import { createSessionToken } from '@/lib/auth/session-token';
import { SESSION_COOKIE_NAME } from '@/lib/constants';
import { SESSION_SECONDS } from '@/lib/workstation/policy';
export async function POST(request:Request) {
 try {
  const {token} = await request.json(); if(typeof token!=='string'||token.length>10000) throw new Error('本站身份校验失败');
  const profile = await siteProfile(token); const id = `site_${profile.id}_${randomUUID()}`;
  await db.user.create({data:{id,email:`${id}@session.invalid`,nickname:String(profile.id),role:profile.role==='admin'?'ADMIN':'USER',credits:0}});
  const response = NextResponse.json({expiresAt:Date.now()+SESSION_SECONDS*1000});
  const options={httpOnly:true,sameSite:'strict' as const,secure:new URL(process.env.APP_URL!).protocol==='https:',path:'/image-workstation',maxAge:SESSION_SECONDS};
  response.cookies.set(SESSION_COOKIE_NAME,await createSessionToken({userId:id,role:profile.role},process.env.AUTH_SECRET!),options);
  response.cookies.set('site_identity',await encryptProviderSecret(token,process.env.AUTH_SECRET!),options);
  return response;
 } catch { return NextResponse.json({error:'无法验证本站登录，请返回本站重试'}, {status:401}); }
}
