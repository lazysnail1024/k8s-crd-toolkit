"""Canonical tracked-source identities; Git tree commitments bind pinned originals."""
import hashlib,json,os,pathlib,subprocess
PREFIX='vendor/github.com/lazysnail1024/k8s-crd-toolkit/'
def object_id(kind,data):return hashlib.sha1(kind.encode()+b' '+str(len(data)).encode()+b'\0'+data).hexdigest()
def digest(value):return hashlib.sha256(json.dumps(value,sort_keys=True,separators=(',',':')).encode()).hexdigest()
def inventory(root):
 result={}
 for p in root.rglob('*'):
  if '.git' in p.relative_to(root).parts or (p.is_dir() and not p.is_symlink()):continue
  if p.is_symlink():data=os.readlink(p).encode();mode='120000'
  else:data=p.read_bytes();mode='100755' if p.stat().st_mode&0o111 else '100644'
  result[p.relative_to(root).as_posix()]={'mode':mode,'object':object_id('blob',data)}
 return result
def tree_id(entries):
 tree={}
 for path,value in entries.items():
  parts=path.split('/')
  if not parts or any(x in ('','..','.') for x in parts):raise ValueError('invalid inventory path')
  node=tree
  for part in parts[:-1]:node=node.setdefault(part,{})
  node[parts[-1]]=(value['mode'],value['object'])
 def build(node):
  rows=[]
  for name,value in node.items():
   mode,oid=('40000',build(value)) if isinstance(value,dict) else value
   if mode not in ('40000','100644','100755','120000'):raise ValueError('invalid source file mode')
   rows.append((name+('/' if mode=='40000' else ''),mode.encode()+b' '+name.encode()+b'\0'+bytes.fromhex(oid)))
  return object_id('tree',b''.join(data for _,data in sorted(rows)))
 return build(tree)
def verify(row,pin,root,files):
 original=row['originalInventory'];derived=row['derivedInventory'];commit=row['originalCommitObject'].encode()
 if object_id('commit',commit)!=pin or commit.splitlines()[0]!=('tree '+tree_id(original)).encode():raise ValueError('original inventory differs from pinned commit')
 expected={k:v for k,v in original.items() if not k.startswith(PREFIX)}
 expected.update({PREFIX+k:{'mode':'100644','object':object_id('blob',(root/k).read_bytes())} for k in files})
 if derived!=expected:raise ValueError('derived inventory is not exact candidate injection')
 patch={k:[original.get(k),derived.get(k)] for k in sorted(set(original)|set(derived)) if original.get(k)!=derived.get(k)}
 if row.get('effectiveSourceTree')!=digest(derived):raise ValueError('derived source tree digest mismatch')
 if row.get('patchDigest')!=digest(patch):raise ValueError('actual candidate patch digest mismatch')
 if row.get('sourcePatch')!=patch:raise ValueError('actual candidate patch mismatch')
