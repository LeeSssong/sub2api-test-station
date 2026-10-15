"""One-time server-local import. Never print configuration or conversation contents."""
import argparse
import json
import os
import secrets
import shutil
import sqlite3
from pathlib import Path
from .state import State

def migrate(source,dest):
 source=Path(source);dest=Path(dest)
 if (dest/'config.json').exists():raise RuntimeError('destination_already_configured')
 dest.mkdir(parents=True,exist_ok=True);os.chmod(dest,0o700)
 data=dest/'data';data.mkdir(exist_ok=True)
 cfg=json.loads((source/'data/cmd_config.json').read_text())
 runner=cfg['agent_runner']['config']; selection=runner['model']
 ids=[selection['provider_id']]+selection.get('fallback_provider_ids',[])
 providers=[]
 for ident in ids:
  p=next(p for p in cfg['provider'] if p['id']==ident and p.get('enable'))
  s=next(s for s in cfg['provider_sources'] if s['id']==p['provider_source_id'] and s.get('enable'))
  keys=s['key'];key=next((k for k in keys if k),None) if isinstance(keys,list) else keys
  if not key:raise ValueError('missing_model_key')
  providers.append({'base':s['api_base'],'key':key,'model':p['model'],'timeout':s.get('timeout',30)})
 with sqlite3.connect('file:'+str(source/'data/data_v4.db')+'?mode=ro',uri=True) as db:
  persona=db.execute('SELECT system_prompt FROM personas WHERE persona_id=?',(runner['persona']['persona_id'],)).fetchone()
  if not persona:raise ValueError('missing_persona')
  platform=next(p for p in cfg['platform'] if p.get('enable') and p['type']=='aiocqhttp')['id']
  state=State(data/'state.sqlite3')
  rows=db.execute('SELECT user_id,content FROM conversations WHERE platform_id=?',(platform,)).fetchall()
  # Prefer the selected conversation if multiple records exist for a session.
  prefs={r[0]:json.loads(r[1]) for r in db.execute("SELECT scope_id,value FROM preferences WHERE scope='umo' AND key='sel_conv_id'")}
  for session,history in rows:
   selected=prefs.get(session,{})
   if isinstance(selected,dict):selected=selected.get('val')
   if selected:
    r=db.execute('SELECT content FROM conversations WHERE conversation_id=? AND platform_id=? AND user_id=?',(selected,platform,session)).fetchone()
    if r:history=r[0]
   messages=json.loads(history or '[]')
   messages=[m for m in messages if m.get('role') in ('user','assistant') and isinstance(m.get('content'),str) and m['content']]
   state.save_history(session,messages)
 settings=cfg['platform_settings']
 if not settings.get('enable_id_white_list'):raise ValueError('explicit_whitelist_required')
 result={'onebot_url':'ws://127.0.0.1:16301/','onebot_token':secrets.token_urlsafe(32),'legacy_platform':platform,'allowed_groups':[str(x) for x in settings['id_whitelist']],'admins':[str(x) for x in cfg['admins_id']],'admin_group_bypass':settings.get('wl_ignore_admin_on_group',False),'allow_private':False,'wake_prefix':cfg['wake_prefix'],'providers':providers,'persona':persona[0],'knowledge':'\n\n'.join(p.read_text() for p in sorted((source/'kb').glob('*.md')))}
 # Retain the existing outbox keys and every terminal/uncertain delivery record.
 if (source/'data/xingqiao-reports').exists():
  shutil.copytree(source/'data/xingqiao-reports',data/'reports',ignore=shutil.ignore_patterns('*.sqlite3','*.sqlite3-wal','*.sqlite3-shm'))
  old=source/'data/xingqiao-reports/outbox.sqlite3'
  if old.exists():
   with sqlite3.connect('file:'+str(old)+'?mode=ro',uri=True) as src,sqlite3.connect(data/'reports/outbox.sqlite3') as dst:src.backup(dst)
 (dest/'config.json').write_text(json.dumps(result,ensure_ascii=False,indent=2));os.chmod(dest/'config.json',0o600)
 for candidate in [source/'data/xingqiao_login.env',source/'secrets.env',source/'.env']:
  if candidate.exists():
   # Merge with first occurrence winning, matching the prior lookup order.
   existing={}
   target=dest/'secrets.env'
   if target.exists():existing=dict(line.split('=',1) for line in target.read_text().splitlines() if '=' in line and not line.startswith('#'))
   for line in candidate.read_text().splitlines():
    if '=' in line and not line.startswith('#'):
     k,v=line.split('=',1)
     if k.strip().startswith('XINGQIAO_'):existing.setdefault(k.strip(),v)
   target.write_text('\n'.join(k+'='+v for k,v in existing.items())+'\n');os.chmod(target,0o600)
 for p in data.rglob('*'):
  if p.is_file():os.chmod(p,0o600)
 print('Imported configuration, persona, knowledge, conversation and report state; no secrets displayed.')

if __name__=='__main__':
 parser=argparse.ArgumentParser();parser.add_argument('--source',required=True);parser.add_argument('--dest',required=True);a=parser.parse_args();migrate(a.source,a.dest)

