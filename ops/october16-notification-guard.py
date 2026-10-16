"""Release-scoped notification fence. Never prints configuration or credentials."""
import json, os, pathlib, selectors, subprocess, sys, time
os.umask(0o077)
postgres, marker_arg = sys.argv[1:]
marker = pathlib.Path(marker_arg)
backup = pathlib.Path(marker_arg + '.config.json')
release = pathlib.Path(marker_arg + '.release')
command = ['docker','exec','-i',postgres,'sh','-c',
           'exec psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"']
key = 'account_ops_notifications_v1'
def query(sql):
    result = subprocess.run(command, input=sql, text=True, capture_output=True, timeout=10)
    if result.returncode: raise RuntimeError('notification database operation failed')
    return result.stdout.strip()
def receive(process):
    with selectors.DefaultSelector() as selector:
        selector.register(process.stdout, selectors.EVENT_READ)
        if not selector.select(8): raise RuntimeError('notification fence response timeout')
        return process.stdout.readline().strip()
holder = None
try:
    if backup.exists(): raise RuntimeError('refuse existing guard backup')
    holder = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                              stderr=subprocess.DEVNULL, text=True, bufsize=1)
    # Session lock serializes all native configuration APIs while allowing readers.
    holder.stdin.write("SELECT CASE WHEN pg_try_advisory_lock(hashtextextended('account_ops_notifications_v1',0)) THEN 'locked' ELSE 'busy' END;" + chr(10)); holder.stdin.flush()
    if receive(holder) != 'locked': raise RuntimeError('notification config busy')
    original = query("SELECT value FROM settings WHERE key='account_ops_notifications_v1';")
    if not original: raise RuntimeError('notification config missing')
    document = json.loads(original)
    if not isinstance(document, dict): raise RuntimeError('invalid notification config')
    pending = query("SELECT (SELECT count(*) FROM account_ops_alerts WHERE state IN ('pending','failed','sending'))+(SELECT count(*) FROM account_ops_threshold_events WHERE state IN ('pending','failed','sending'));")
    if pending != '0': raise RuntimeError('pending notifications must finish before pause')
    backup.write_text(original)
    # The lock owner performs this update. Never call SaveConfig, which suppresses work.
    holder.stdin.write("UPDATE settings SET value=jsonb_set(value::jsonb,'{enabled}','false')::text,updated_at=NOW() WHERE key='account_ops_notifications_v1'; SELECT 'paused';"+chr(10));holder.stdin.flush()
    if receive(holder) != 'paused': raise RuntimeError('pause not confirmed')
    # All replicas refresh in 15s; allow one 5s settings read and 3s queued write.
    deadline = time.monotonic()+25
    while time.monotonic()<deadline:
        if holder.poll() is not None: raise RuntimeError('config fence lost')
        time.sleep(.2)
    if query("SELECT (SELECT count(*) FROM account_ops_alerts WHERE state IN ('pending','failed','sending'))+(SELECT count(*) FROM account_ops_threshold_events WHERE state IN ('pending','failed','sending'));") != '0':
        raise RuntimeError('notification queue changed during pause')
    marker.write_text('ready')
    deadline=time.monotonic()+1800
    while not release.exists():
        if holder.poll() is not None or time.monotonic()>deadline: raise RuntimeError('notification fence lost or expired')
        time.sleep(.2)
    action=release.read_text().strip()
    if action=='restore':
        literal="'"+original.replace("'","''")+"'"
        holder.stdin.write("UPDATE settings SET value="+literal+",updated_at=NOW() WHERE key='account_ops_notifications_v1'; SELECT 'restored';"+chr(10));holder.stdin.flush()
        if receive(holder)!='restored': raise RuntimeError('restore not confirmed')
    elif action!='keep-paused': raise RuntimeError('invalid fence release action')
    holder.communicate(timeout=5)
    marker.write_text('released')
except Exception as exc:
    marker.write_text('failed')
    print(type(exc).__name__+': '+str(exc),file=sys.stderr)
    raise SystemExit(1)
finally:
    if holder is not None and holder.poll() is None:
        holder.kill();holder.communicate()
