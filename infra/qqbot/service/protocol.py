from __future__ import annotations
import asyncio
import json
import logging
import uuid

logger=logging.getLogger(__name__)

def route_event(event,cfg):
    if event.get('post_type')!='message' or str(event.get('user_id'))==str(event.get('self_id')):
        return None
    kind=event.get('message_type')
    if kind not in ('group','private') or event.get('message_id') is None:
        return None
    sender=str(event.get('user_id',''))
    admin=sender in cfg.get('admins',[])
    target=event.get('group_id') if kind=='group' else event.get('user_id')
    if kind=='group' and str(target) not in cfg.get('allowed_groups',[]):
        if not (admin and cfg.get('admin_group_bypass',False)):return None
    if kind=='private' and not (admin or cfg.get('allow_private',False)):
        return None
    parts=event.get('message',[])
    if not isinstance(parts,list):return None
    text=''.join(str(p.get('data',{}).get('text','')) for p in parts if p.get('type')=='text').strip()
    mention=any(p.get('type')=='at' and str(p.get('data',{}).get('qq'))==str(event.get('self_id')) for p in parts)
    prefix=next((p for p in cfg.get('wake_prefix',[]) if p and text.startswith(p)),None)
    if kind=='group' and not mention and not prefix:return None
    if prefix:text=text[len(prefix):].strip()
    if not str(target).isdigit():return None
    session=cfg.get('legacy_platform','napcat')+(':'+ 'GroupMessage:' if kind=='group' else ':FriendMessage:')+str(target)
    return {'kind':kind,'target':int(target),'sender':sender,'session':session,'text':text,'image':any(p.get('type')=='image' for p in parts),'event_key':f"{event.get('self_id')}:{kind}:{target}:{event['message_id']}"}

class ActionRejected(Exception):pass

class OneBot:
    def __init__(self,url,token):
        self.url=url; self.token=token; self.ws=None; self.pending={}
    async def call(self,action,params,timeout=30):
        if self.ws is None:raise ConnectionError('onebot_offline')
        echo=uuid.uuid4().hex
        f=asyncio.get_running_loop().create_future(); self.pending[echo]=f
        try:
            await self.ws.send_json({'action':action,'params':params,'echo':echo})
            return await asyncio.wait_for(f,timeout)
        finally:self.pending.pop(echo,None)
    def receive(self,message):
        f=self.pending.get(message.get('echo'))
        if f is None or f.done():return
        if message.get('status')=='ok' and message.get('retcode',0)==0:f.set_result(message.get('data'))
        else:f.set_exception(ActionRejected('onebot_action_rejected'))
    def disconnected(self):
        self.ws=None
        for f in self.pending.values():
            if not f.done():f.set_exception(ConnectionError('onebot_disconnected_delivery_unknown'))
        self.pending.clear()
    async def run(self,queue):
        import aiohttp
        async with aiohttp.ClientSession() as session:
            while True:
                try:
                    async with session.ws_connect(self.url,headers={'Authorization':'Bearer '+self.token},heartbeat=30,max_msg_size=4*1024*1024) as ws:
                        self.ws=ws; logger.info('onebot_connected')
                        async for msg in ws:
                            if msg.type!=aiohttp.WSMsgType.TEXT:continue
                            data=json.loads(msg.data)
                            if 'echo' in data:self.receive(data)
                            elif data.get('post_type')=='message':
                                try:queue.put_nowait(data)
                                except asyncio.QueueFull:logger.warning('message_queue_full')
                except asyncio.CancelledError:raise
                except Exception as exc:logger.warning('onebot_connection_error %s',type(exc).__name__)
                finally:self.disconnected()
                await asyncio.sleep(3)

