import 'server-only';
export async function siteProfile(token: string) {
 const response = await fetch(`${process.env.SITE_INTERNAL_ORIGIN}/api/v1/user/profile`, {headers:{Authorization:`Bearer ${token}`}, cache:'no-store', signal:AbortSignal.timeout(10000), redirect:'error'});
 if (!response.ok) throw new Error('本站登录已失效，请返回本站重新登录');
 const result = await response.json(); const user = result.data;
 if (!user || !Number.isSafeInteger(user.id) || !['user','admin'].includes(user.role)) throw new Error('本站身份校验失败');
 return user as {id:number;role:'user'|'admin';username?:string};
}
export async function checkSiteKey(key: string) {
 const response = await fetch(`${process.env.SITE_INTERNAL_ORIGIN}/v1/sub2api/billing`, {headers:{Authorization:`Bearer ${key}`},cache:'no-store',signal:AbortSignal.timeout(10000),redirect:'error'});
 if (!response.ok) throw new Error(response.status===401?'请填写有效的本站 API Key':'本站 Key 暂不可使用，请检查状态与分组');
 const data = await response.json(); if(data.object!=='sub2api.key_billing') throw new Error('本站 Key 校验失败');
}
