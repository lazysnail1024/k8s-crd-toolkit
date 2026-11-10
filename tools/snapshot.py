#!/usr/bin/env python3
"""Create an immutable local Go module proxy from this checkout; never publish."""
import argparse,hashlib,io,json,pathlib,zipfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
MODULE='github.com/lazysnail1024/k8s-crd-toolkit'
def create(destination):
 destination=pathlib.Path(destination).resolve();destination.mkdir(parents=True,exist_ok=True)
 paths=[ROOT/x for x in ('go.mod','go.sum','LICENSE','CONTRACT.md')]
 for name in ('source','edit','yamlview','index','workspace'):paths.extend(p for p in (ROOT/name).rglob('*') if p.is_file() and not p.name.endswith('_test.go'))
 contents={}
 for p in sorted(paths,key=lambda p:p.relative_to(ROOT).as_posix()):
  if p.is_symlink() or not p.is_file():raise ValueError('missing/linked runtime input: '+str(p))
  contents[p.relative_to(ROOT).as_posix()]=p.read_bytes()
 hashes={n:hashlib.sha256(b).hexdigest() for n,b in contents.items()};digest=hashlib.sha256(json.dumps(hashes,sort_keys=True,separators=(',',':')).encode()).hexdigest();version='v0.0.0-local.g'+digest[:16]
 data=io.BytesIO()
 with zipfile.ZipFile(data,'w',compression=zipfile.ZIP_STORED) as z:
  for n,b in contents.items():
   i=zipfile.ZipInfo(MODULE+'@'+version+'/'+n,date_time=(1980,1,1,0,0,0));i.compress_type=zipfile.ZIP_STORED;i.create_system=3;i.external_attr=0o100644<<16;z.writestr(i,b)
 base=destination/MODULE/'@v';base.mkdir(parents=True,exist_ok=True)
 for suffix,b in {'.zip':data.getvalue(),'.mod':contents['go.mod'],'.info':(json.dumps({'Version':version})+'\n').encode()}.items():
  p=base/(version+suffix)
  if p.exists() and p.read_bytes()!=b:raise ValueError('immutable version content mismatch: '+str(p))
  p.write_bytes(b)
 (base/'list').write_text('\n'.join(sorted(p.stem for p in base.glob('*.info')))+'\n')
 m=dict(module=MODULE,version=version,origin='local-content-snapshot',published=False,sourceSHA256=digest,contractSHA256=hashes['CONTRACT.md'],zipSHA256=hashlib.sha256(data.getvalue()).hexdigest(),files=hashes)
 (destination/'manifest.json').write_text(json.dumps(m,indent=2)+'\n');return m
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--output',type=pathlib.Path,default=ROOT/'artifacts/module-proxy');args=p.parse_args();print(json.dumps(create(args.output)))
