import { getCurrentUserRecord } from '@/lib/server/current-user';
import { SiteStudio } from '@/components/create/site-studio';
export const dynamic='force-dynamic';
export default async function CreatePage(){ const user=await getCurrentUserRecord(); if(!user)return <main>请从本站菜单打开生图工作站。</main>;return <SiteStudio admin={user.role==='ADMIN'} expiresAt={user.createdAt.getTime()+21600000}/>; }
