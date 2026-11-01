"""Static operational contract; runtime acceptance is separate."""
import json, pathlib, subprocess, unittest
ROOT=pathlib.Path(__file__).resolve().parents[2]
class Packaging(unittest.TestCase):
 def test_compose_is_private_and_bounded(self):
  p=subprocess.run(['docker','compose','--env-file','/dev/null','-f',str(ROOT/'compose.yaml'),'config','--format','json'],capture_output=True,text=True)
  self.assertEqual(p.returncode,0,p.stderr)
  c=json.loads(p.stdout);services=c['services']
  for name in ('api','worker','web'):
   s=services[name];self.assertTrue(s['read_only']);self.assertIn('ALL',s['cap_drop']);self.assertIn('no-new-privileges:true',s['security_opt']);self.assertGreater(int(s['mem_limit']),0);self.assertGreater(s['pids_limit'],0);
   for v in s.get('volumes',[]):
    if v['type']=='bind':self.assertTrue(v['target'].startswith('/run/secrets/'));self.assertTrue(v['read_only'])
  for s in services.values():
   for mount in s.get('tmpfs',[]):
    self.assertTrue(mount.startswith('/'),f'Invalid tmpfs mount {mount!r}; quote comma-separated options in YAML')
   for port in s.get('ports',[]):self.assertEqual(port['host_ip'],'127.0.0.1')
  self.assertFalse(services['db'].get('ports'));self.assertEqual(services['web']['network_mode'],'service:api')
 def test_nginx_temp_paths_support_read_only_root(self):
  for name in ('web/nginx.conf','deploy/nginx.conf'):
   config=(ROOT/name).read_text()
   for kind in ('client_body','proxy','fastcgi','uwsgi','scgi'):
    self.assertIn(f'{kind}_temp_path /tmp/',config,name)
  self.assertEqual((ROOT/'web/nginx.conf').read_bytes(),(ROOT/'deploy/nginx.conf').read_bytes())
 def test_operating_artifacts_exist(self):
  for p in ('Dockerfile','web/Dockerfile','deploy/nginx.conf','deploy/init-secrets.py','deploy/backup.sh','deploy/restore.sh','docs/console-operations.md'):
   self.assertTrue((ROOT/p).is_file(),p)
if __name__=='__main__':unittest.main()
