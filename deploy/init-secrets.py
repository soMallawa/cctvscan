#!/usr/bin/env python3
"""Create a NEW deployment's secrets. Never replace an existing key."""
import os,pathlib,secrets,sys
root=pathlib.Path(sys.argv[1] if len(sys.argv)>1 else 'deploy/secrets')
root.mkdir(mode=0o700,parents=True,exist_ok=True)
if root.is_symlink() or any(root.iterdir()):raise SystemExit('Refusing nonempty or symlink secret directory')
if os.getuid()!=int(os.environ.get('CCTVSCAN_UID','1000')):raise SystemExit('Export CCTVSCAN_UID=$(id -u) and CCTVSCAN_GID=$(id -g) for this deployment before provisioning')
os.chmod(root,0o700);password=secrets.token_hex(32)
for name,data in {'key.bin':secrets.token_bytes(32),'db-password':(password+'\n').encode(),'database-url':('postgres://cctvscan:'+password+'@/cctvscan?host=/var/run/postgresql&sslmode=disable\n').encode()}.items():
 fd=os.open(root/name,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
 with os.fdopen(fd,'wb') as f:f.write(data)
print('Created owner-only secret files; values are not displayed. Back up the key securely.')
