#!/usr/bin/env python3
"""One-shot CPAMP -> CPAMC static switch; run from a verified main bundle."""
import fcntl, hashlib, json, os, pathlib, shutil, subprocess, sys, time, urllib.request
R=pathlib.Path('/opt/cpa-test-station'); B=pathlib.Path(sys.argv[1]); commit,tree=sys.argv[2:4]
locks=[]
for p in ['/opt/sub2api-test-station/.api-release.lock',str(R/'.keeper-release.lock'),str(R/'.ticket-release.lock')]:
 f=open(p,'a');fcntl.flock(f,fcntl.LOCK_EX|fcntl.LOCK_NB);locks.append(f)
def run(*a): return subprocess.check_output(a,text=True).strip()
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def atomic(src,dst):
 tmp=dst.with_name(dst.name+'.cpamc-new');shutil.copyfile(src,tmp);os.chmod(tmp,0o644);os.replace(tmp,dst)
def get(path,key=None,base='http://127.0.0.1:8317'):
 q=urllib.request.Request(base+path,headers={'Authorization':'Bearer '+key} if key else {})
 with urllib.request.urlopen(q,timeout=25) as f:return f.status,f.read()
start=time.monotonic(); source=json.loads((B/'cpamc/source.json').read_text());assert sha(B/'cpamc/management.html')==source['sha256']
assert 'cpa-gateway:8317' in (B/'Caddyfile').read_text() and 'cpa-manager:18317' not in (B/'Caddyfile').read_text()
key=(R/'secrets/cpa-management-key').read_text().strip()
for p in ['/v8/management/config','/v8/management/credentials','/v8/management/plugins']:assert get(p,key)[0]==200
before=run('docker','inspect','cpa-test-station-cpa-gateway-1','--format','{{.State.StartedAt}}')
backup=R/'backups'/time.strftime('cpamc-%Y%m%dT%H%M%SZ',time.gmtime());backup.mkdir(mode=0o700)
files={'Caddyfile':R/'edge/Caddyfile','management.html':R/'edge/management.html','static-management.html':R/'cliproxyapi/static/management.html','compose.yaml':R/'compose.yaml'}
for n,p in files.items():shutil.copy2(p,backup/n)
record={'commit':commit,'tree':tree,'source':source,'backup':str(backup),'database_backup':False}
try:
 atomic(B/'Caddyfile',R/'edge/Caddyfile')
 run('docker','exec','cpa-test-station-edge-cpa-edge-1','caddy','validate','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')
 atomic(B/'cpamc/management.html',R/'edge/management.html');atomic(B/'cpamc/management.html',R/'cliproxyapi/static/management.html')
 run('docker','exec','cpa-test-station-edge-cpa-edge-1','caddy','reload','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')
 for base in ['https://cpa-test.xingqiaolab.top','http://127.0.0.1:8317']:
  assert hashlib.sha256(get('/management.html',base=base)[1]).hexdigest()==source['sha256']
  for p in ['/v8/management/config','/v8/management/credentials','/v8/management/plugins']:assert get(p,key,base)[0]==200
 assert get('/health',base='http://127.0.0.1')[0]==200
 record['switch_seconds']=round(time.monotonic()-start,2)
except Exception:
 for n,p in files.items():atomic(backup/n,p)
 run('docker','exec','cpa-test-station-edge-cpa-edge-1','caddy','reload','--config','/etc/caddy/Caddyfile','--adapter','caddyfile')
 record['result']='rolled-back';(backup/'release.json').write_text(json.dumps(record,indent=2));raise
# Gracefully stop only the obsolete manager, allowing up to 300 seconds.
run('docker','stop','--time','300','cpa-test-station-cpa-manager-1')
run('docker','rm','cpa-test-station-cpa-manager-1')
run('docker','volume','rm','cpa-test-station-manager-data')
atomic(B/'legacy-entry.Caddyfile',R/'edge/legacy-entry.Caddyfile');atomic(B/'compose.yaml',R/'compose.yaml')
run('docker','compose','-f',str(R/'compose.yaml'),'up','-d','--no-deps','cpa-legacy-entry')
assert before==run('docker','inspect','cpa-test-station-cpa-gateway-1','--format','{{.State.StartedAt}}')
deadline=time.monotonic()+30
while True:
 try:
  assert get('/management.html',base='http://127.0.0.1:18317')[0]==200
  break
 except (OSError, AssertionError):
  if time.monotonic()>=deadline:raise
  time.sleep(0.25)
record.update(result='success',total_seconds=round(time.monotonic()-start,2),gateway_not_restarted=True,deleted_volume='cpa-test-station-manager-data',rollback='Restore saved Caddyfile/pages and old compose; CPAMP would start with an empty database. Historical data deliberately deleted.')
(backup/'release.json').write_text(json.dumps(record,indent=2)+'\n');print(json.dumps(record))
