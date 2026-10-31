import contextlib,http.server,json,os,pathlib,socketserver,subprocess,tempfile,threading
ROOT=pathlib.Path(__file__).resolve().parent;BIN=pathlib.Path(os.environ.get('CCTVSCAN_TEST_BIN',str(ROOT.parent/'bin'/'cctvscan'))).resolve()
class Fixture(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  self.server.hits+=1;body=b'<html>Public brochure</html>';self.send_response(200);self.send_header('Content-Type','text/html');self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body)
 def log_message(self,*a):pass
@contextlib.contextmanager
def service():
 s=http.server.ThreadingHTTPServer(('127.0.0.1',0),Fixture);s.hits=0;thread=threading.Thread(target=s.serve_forever,daemon=True);thread.start()
 try:yield s
 finally:s.shutdown();s.server_close();thread.join()
base=os.environ.copy();base.pop('CCTVSCAN_HTTP_PROXY',None)
def scan(port,env,folder):
 p=subprocess.run([str(BIN),'--unprivileged','--ports',str(port),'--rate','20','--retry','0','--wait','0','--timeout','20s','--host-timeout','5s','--http-rps','0','--output',str(folder),'127.0.0.1'],env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=30)
 files=list(folder.glob('scan-*.json'));assert len(files)==1,(p.returncode,p.stdout,files)
 return p,json.loads(files[0].read_text())
receipts=[]
with service() as origin:
 with tempfile.TemporaryDirectory(prefix='cctv-local-') as d:
  p,r=scan(origin.server_port,base,pathlib.Path(d));assert p.returncode==0,(p.returncode,p.stdout,r)
  assert r['State']=='complete' and len(r['Results'])==1 and origin.hits==1,(p.stdout,r,origin.hits)
  assert not r['Results'][0]['CameraDetection']['IsCamera'] and not r['Results'][0]['StreamResults'];assert r['Results'][0]['CredentialState']=='not_tested'
  receipts.append({'case':'Real CLI / Naabu CONNECT / exact-pair verification / neutral HTTP / JSON','exit':p.returncode,'http_requests':origin.hits,'state':r['State']})
 before=origin.hits
 with service() as proxy:
  proxy_port=proxy.server_port;env=base.copy();env['CCTVSCAN_HTTP_PROXY']=f'http://fixture:synthetic-secret-token@127.0.0.1:{proxy_port}'
  with tempfile.TemporaryDirectory(prefix='cctv-proxy-') as d:
   p,r=scan(origin.server_port,env,pathlib.Path(d));assert p.returncode==0,(p.returncode,p.stdout,r);assert origin.hits==before and proxy.hits==1,(origin.hits,before,proxy.hits);assert 'synthetic-secret-token' not in p.stdout+json.dumps(r)
   receipts.append({'case':'Real CLI / proxied HTTP / raw CONNECT stays separate','exit':p.returncode,'direct_http_requests':origin.hits-before,'proxy_requests':proxy.hits})
 # The private proxy has now been shut down. Explicit policy must fail closed.
 with tempfile.TemporaryDirectory(prefix='cctv-failure-') as d:
  p,r=scan(origin.server_port,env,pathlib.Path(d));assert p.returncode!=0 and r['State']=='inconclusive',(p.stdout,r);assert origin.hits==before;assert 'synthetic-secret-token' not in p.stdout+json.dumps(r)
  receipts.append({'case':'Real CLI / dead proxy / no direct fallback / durable inconclusive result','exit':p.returncode,'direct_http_requests':origin.hits-before,'state':r['State']})
with tempfile.TemporaryDirectory(prefix='cctv-fake-') as d:
 d=pathlib.Path(d);stub=d/'masscan';argv_file=d/'argv.json';stub.write_text('#!/usr/bin/python3\nimport json,os,sys\nopen(os.environ["FIXTURE_ARGV"],"w").write(json.dumps(sys.argv[1:]))\n');stub.chmod(0o700)
 env=base.copy();env['PATH']=str(d)+os.pathsep+env['PATH'];env['FIXTURE_ARGV']=str(argv_file)
 p=subprocess.run([str(BIN),'--ports','0-65535','--rate','4','--wait','7','--retry','2','--output',str(d/'output'),'198.51.100.10'],env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=10)
 args=json.loads(argv_file.read_text());assert p.returncode==0,(p.stdout,args)
 for flag,value in [('-p','0-65535'),('--rate','4'),('--wait','7'),('--retries','2')]:assert args[args.index(flag)+1]==value,args
 receipts.append({'case':'Fake Masscan only / explicit full ports and wait/retry/rate','exit':p.returncode,'argv':args})
(ROOT/'integration-receipts.json').write_text(json.dumps(receipts,indent=2)+'\n');print(json.dumps(receipts,indent=2))
