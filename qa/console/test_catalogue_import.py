import gzip,json,pathlib,subprocess,tempfile,unittest

ROOT=pathlib.Path(__file__).resolve().parents[2]
class CatalogueImport(unittest.TestCase):
 def test_pinned_records_and_only_official_verified_profiles(self):
  with tempfile.TemporaryDirectory() as td:
   p=pathlib.Path(td);source=p/'source';(source/'data').mkdir(parents=True)
   records=[{'id':'fixture-a','brand':'Fixture','model':'A','type':'bullet','resolution':{'megapixels':2,'label':'1080p'},'sources':['https://manufacturer.example/a'],'features':['original detail']},{'id':'fixture-b','brand':'Fixture','model':'B','resolution':{'megapixels':4}}]
   (source/'data/cameras.json').write_text(json.dumps(records));(source/'LICENSE').write_text('CC0 1.0 Universal')
   (source/'data/rtsp-patterns.json').write_text(json.dumps({'_meta':{'license':'CC0-1.0'},'brands':[{'brand':'Fixture','brand_id':'fixture','status':'verified','default_port':554,'templates':[{'stream':'main','path':'/live','verified':True,'source':'https://manufacturer.example/manual'},{'stream':'guess','path':'/guess','verified':False}],'unverified_leads':[{'source':'https://www.ispyconnect.com/cameras','path':'/not-licensed'}]}]}))
   subprocess.run(['python3',str(ROOT/'tools/import_camera_catalogue.py'),'--source',str(source),'--output',str(p/'out'),'--revision','fixture-revision'],check=True,capture_output=True)
   index=json.loads(gzip.decompress((p/'out/index.json.gz').read_bytes()));self.assertEqual(index['manifest']['revision'],'fixture-revision');self.assertEqual(index['manifest']['models'],2);self.assertEqual(len(index['profiles']),1);self.assertEqual(index['profiles'][0]['path'],'/live')
   b=index['brands'][0];pack=(p/'out/records.pack').read_bytes();rows=[json.loads(x) for x in gzip.decompress(pack[b['offset']:b['offset']+b['length']]).splitlines()];self.assertEqual(rows,records)
   self.assertNotIn('ispyconnect',json.dumps(index));self.assertEqual(index['manifest']['source_hashes']['data/cameras.json'].__len__(),64)
if __name__=='__main__':unittest.main()
