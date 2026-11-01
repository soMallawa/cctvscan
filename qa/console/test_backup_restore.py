"""Offline regression for fresh restore ordering; live proof is installed_recovery.py."""
import hashlib,importlib.util,io,os,pathlib,tarfile,tempfile,unittest
from unittest.mock import patch
ROOT=pathlib.Path(__file__).resolve().parents[2]
spec=importlib.util.spec_from_file_location('console_backup',ROOT/'deploy/backup_restore.py')
assert spec is not None and spec.loader is not None
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
class RestoreOrdering(unittest.TestCase):
 def test_initializes_fresh_volume_before_unprivileged_tar_without_starting_worker(self):
  with tempfile.TemporaryDirectory() as tmp:
   root=pathlib.Path(tmp)/'backup';root.mkdir();secrets=pathlib.Path(tmp)/'new-secrets'
   (root/'database.dump').write_bytes(b'owned synthetic dump fixture')
   for name,members in [('jobs.tar.gz',{'jobs/fixture.txt':b'owned synthetic evidence'}),('secrets.tar.gz',{'key.bin':b'x'*32,'db-password':b'qa-only','database-url':b'qa-only'})]:
    with tarfile.open(root/name,'w:gz') as archive:
     for path,content in members.items():
      member=tarfile.TarInfo(path);member.size=len(content);member.mode=0o600;archive.addfile(member,io.BytesIO(content))
   (root/'SHA256SUMS').write_text(''.join(hashlib.sha256((root/name).read_bytes()).hexdigest()+'  '+name+'\n' for name in ['database.dump','jobs.tar.gz','secrets.tar.gz']))
   calls=[]
   def run(args,**kwargs):
    calls.append(args)
    return type('Completed',(),{'stdout':''})()
   with patch.dict(os.environ,{'CCTVSCAN_SECRETS_DIR':str(secrets)}),patch.object(module,'run',side_effect=run):module.restore(root)
   init=calls.index(['run','--rm','--no-deps','-T','init']);unpack=next(i for i,args in enumerate(calls) if args[:1]==['run'] and 'tar' in args)
   self.assertLess(init,unpack);self.assertEqual(calls[-1],['up','-d','--wait','api','web'])
   self.assertEqual((secrets/'key.bin').read_bytes(),b'x'*32)
if __name__=='__main__':unittest.main()
