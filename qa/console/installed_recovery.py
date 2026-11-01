#!/usr/bin/env python3
"""Installed-image recovery proof. Explicit disposable project only; owned fixtures.
Run with the same local Compose environment as the acceptance stack. No real fleet.
"""
import hashlib,json,os,pathlib,subprocess,time,urllib.request,uuid
ROOT=pathlib.Path(__file__).resolve().parents[2]
ORIGIN=os.environ.get('CCTVSCAN_PUBLIC_ORIGIN','http://127.0.0.1:14910')
PROJECT=os.environ.get('COMPOSE_PROJECT_NAME','')
if not PROJECT.startswith('cctvscan-acceptance'):raise SystemExit('Requires an explicitly disposable cctvscan-acceptance* project')
TARGET=os.environ['CCTVSCAN_E2E_TARGET']
import ipaddress
if not ipaddress.ip_address(TARGET).is_private:raise SystemExit('Owned private fixture address required')
REPORT=pathlib.Path(os.environ['CCTVSCAN_QA_REPORT_DIR']);REPORT.mkdir(parents=True,exist_ok=True)
def api(path,body=None,origin=ORIGIN,method=None):
 data=None if body is None else json.dumps(body).encode()
 req=urllib.request.Request(origin+'/api'+path,data=data,method=method,headers={'Origin':origin,'Content-Type':'application/json'})
 with urllib.request.urlopen(req,timeout=15) as r:
  raw=r.read();return json.loads(raw) if 'application/json' in r.headers.get('Content-Type','') else raw

def compose(*args,env=None):return subprocess.run(['docker','compose','--env-file','/dev/null','-f',str(ROOT/'compose.yaml'),*args],cwd=ROOT,env=env,check=True)
def wait_job(id):
 deadline=time.monotonic()+90
 while time.monotonic()<deadline:
  j=api('/jobs/'+id)
  if j['state'] in ('completed','inconclusive','failed','canceled','timed_out','orphaned'):return j
  time.sleep(.3)
 raise AssertionError('fixture job did not terminate')

def snapshot(id,origin=ORIGIN):
 j=api('/jobs/'+id,origin=origin);results=api('/jobs/'+id+'/results',origin=origin);history=api('/jobs/'+id+'/history?limit=200',origin=origin);art=api('/jobs/'+id+'/artifacts',origin=origin)
 images={a['id']:hashlib.sha256(api('/artifacts/'+a['id'],origin=origin)).hexdigest() for a in art['items']}
 return {'job':j,'results':results,'history':history,'artifacts':art,'images':images}

# A real scanner, synthetic JPEG fixture and explicit capture opt-in.
c=api('/bootstrap')['defaults'];c.update(targets=[TARGET],port_mode='custom',ports='18082',scan_mode='connect',max_targets=1,retry=1,wait=1,threads=1,host_workers=1,timeout_seconds=45,host_timeout_seconds=20,http_rps=0,capture=True,check_credentials=False,onvif=False)
j=api('/jobs',{'name':'QA synthetic JPEG recovery fixture','config':c,'priority':3,'idempotency_key':str(uuid.uuid4())})
j=wait_job(j['id']);assert j['state']=='completed',j['state'];assert j['result_count']==1,j
before=snapshot(j['id']);assert before['images'],'Real scanner did not save the synthetic image'
# Encrypted configuration must survive the same restore; no secret read-back.
proxy=api('/proxies',{'name':'QA encrypted recovery fixture','url':'http://qa-only:qa-restore-secret@127.0.0.1:18081'})
assert 'qa-restore-secret' not in json.dumps(proxy)
(REPORT/'recovery-before.json').write_text(json.dumps(before,indent=2))
# Full service recreation, same named volumes; app/web share a namespace.
compose('stop','worker','web','api','db')
compose('up','-d','--no-build','--force-recreate','--wait')
after=snapshot(j['id']);assert after==before,'Historical evidence changed after recreation'
# SSE replay through installed nginx, not a direct API-only substitute.
first=min(int(e['seq']) for e in before['history']['items'])
request=urllib.request.Request(ORIGIN+'/api/events',headers={'Last-Event-ID':str(first),'Accept':'text/event-stream'})
with urllib.request.urlopen(request,timeout=10) as response:
 ids=[]
 for _ in range(100):
  line=response.readline().decode().strip()
  if line.startswith('id:'):ids.append(int(line[3:]));break
 assert ids and ids[0]>first,'SSE replay did not advance beyond supplied cursor'
backup=REPORT/('private-backup-'+uuid.uuid4().hex[:8])
subprocess.run(['python3',str(ROOT/'deploy/backup_restore.py'),'backup',str(backup)],cwd=ROOT,check=True)
subprocess.run(['python3',str(ROOT/'deploy/backup_restore.py'),'verify',str(backup)],cwd=ROOT,check=True)
# Stop original runtime temporarily to reserve headroom; volumes remain intact.
compose('stop','worker','web','api','db')
restored=dict(os.environ,COMPOSE_PROJECT_NAME='cctvscan-acceptance-restored-'+uuid.uuid4().hex[:8],CCTVSCAN_PORT='14911',CCTVSCAN_PUBLIC_ORIGIN='http://127.0.0.1:14911',CCTVSCAN_SECRETS_DIR=str(REPORT/('restored-secrets-'+uuid.uuid4().hex[:8])))
try:
 subprocess.run(['python3',str(ROOT/'deploy/backup_restore.py'),'restore',str(backup)],cwd=ROOT,env=restored,check=True)
 restored_state=snapshot(j['id'],restored['CCTVSCAN_PUBLIC_ORIGIN']);assert restored_state==before,'Restored database/artifact content mismatch'
 assert api('/proxies/'+proxy['id'],origin=restored['CCTVSCAN_PUBLIC_ORIGIN'])==proxy
 running=subprocess.check_output(['docker','compose','--env-file','/dev/null','-f',str(ROOT/'compose.yaml'),'ps','--services','--status','running'],env=restored,text=True)
 assert 'worker' not in running.splitlines(),'Restored worker must stay stopped'
 receipt={'status':'PASS','job_id':j['id'],'synthetic_image_count':len(before['images']),'recreation_preserved_exact_state':True,'sse_replay':True,'backup_verified':True,'restore_preserved_exact_state':True,'restored_worker_stopped':True,'proxy_metadata_preserved':True}
 (REPORT/'installed-recovery.json').write_text(json.dumps(receipt,indent=2));print(json.dumps(receipt))
finally:
 compose('stop',env=restored)
 compose('up','-d','--no-build','--wait')
