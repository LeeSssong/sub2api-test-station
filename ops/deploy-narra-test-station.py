#!/usr/bin/env python3
"""Explicitly authorized test-station maintenance, with native migrations and restore rehearsal.
Uses the existing API release controller; does not touch production or unrelated services.
"""
import argparse, fcntl, hashlib, importlib.util, json, os, subprocess, time
from pathlib import Path

ROOT=Path('/opt/sub2api-test-station')
PROJECT='sub2api-test-station'

def run(args, data=None):
    p=subprocess.run(args,input=data,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
    if p.returncode: raise RuntimeError('command failed: '+args[0]+' exit '+str(p.returncode))
    return p.stdout

def load_base(path):
    spec=importlib.util.spec_from_file_location('test_release',path)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module);return module

def db_command(pg, query, database=None):
    command=['docker','exec','-i',pg,'sh','-c','exec psql -X -v ON_ERROR_STOP=1 -At -U "$POSTGRES_USER" -d '+('"$POSTGRES_DB"' if database is None else database)]
    return run(command,query.encode()).decode()

def dump(pg,path):
    with path.open('wb') as out:
        p=subprocess.run(['docker','exec',pg,'sh','-c','exec pg_dump -Fc -U "$POSTGRES_USER" "$POSTGRES_DB"'],stdout=out,stderr=subprocess.PIPE)
    if p.returncode:raise RuntimeError('database backup failed')
    path.chmod(0o600)

def restore(pg,path,database):
    with path.open('rb') as data:
        p=subprocess.run(['docker','exec','-i',pg,'sh','-c','exec pg_restore --exit-on-error --no-owner -U "$POSTGRES_USER" -d '+database],stdin=data,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
    if p.returncode:
        error_path=path.parent/'restore-error.log';error_path.write_bytes(p.stderr);error_path.chmod(0o600)
        raise RuntimeError('database restore failed; protected diagnostics saved')

def native_migrate(image,env_path):
    run(['docker','run','--rm','--network',PROJECT+'-network','--env-file',str(env_path),'-v',PROJECT+'-app-data:/app/data',image,'/app/sub2api','--migrate-only'])

def main():
    parser=argparse.ArgumentParser();parser.add_argument('bundle',type=Path);args=parser.parse_args()
    if os.geteuid()!=0:raise RuntimeError('root required')
    lock=open(ROOT/'.api-release.lock','a');fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
    bundle=args.bundle.resolve()
    if not str(bundle).startswith('/var/tmp/sub2api-test-station-api.'):raise RuntimeError('unsafe bundle')
    manifest=json.loads((bundle/'manifest.json').read_text())
    base=load_base(bundle/'deploy.py');previous=json.loads((ROOT/'release-state.json').read_text())
    if previous['source_commit']!=manifest['previous_commit']:raise RuntimeError('active release changed')
    binary=bundle/'sub2api'
    if hashlib.sha256(binary.read_bytes()).hexdigest()!=manifest['binary_sha256']:raise RuntimeError('binary checksum mismatch')
    old_api=previous['active_api_container'];old_worker=base.active_worker()
    pg=run(['docker','ps','-q','--filter','label=com.docker.compose.project='+PROJECT,'--filter','label=com.docker.compose.service=test-station-postgres']).decode().strip()
    if not pg or '\n' in pg:raise RuntimeError('database identity mismatch')
    image=PROJECT+'-runtime:'+manifest['source_commit']
    if run(['docker','image','inspect','--format','{{.Id}}',previous['image_tag']]).decode().strip()!=manifest['base_image_id']:raise RuntimeError('base image mismatch')
    (bundle/'Dockerfile').write_text('FROM '+previous['image_tag']+'\nCOPY --chown=1000:1000 --chmod=0555 sub2api /app/sub2api\n')
    run(['docker','build','--pull=false','-t',image,str(bundle)])
    backup=ROOT/'backups'/('narra-'+manifest['source_commit'][:12]);backup.mkdir(mode=0o700,exist_ok=False)
    api_env=base.inspect(old_api)['Config']['Env']
    base.private_write(backup/'migration.env','\n'.join(api_env)+'\n')
    rehearsal='narra_rehearsal_'+manifest['source_commit'][:12]
    base.private_write(backup/'rehearsal.env','\n'.join(e for e in api_env if not e.startswith('DATABASE_DBNAME='))+'\nDATABASE_DBNAME='+rehearsal+'\n')
    started=time.monotonic();print('test_station maintenance=preparation restore_rehearsal=started',flush=True)
    dump(pg,backup/'preflight.dump')
    db_command(pg,'CREATE DATABASE '+rehearsal+';')
    try:
        restore(pg,backup/'preflight.dump',rehearsal)
        native_migrate(image,backup/'rehearsal.env')
        rows=db_command(pg,"SELECT filename || '|' || checksum FROM schema_migrations ORDER BY filename;",rehearsal)
        base.verify_migration_checksums(manifest['migration_checksums'],rows)
    finally:db_command(pg,'DROP DATABASE '+rehearsal+' WITH (FORCE);')
    print('test_station restore_rehearsal=passed seconds='+str(round(time.monotonic()-started,2)),flush=True)
    downtime=time.monotonic();migrated=False;promoted=False
    try:
        print('test_station maintenance=stopping_business',flush=True)
        run(['docker','stop','--time','300',old_api]);base.stop_worker(old_worker)
        dump(pg,backup/'stopped.dump')
        digest=hashlib.sha256((backup/'stopped.dump').read_bytes()).hexdigest()
        base.private_write(backup/'SHA256SUMS',digest+'  stopped.dump\n')
        migrated=True;native_migrate(image,backup/'migration.env')
        base.verify_migration_checksums(manifest['migration_checksums'],db_command(pg,"SELECT filename || '|' || checksum FROM schema_migrations ORDER BY filename;"))
        old_active=base.active_worker;old_stop=base.stop_worker;old_change=base.change_upstream
        def active():
            try:return old_active()
            except ValueError:return old_worker
        def stop(container,timeout=300):
            if base.inspect(container)['State']['Status']=='exited':return
            return old_stop(container,timeout)
        def change(config,previous_service,candidate):
            new=old_change(config,previous_service,candidate)
            if '/image-workstation/*' not in new:
                # Caddy orders path-specific proxies ahead of the catch-all.
                first=new.find('{'); new=new[:first+1]+'\n  reverse_proxy /image-workstation/* narra-image:3000'+new[first+1:]
            return new
        base.active_worker=active;base.stop_worker=stop;base.change_upstream=change
        base.deploy(bundle);promoted=True
        release=ROOT/'releases'/manifest['source_commit']
        meta=json.loads((release/'deployment.json').read_text());meta['maintenance']={'backup_dir':str(backup),'restore_rehearsal':True,'downtime_seconds':round(time.monotonic()-downtime,2)};meta['result']='succeeded'
        base.save(release/'deployment.json',meta)
        print('test_station maintenance=succeeded downtime_seconds='+str(meta['maintenance']['downtime_seconds']),flush=True)
    except Exception:
        release=ROOT/'releases'/manifest['source_commit'];new_worker_started=False
        if (release/'deployment.json').exists():new_worker_started=json.loads((release/'deployment.json').read_text()).get('worker_update_started',False)
        if migrated and not new_worker_started:
            run(['docker','stop','--time','300',old_api])
            dbname=db_command(pg,'SELECT current_database();').strip()
            db_command(pg,"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid();")
            run(['docker','exec',pg,'sh','-c','exec dropdb -U "$POSTGRES_USER" "$POSTGRES_DB"'])
            run(['docker','exec',pg,'sh','-c','exec createdb -U "$POSTGRES_USER" "$POSTGRES_DB"'])
            restore(pg,backup/'stopped.dump',dbname)
        run(['docker','start',old_api,old_worker]);raise
if __name__=='__main__':
    try:main()
    except Exception as error:print('test_station maintenance=failed reason='+str(error),flush=True);raise SystemExit(1)
