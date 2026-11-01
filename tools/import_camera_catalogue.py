#!/usr/bin/env python3
"""Build a compact offline catalogue from an explicitly pinned CC0 upstream checkout.
No camera requests, third-party aggregation, package installs or runtime downloads.
"""
import argparse,gzip,hashlib,json,pathlib,re,shutil,tempfile

def records(path):
    decoder=json.JSONDecoder();buf='';started=False;ended=False
    with path.open(encoding='utf-8') as f:
        while True:
            chunk=f.read(131072);buf+=chunk
            while True:
                buf=buf.lstrip()
                if not started:
                    if not buf:break
                    if buf[0]!='[':raise ValueError('camera input must be an array')
                    buf=buf[1:];started=True;continue
                if buf.startswith(','):buf=buf[1:];continue
                if buf.startswith(']'):ended=True;buf=buf[1:];break
                if ended or not buf:break
                try:row,n=decoder.raw_decode(buf)
                except json.JSONDecodeError:
                    if not chunk:raise
                    break
                yield row;buf=buf[n:]
            if ended:
                if buf.strip() or f.read().strip():raise ValueError('trailing input')
                return
            if not chunk:raise ValueError('incomplete camera array')

def key(name):return re.sub('[^a-z0-9]+','-',name.lower()).strip('-')
def digest(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda:f.read(131072),b''):h.update(block)
    return h.hexdigest()

def build(source,output,revision):
    if 'CC0' not in (source/'LICENSE').read_text():raise ValueError('expected CC0 data licence')
    output.mkdir(parents=True,exist_ok=True);models=[];brands={};seen=set()
    with tempfile.TemporaryDirectory(dir=output.parent) as td:
        tmp=pathlib.Path(td)
        for row in records(source/'data/cameras.json'):
            rid=row.get('id');brand=row.get('brand');model=row.get('model')
            if not all(isinstance(x,str) and x for x in (rid,brand,model)) or rid in seen:raise ValueError('missing or duplicate camera identity')
            seen.add(rid);bk=key(brand)
            if not bk:raise ValueError('unsupported empty brand key')
            if bk in brands and brands[bk]['name']!=brand:raise ValueError('brand key collision')
            b=brands.setdefault(bk,{'key':bk,'name':brand,'count':0});b['count']+=1
            with (tmp/(bk+'.jsonl')).open('ab') as f:f.write(json.dumps(row,ensure_ascii=False,separators=(',',':')).encode()+b'\n')
            res=row.get('resolution') or {}
            models.append({'id':rid,'brand':brand,'brand_key':bk,'model':model,'type':row.get('type',''),'megapixels':res.get('megapixels'),'resolution':res.get('label',''),'protocols':row.get('protocols') or []})
        with (output/'records.pack').open('wb') as pack:
            for bk,b in sorted(brands.items()):
                start=pack.tell()
                with gzip.GzipFile(fileobj=pack,mode='wb',filename='',mtime=0) as gz,(tmp/(bk+'.jsonl')).open('rb') as inp:shutil.copyfileobj(inp,gz,131072)
                b.update(offset=start,length=pack.tell()-start)
    patterns=json.loads((source/'data/rtsp-patterns.json').read_text());profiles=[]
    if patterns.get('_meta',{}).get('license')!='CC0-1.0':raise ValueError('RTSP reference must explicitly be CC0')
    for b in patterns['brands']:
        if b.get('status')!='verified':continue
        bk=key(b['brand']);port=b.get('default_port') or 554
        for i,t in enumerate(b.get('templates',[])):
            if t.get('verified') is not True or not t.get('source') or not isinstance(t.get('path'),str) or not t['path'].startswith('/'):continue
            # Unverified leads are deliberately never consumed. Provenance remains intact.
            profiles.append({'id':b['brand_id']+'-'+str(i),'brand':b['brand'],'brand_key':bk,'label':t.get('stream') or 'stream','scheme':'rtsp','port':port,'path':t['path'],'codec':t.get('codec',''),'status':'documented','source':t['source'],'verified_on':b.get('verified_on',''),'notes':b.get('notes','')})
    for b in brands.values():b['profile_count']=sum(p['brand_key']==b['key'] for p in profiles)
    files=['data/cameras.json','data/rtsp-patterns.json','LICENSE']
    manifest={'source':'https://github.com/ch-bas/cctv-camera-database','revision':revision,'license':'CC0-1.0','models':len(models),'brands':len(brands),'profiles':len(profiles),'source_hashes':{p:digest(source/p) for p in files},'pack_sha256':digest(output/'records.pack'),'format_version':1}
    index={'manifest':manifest,'models':sorted(models,key=lambda m:(m['brand'].lower(),m['model'].lower(),m['id'])),'brands':[b for _,b in sorted(brands.items())],'profiles':profiles}
    raw=json.dumps(index,ensure_ascii=False,separators=(',',':')).encode();(output/'index.json.gz').write_bytes(gzip.compress(raw,mtime=0));shutil.copyfile(source/'LICENSE',output/'LICENSE.upstream');(output/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n');print(json.dumps(manifest))
if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--source',required=True,type=pathlib.Path);p.add_argument('--output',required=True,type=pathlib.Path);p.add_argument('--revision',required=True);a=p.parse_args();build(a.source,a.output,a.revision)
