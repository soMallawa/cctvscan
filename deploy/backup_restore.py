#!/usr/bin/env python3
"""Quiesced backups and fresh-project restores. No secret values enter argv."""
import argparse,hashlib,os,pathlib,shutil,subprocess,tarfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
COMMAND=['docker','compose','--env-file',os.environ.get('CCTVSCAN_COMPOSE_ENV_FILE','/dev/null'),'-f',str(ROOT/'compose.yaml')]
def run(args,**kw):return subprocess.run(COMMAND+args,cwd=ROOT,check=True,**kw)
def pg(command,**kw):return run(['exec','-T','db','sh','-ec','IFS= read -r PGPASSWORD < /run/secrets/db-password; export PGPASSWORD; exec '+command],**kw)
def verify(root):
 expected={'database.dump','jobs.tar.gz','secrets.tar.gz'};lines=(root/'SHA256SUMS').read_text().splitlines();seen=set()
 for line in lines:
  digest,name=line.split('  ',1)
  if name not in expected or name in seen:raise SystemExit('Invalid backup manifest')
  seen.add(name);h=hashlib.sha256()
  with (root/name).open('rb') as f:
   for b in iter(lambda:f.read(1048576),b''):h.update(b)
  if h.hexdigest()!=digest:raise SystemExit('Backup checksum mismatch')
 if seen!=expected:raise SystemExit('Incomplete backup')
 for archive in ['jobs.tar.gz','secrets.tar.gz']:
  with tarfile.open(root/archive,'r:gz') as t:
   for m in t.getmembers():
    p=pathlib.PurePosixPath(m.name)
    if p.is_absolute() or '..' in p.parts or not (m.isdir() or m.isfile()):raise SystemExit('Unsafe archive member')
    if archive=='jobs.tar.gz' and (not p.parts or p.parts[0]!='jobs'):raise SystemExit('Unexpected job archive root')
    if archive=='secrets.tar.gz' and str(p) not in {'.','key.bin','db-password','database-url'}:raise SystemExit('Unexpected secret archive member')
def backup(root):
 root.mkdir(mode=0o700,parents=True,exist_ok=False);os.chmod(root,0o700)
 try:
  run(['stop','worker','api'])
  with (root/'database.dump').open('wb') as f:pg('pg_dump -h /var/run/postgresql -U cctvscan -d cctvscan -Fc --no-owner --no-acl',stdout=f)
  with (root/'jobs.tar.gz').open('wb') as f:run(['run','--rm','--no-deps','-T','--entrypoint','tar','api','-C','/var/lib/cctvscan','-czf','-','jobs'],stdout=f)
  secrets=pathlib.Path(os.environ.get('CCTVSCAN_SECRETS_DIR',str(ROOT/'deploy/secrets')))
  with tarfile.open(root/'secrets.tar.gz','w:gz') as t:
   for name in ['key.bin','db-password','database-url']:t.add(secrets/name,arcname=name,recursive=False)
  with (root/'SHA256SUMS').open('w') as manifest:
   for name in ['database.dump','jobs.tar.gz','secrets.tar.gz']:
    h=hashlib.sha256()
    with (root/name).open('rb') as f:
     for b in iter(lambda:f.read(1048576),b''):h.update(b)
    manifest.write(h.hexdigest()+'  '+name+'\n')
  verify(root)
 finally:run(['start','api','worker'])
 print('Verified backup created; database, private job bytes and encryption key included.')
def restore(root):
 verify(root)
 if run(['ps','-aq'],capture_output=True,text=True).stdout.strip():raise SystemExit('Restore requires a fresh Compose project with no existing containers')
 secrets=pathlib.Path(os.environ.get('CCTVSCAN_SECRETS_DIR',str(ROOT/'deploy/secrets')))
 if secrets.exists() and any(secrets.iterdir()):raise SystemExit('Restore requires a new empty secret directory')
 secrets.mkdir(mode=0o700,parents=True,exist_ok=True);os.chmod(secrets,0o700)
 with tarfile.open(root/'secrets.tar.gz','r:gz') as t:t.extractall(secrets,filter='data')
 for f in secrets.iterdir():os.chmod(f,0o600)
 run(['up','-d','--wait','db'])
 with (root/'database.dump').open('rb') as f:pg('pg_restore -h /var/run/postgresql -U cctvscan -d cctvscan --no-owner --no-acl --exit-on-error',stdin=f)
 run(['run','--rm','--no-deps','-T','init'])
 with (root/'jobs.tar.gz').open('rb') as f:run(['run','--rm','--no-deps','-T','--entrypoint','tar','worker','-C','/var/lib/cctvscan','--no-same-owner','-xzf','-'],stdin=f)
 run(['up','-d','--wait','api','web'])
 print('Restored into fresh volumes. WORKER REMAINS STOPPED: inspect queued jobs and enabled schedules before explicitly starting it.')
if __name__=='__main__':
 os.umask(0o077);p=argparse.ArgumentParser();p.add_argument('action',choices=['backup','restore','verify']);p.add_argument('directory',type=pathlib.Path);a=p.parse_args();globals()[a.action](a.directory.resolve())
