#!/usr/bin/env python3
"""Switch only the Narra web app; preserve the running queue worker and cleanup process."""
import copy,fcntl,importlib.util,json,os,re,subprocess,time
from pathlib import Path
ROOT=Path('/opt/sub2api-test-station')
PROJECT='sub2api-test-station-narra'
def run(args,data=None):
 p=subprocess.run(args,input=data,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 if p.returncode:raise RuntimeError('command failed: '+args[0])
 return p.stdout.decode()
def write(p,data):
 p=Path(p);tmp=p.with_suffix(p.suffix+'.tmp');tmp.write_text(data);tmp.chmod(0o600);os.replace(tmp,p)
def inspect(container):return json.loads(run(['docker','inspect',container]))[0]
def app_config(config,old_service,image):
 config=copy.deepcopy(config);name='narra-image-blue' if old_service=='narra-image-green' else 'narra-image-green'
 app=copy.deepcopy(config['services'][old_service]);app['image']=image;app.pop('depends_on',None);app.pop('container_name',None)
 app.setdefault('environment',{}).setdefault('WORKSTATION_MODEL_FAMILIES','')
 config['services'][name]=app
 return config,name

def main():
 import sys
 commit,tree,image=sys.argv[1:]
 if os.geteuid()!=0 or not re.fullmatch('[a-f0-9]{40}',commit) or not re.fullmatch('[a-f0-9]{40}',tree):raise RuntimeError('invalid release source')
 lock=open(ROOT/'.api-release.lock','a');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
 narra=ROOT/'narra';state=json.loads((narra/'release-state.json').read_text());old_service=state.get('active_app_service','narra-image')
 old_id=run(['docker','ps','-q','--filter','label=com.docker.compose.project='+PROJECT,'--filter','label=com.docker.compose.service='+old_service]).strip()
 if not old_id or '\n' in old_id:raise RuntimeError('expected one active app')
 path=Path(inspect(old_id)['Config']['Labels']['com.docker.compose.project.config_files'])
 if not str(path).startswith(str(narra)+'/'):raise RuntimeError('unexpected app configuration path')
 config=json.loads(run(['docker','compose','-p',PROJECT,'--env-file',str(narra/'.env'),'-f',str(path),'config','--format','json']))
 config,name=app_config(config,old_service,image)
 release=narra/'releases'/commit;release.mkdir(mode=0o700,parents=True)
 write(release/'compose.json',json.dumps(config,indent=2)+'\n');write(release/'previous-state.json',json.dumps(state,indent=2)+'\n')
 compose=['docker','compose','-p',PROJECT,'--env-file',str(narra/'.env'),'-f',str(release/'compose.json')]
 start=time.monotonic();run(compose+['up','-d','--no-deps',name]);new_id=run(compose+['ps','-q',name]).strip()
 deadline=time.monotonic()+120
 while inspect(new_id)['State'].get('Health',{}).get('Status')!='healthy':
  if time.monotonic()>deadline:raise RuntimeError('app readiness timed out')
  time.sleep(1)
 run(['docker','exec',new_id,'node','-e',"fetch('http://127.0.0.1:3000/image-workstation/api/readyz').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"])
 caddy=run(['docker','ps','-q','--filter','label=com.docker.compose.project=sub2api-test-station','--filter','label=com.docker.compose.service=test-station-caddy']).strip()
 caddy_path=Path(next(m['Source']for m in inspect(caddy)['Mounts']if m['Destination']=='/etc/caddy/Caddyfile'))
 before=caddy_path.read_text();after,n=re.subn(r'(?<![\w-])'+re.escape(old_service)+r':3000(?![\w-])',name+':3000',before)
 if n!=1:raise RuntimeError('expected exactly one app route')
 write(release/'previous-Caddyfile',before)
 def reload(value):
  run(['docker','exec','-i',caddy,'caddy','validate','--config','-','--adapter','caddyfile'],value.encode())
  run(['docker','exec','-i',caddy,'caddy','reload','--config','-','--adapter','caddyfile'],value.encode())
 try:
  reload(after)
  import urllib.request
  with urllib.request.urlopen('http://127.0.0.1/image-workstation/api/readyz',timeout=15)as response:
   if json.load(response).get('status')!='ready':raise RuntimeError('public readiness failed')
 except Exception:
  reload(before);raise
 write(caddy_path,after)
 state.update(source_commit=commit,source_tree=tree,image_tag=image,active_app_service=name,active_app_container=new_id,result='succeeded',previous_app_service=old_service,previous_release_dir=str(path.parent),release_dir=str(release),app_image_id=inspect(new_id)['Image'],app_ready_seconds=round(time.monotonic()-start,2))
 state.pop('image_manifest_digest',None);state.pop('source_archive_sha256',None)
 state['narra-image_image_id']=state['app_image_id']
 write(narra/'release-state.json',json.dumps(state,indent=2)+'\n')
 site=json.loads((ROOT/'release-state.json').read_text());site['narra']=state;write(ROOT/'release-state.json',json.dumps(site,indent=2)+'\n')
 print('narra_ui promoted source='+commit+' service='+name,flush=True)
 deadline=time.monotonic()+300
 while time.monotonic()<deadline:
  rows=run(['docker','exec',old_id,'cat','/proc/net/tcp','/proc/net/tcp6'])
  count=sum(1 for line in rows.splitlines()if len(line.split())>=4 and line.split()[1].endswith(':0BB8')and line.split()[3]=='01')
  if not count:break
  time.sleep(2)
 run(['docker','stop','--time','10',old_id])
 print('narra_ui completed worker_unchanged=true cleanup_unchanged=true',flush=True)
if __name__=='__main__':
 try:main()
 except Exception as e:print('narra_ui failed '+str(e),flush=True);raise SystemExit(1)
