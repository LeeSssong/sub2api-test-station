import subprocess,pathlib,tempfile,time,json
cmd=['docker','exec','-i','sub2api-oct16-guard-test','psql','-X','-qAt','-U','postgres','-v','ON_ERROR_STOP=1']
def q(sql):return subprocess.run(cmd,input=sql,text=True,capture_output=True,check=True).stdout.strip()
q('''DROP TABLE IF EXISTS settings,account_ops_alerts,account_ops_threshold_events; CREATE TABLE settings(key text PRIMARY KEY,value text,updated_at timestamptz); INSERT INTO settings VALUES('account_ops_notifications_v1','{"enabled":true,"recipient":"fixture@example.invalid"}',NOW()); CREATE TABLE account_ops_alerts(state text); CREATE TABLE account_ops_threshold_events(state text);''')
d=pathlib.Path(tempfile.mkdtemp(prefix='oct16-guard-'));marker=d/'guard'
p=subprocess.Popen(['python3','ops/october16-notification-guard.py','sub2api-oct16-guard-test',str(marker)],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
try:
 deadline=time.monotonic()+35
 while not marker.exists():
  if p.poll() is not None:raise RuntimeError(p.communicate()[1])
  if time.monotonic()>deadline:raise RuntimeError('guard readiness timeout')
  time.sleep(.2)
 assert marker.read_text()=='ready'
 assert q("SELECT value::jsonb->>'enabled' FROM settings;")=='false'
 assert q("SELECT pg_try_advisory_lock(hashtextextended('account_ops_notifications_v1',0));")=='f'
 pathlib.Path(str(marker)+'.release').write_text('restore')
 out,err=p.communicate(timeout=8);assert p.returncode==0,err
 assert q("SELECT value::jsonb->>'enabled' FROM settings;")=='true'
 assert q("SELECT pg_try_advisory_lock(hashtextextended('account_ops_notifications_v1',0));")=='t'
 print('PASS pause, native config fence, exact restore, lock release')
finally:
 if p.poll() is None:p.kill();p.communicate()
