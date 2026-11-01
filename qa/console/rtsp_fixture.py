"""Disposable loopback RTSP/H.264 camera fixture. No external targets."""
import base64,json,pathlib,socketserver,struct,subprocess,tempfile,time
class Camera(socketserver.StreamRequestHandler):
 def handle(self):
  try:
   while True:
    line=self.rfile.readline(4096).decode().strip()
    if not line:return
    parts=line.split(' ')
    if len(parts)!=3:return
    method,uri,_=parts;headers={}
    while True:
     h=self.rfile.readline(4096).decode().strip()
     if not h:break
     k,v=h.split(':',1);headers[k.lower()]=v.strip()
    response={'CSeq':headers.get('cseq','1'),'Server':'Owned Camera Fixture'};body=b'';status='200 OK'
    if method=='DESCRIBE':
     expected='Basic '+base64.b64encode(b'fixture-user:fixture-pass').decode()
     if headers.get('authorization')!=expected:status='401 Unauthorized';response['WWW-Authenticate']='Basic realm="owned-fixture"'
     else:body=self.server.sdp;response['Content-Type']='application/sdp';response['Content-Base']=self.server.base+'/'
    elif method=='OPTIONS':response['Public']='OPTIONS, DESCRIBE, SETUP, PLAY, TEARDOWN'
    elif method=='SETUP':response['Session']='ownedfixture';response['Transport']='RTP/AVP/TCP;unicast;interleaved=0-1'
    elif method=='PLAY':response['Session']='ownedfixture';response['Range']='npt=0.000-'
    elif method=='TEARDOWN':return
    response['Content-Length']=str(len(body));wire='RTSP/1.0 '+status+'\r\n'+''.join(k+': '+v+'\r\n' for k,v in response.items())+'\r\n';self.wfile.write(wire.encode()+body);self.wfile.flush()
    if method=='PLAY':
     seq=1;stamp=0
     for frame in range(100):
      for nal in self.server.nals:
       kind=nal[0]&31;marker=0x80 if kind in (1,5) else 0
       payload=struct.pack('!BBHII',0x80,96|marker,seq&65535,stamp,0x12345678)+nal;seq+=1
       self.wfile.write(b'$\x00'+struct.pack('!H',len(payload))+payload)
      self.wfile.flush();stamp+=18000;time.sleep(.2)
     return
  except (BrokenPipeError,ConnectionResetError,UnicodeDecodeError,ValueError,OSError):return
class Server(socketserver.ThreadingTCPServer):allow_reuse_address=True;daemon_threads=True
with tempfile.TemporaryDirectory() as td:
 p=pathlib.Path(td)/'fixture.h264'
 subprocess.run(['ffmpeg','-nostdin','-hide_banner','-loglevel','error','-f','lavfi','-i','color=red:size=64x48:rate=5','-frames:v','1','-c:v','libx264','-threads','1','-preset','ultrafast','-tune','zerolatency','-f','h264',str(p)],check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
 data=p.read_bytes().replace(b'\x00\x00\x00\x01',b'\x00\x00\x01');nals=[n for n in data.split(b'\x00\x00\x01') if n]
 sps=next(n for n in nals if n[0]&31==7);pps=next(n for n in nals if n[0]&31==8)
 with Server(('127.0.0.1',0),Camera) as server:
  server.nals=nals;host,port=server.server_address;server.base=f'rtsp://{host}:{port}/live'
  server.sdp=('v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=Owned Fixture\r\nt=0 0\r\na=control:*\r\nm=video 0 RTP/AVP 96\r\na=rtpmap:96 H264/90000\r\na=fmtp:96 packetization-mode=1;sprop-parameter-sets='+base64.b64encode(sps).decode()+','+base64.b64encode(pps).decode()+'\r\na=control:trackID=0\r\n').encode()
  print(json.dumps({'host':host,'port':port}),flush=True);server.serve_forever()
