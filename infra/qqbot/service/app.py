import asyncio
import base64
import json
import logging
import os
from pathlib import Path
from .business import Business, FRIENDLY_ERROR, IMAGE_GUIDE, redact_secrets, strip_markdown, is_tech_error
from .model import Model
from .protocol import OneBot, route_event, ActionRejected
from .state import State
from infra.qqbot.plugins.astrbot_plugin_xingqiao_ops.performance import is_performance_query, load_snapshot, render_card, read_cached_card, write_cached_card, next_refresh_delay, REFRESH_INTERVAL
from infra.qqbot.plugins.astrbot_plugin_xingqiao_ops.pelican_delivery import ReportDelivery,load_report_config,DefiniteSendFailure,content_digest,report_is_usable
from infra.qqbot.plugins.astrbot_plugin_xingqiao_ops.pelican_commands import parse_report_query
from infra.qqbot.plugins.astrbot_plugin_xingqiao_ops.pelican_report_source import load_detection_report_snapshot
from infra.qqbot.plugins.astrbot_plugin_xingqiao_ops.pelican_client import ReadOnlyReportClient
from .business import _load_admin_api_key,_payload_data
from datetime import datetime, timezone

logger=logging.getLogger(__name__)

class App:
 def __init__(self,cfg,root):
  self.cfg=cfg;self.root=Path(root);self.root.mkdir(parents=True,exist_ok=True)
  self.state=State(self.root/'state.sqlite3');self.business=Business()
  self.bot=OneBot(cfg['onebot_url'],cfg['onebot_token'])
  self.model=Model(cfg,self.state,self.invoke)
  self.queue=asyncio.Queue(maxsize=100)
  self.reports=self.root/'reports';self.report_config=self.reports/'config.json'
  self.performance=self.root/'performance.png'
  self.report_service=None;self.current_report_config=None
  self.rate={}
 async def invoke(self,name,args):
  method=getattr(self.business,name)
  return await asyncio.to_thread(lambda:asyncio.run(method(None,**args)))
 async def send(self,kind,target,segments):
  action='send_group_msg' if kind=='group' else 'send_private_msg'
  return await self.bot.call(action,{('group_id' if kind=='group' else 'user_id'):target,'message':segments})
 async def text(self,event,text):
  if is_tech_error(text):text=FRIENDLY_ERROR
  text=redact_secrets(strip_markdown(text))
  # One bounded reply avoids partial multi-send retries.
  return await self.send(event['kind'],event['target'],[{'type':'text','data':{'text':text[:6000]}}])
 @staticmethod
 def image(image):return {'type':'image','data':{'file':'base64://'+base64.b64encode(image).decode()}}
 async def snapshot(self,group):
  cfg=load_report_config(self.report_config)
  client=ReadOnlyReportClient(_load_admin_api_key())
  return await asyncio.to_thread(load_detection_report_snapshot,client,group,max_age_seconds=cfg.stats_max_age_seconds,artwork_max_age_seconds=cfg.artwork_max_age_seconds)
 async def render(self,snapshot):
  from infra.qqbot.plugins.astrbot_plugin_xingqiao_ops.pelican_renderer import render_report_png
  return await render_report_png(snapshot,timeout_seconds=45,executable_path=os.environ.get('XINGQIAO_REPORTS_CHROMIUM') or None)
 async def report_send(self,session,image):
  cfg=load_report_config(self.report_config)
  if not cfg.enabled or cfg.dry_run or cfg!=self.current_report_config or session not in [t.session for t in cfg.targets]:raise DefiniteSendFailure('report_disabled')
  group=session.rsplit(':',1)[-1]
  if group not in self.cfg['allowed_groups']:raise DefiniteSendFailure('group_not_allowed')
  if self.bot.ws is None:raise DefiniteSendFailure('offline_before_send')
  try:r=await self.send('group',int(group),[self.image(image)])
  except ActionRejected as exc:raise DefiniteSendFailure('platform_rejected') from exc
  # Timeouts/disconnects remain uncertain; never automatically resend.
  return 'onebot:'+str((r or {}).get('message_id','accepted'))
 async def manual_report(self,event,requested):
  cfg=load_report_config(self.report_config)
  target=next((t for t in cfg.targets if t.session==event['session']),None)
  if cfg.dry_run or target is None:return
  groups=[requested] if requested else list(target.group_ids[:cfg.max_per_target])
  if any(g not in target.group_ids for g in groups):return
  snapshots=[];images=[]
  for group in groups:
   snap=await self.snapshot(group)
   if snap.get('group_id')!=group or not report_is_usable(snap):raise ValueError('stale_report')
   snapshots.append(snap);images.append(self.image(await self.render(snap)))
  for group,snap in zip(groups,snapshots):
   current=await self.snapshot(group)
   if content_digest(current)!=content_digest(snap) or not report_is_usable(current):raise ValueError('report_changed')
  if load_report_config(self.report_config)!=cfg:return
  await self.send('group',event['target'],images)
 async def handle(self,raw):
  event=route_event(raw,self.cfg)
  if event is None:return
  if not self.state.claim(event['event_key']):return
  import time
  times=self.rate.setdefault(event['session'],[])
  times[:]=[t for t in times if t>time.monotonic()-60]
  if len(times)>=30:return
  times.append(time.monotonic())
  text=event['text']
  if text in ('reset','重置会话','清空会话'):
   if event['sender'] not in self.cfg['admins']:return
   self.state.save_history(event['session'],[]);await self.text(event,'会话已重置。');return
  if text in ('help','帮助'):
   await self.text(event,'@小星 可以咨询星桥使用问题、查询分组、发送 request id 查报错，或查询性能卡片、检测报告。');return
  report=parse_report_query(text)
  if report is not None and event['kind']=='group':
   await self.manual_report(event,report);return
  if is_performance_query(text):
   try:image=await asyncio.to_thread(read_cached_card,self.performance)
   except Exception:await self.text(event,'性能卡片正在更新，稍后再试一下呀。');return
   await self.send(event['kind'],event['target'],[self.image(image)]);return
  if event['image']:await self.text(event,IMAGE_GUIDE);return
  if not text:await self.text(event,'我在呀，把问题发给我吧。');return
  try:answer=await self.model.answer(event['session'],redact_secrets(text))
  except Exception as exc:
   logger.warning('model_failed %s',type(exc).__name__);answer=FRIENDLY_ERROR
  await self.text(event,answer)
 async def worker(self):
  while True:
   raw=await self.queue.get()
   try:await self.handle(raw)
   except Exception as exc:logger.warning('message_processing_failed %s',type(exc).__name__)
   finally:self.queue.task_done()
 async def report_loop(self):
  while True:
   try:
    if self.report_config.exists():
     cfg=load_report_config(self.report_config)
     if cfg!=self.current_report_config:
      self.current_report_config=cfg;self.report_service=ReportDelivery(cfg,self.reports,self.snapshot,self.render,self.report_send)
     if self.bot.ws is not None:await self.report_service.tick()
   except Exception as exc:logger.warning('report_tick_failed %s',type(exc).__name__)
   await asyncio.sleep(30)
 async def performance_loop(self):
  while True:
   try:
    def refresh():
     import time
     end=datetime.fromtimestamp(time.time()//REFRESH_INTERVAL*REFRESH_INTERVAL,timezone.utc)
     snap=load_snapshot(lambda p:_payload_data(self.business._auth_get(p)),end=end)
     write_cached_card(self.performance,render_card(snap),snap['end'].timestamp())
    await asyncio.to_thread(refresh)
   except Exception as exc:logger.warning('performance_refresh_failed %s',type(exc).__name__)
   await asyncio.sleep(next_refresh_delay())
 async def run(self):
  await asyncio.gather(self.bot.run(self.queue),self.worker(),self.report_loop(),self.performance_loop())

def main():
 logging.basicConfig(level=logging.INFO,format='%(asctime)s %(levelname)s %(message)s')
 cfg=json.loads(Path(os.environ.get('XINGQIAO_BOT_CONFIG','/opt/qqbot-native/config.json')).read_text())
 asyncio.run(App(cfg,os.environ.get('XINGQIAO_BOT_DATA','/opt/qqbot-native/data')).run())

if __name__=='__main__':main()

