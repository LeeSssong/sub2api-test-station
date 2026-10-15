import asyncio
import json
from datetime import datetime
from zoneinfo import ZoneInfo

TOOLS=[{'type':'function','function':{'name':name,'description':desc,'parameters':{'type':'object','properties':{arg:{'type':'string'}},'required':[arg],'additionalProperties':False}}} for name,arg,desc in [('xingqiao_public_config','query','查询星桥实时公开分组、配置和服务说明'),('xingqiao_lookup_request','request_id','按用户提供的请求ID查询对用户可见的错误原因')]]

async def request(provider,messages,tools):
 import aiohttp
 async with aiohttp.ClientSession(timeout=aiohttp.ClientTimeout(total=provider.get('timeout',30))) as s:
  async with s.post(provider['base'].rstrip('/')+'/chat/completions',headers={'Authorization':'Bearer '+provider['key']},json={'model':provider['model'],'messages':messages,'tools':tools,'stream':False}) as r:
   if r.status!=200:raise RuntimeError('model_http_'+str(r.status))
   data=await r.json()
   return data['choices'][0]['message']

class Model:
 def __init__(self,config,state,invoke,transport=request):
  self.config=config;self.state=state;self.invoke=invoke;self.transport=transport
 async def answer(self,session,text):
  history=[m for m in self.state.history(session) if m.get('role') in ('user','assistant') and isinstance(m.get('content'),str) and m['content']]
  history=history[-24:]
  while sum(len(m['content']) for m in history)>24000:history.pop(0)
  system=self.config.get('persona','')+'\n当前时间：'+datetime.now(ZoneInfo('Asia/Shanghai')).isoformat()+'\n参考知识（可能过时，实时信息以查询工具为准）：\n'+self.config.get('knowledge','')
  messages=[{'role':'system','content':system}]+history+[{'role':'user','content':text[:12000]}]
  for step in range(6):
   result=None
   for provider in self.config['providers']:
    try:
     result=await self.transport(provider,messages,TOOLS);break
    except (Exception,) as exc:
     if isinstance(exc,asyncio.CancelledError):raise
   if result is None:raise RuntimeError('all_models_failed')
   calls=result.get('tool_calls')
   if not calls:
    answer=result.get('content')
    if not isinstance(answer,str) or not answer.strip():raise RuntimeError('empty_model_reply')
    self.state.save_history(session,history+[{'role':'user','content':text[:12000]},{'role':'assistant','content':answer}])
    return answer
   messages.append({k:result[k] for k in ('role','content','tool_calls') if k in result})
   for call in calls[:8]:
    name=call.get('function',{}).get('name')
    try:
     if name not in ('xingqiao_public_config','xingqiao_lookup_request'):raise ValueError('unknown_tool')
     args=json.loads(call['function'].get('arguments','{}'))
     key='query' if name=='xingqiao_public_config' else 'request_id'
     if not isinstance(args,dict) or not isinstance(args.get(key),str):raise ValueError('invalid_arguments')
     value=await asyncio.wait_for(self.invoke(name,{key:args[key][:500]}),60)
    except Exception:value='{"error":"查询暂时不可用，请勿编造结果"}'
    messages.append({'role':'tool','tool_call_id':call['id'],'content':value})
  raise RuntimeError('tool_step_limit')

