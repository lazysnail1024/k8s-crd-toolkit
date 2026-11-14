#!/usr/bin/env python3
"""Compile fixed real consumers with the exact candidate vendor bytes, in isolation."""
import argparse,hashlib,json,os,pathlib,shutil,subprocess,sys,tempfile,time,tarfile
import os
CI_IDENTITY = {k: os.getenv(k, 'local') for k in ('GITHUB_SHA', 'GITHUB_RUN_ID', 'GITHUB_RUN_ATTEMPT')}
ROOT=pathlib.Path(__file__).resolve().parents[1];sys.path.insert(0,str(ROOT/'tools'))
from snapshot import create
from source_inventory import inventory, digest as inventory_digest
MODULE='github.com/lazysnail1024/k8s-crd-toolkit'
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def tree(root):return {p.relative_to(root).as_posix():sha(p) for p in root.rglob('*') if p.is_file() and '.git' not in p.parts}
def main():
 p=argparse.ArgumentParser();p.add_argument('--output',type=pathlib.Path,default=ROOT/'artifacts/downstream');p.add_argument('--control',choices=['old-vendor','wrong-sha','missing-file','missing-consumer']);p.add_argument('--checkout-cache',type=pathlib.Path,help='optional directory containing fixed local clones; only read by git clone');a=p.parse_args();out=a.output.resolve();out.mkdir(parents=True,exist_ok=False)
 pins=json.loads((ROOT/'tests/downstream-pins.json').read_text());tuple_={'toolkit':subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),**{k:v['commit'] for k,v in pins['consumers'].items()}};snapshot=create(out/'snapshot');reports=[]
 if a.control=='wrong-sha':tuple_['toolkit']='0'*40
 env=os.environ.copy();env.update(GOWORK='off',CGO_ENABLED='1',GOTOOLCHAIN='local',GOFLAGS='-mod=vendor')
 report=dict(executionIdentity=CI_IDENTITY,schemaVersion=1,buildRole='downstream-candidate',tuple=tuple_,snapshot=snapshot,runID=os.getenv('GITHUB_RUN_ID','local'),attempt=os.getenv('GITHUB_RUN_ATTEMPT','1'),consumers=reports,passed=False)
 def save(): (out/'report.json').write_text(json.dumps(report,indent=2))
 try:
  if tuple_['toolkit']!=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip():raise ValueError('wrong toolkit SHA')
  for file,digest in snapshot['files'].items():
   committed=subprocess.check_output(['git','show',tuple_['toolkit']+':'+file],cwd=ROOT)
   if hashlib.sha256(committed).hexdigest()!=digest:raise ValueError('candidate snapshot differs from toolkit SHA: '+file)
  for name,pin in pins['consumers'].items():
   if a.control=='missing-consumer' and name=='zed-argo-workflows':continue
   dest=out/name;dest.mkdir();row=dict(consumer=name,tuple=tuple_,steps=[],passed=False);reports.append(row);save()
   def run(cmd,cwd,timeout=300):
    start=time.monotonic()
    try:r=subprocess.run(cmd,cwd=cwd,env=env,text=True,capture_output=True,timeout=timeout)
    except (subprocess.TimeoutExpired,OSError) as error:
     row['steps'].append(dict(command=cmd,stdout=str(getattr(error,'stdout','')),stderr=str(getattr(error,'stderr',''))+repr(error),exitCode=124 if isinstance(error,subprocess.TimeoutExpired) else 127,seconds=time.monotonic()-start));save();raise
    row['steps'].append(dict(command=cmd,stdout=r.stdout,stderr=r.stderr,exitCode=r.returncode,seconds=time.monotonic()-start));save()
    if r.returncode:raise RuntimeError(name+': command failed '+repr(cmd))
    return r.stdout
   checkout=dest/'checkout';origin=str(a.checkout_cache/name) if a.checkout_cache else pin['repository']
   run(['git','clone','--no-hardlinks','--no-checkout',origin,str(checkout)],ROOT);run(['git','checkout','--detach',pin['commit']],checkout)
   before=inventory(checkout);commit_object=run(['git','cat-file','commit',pin['commit']],checkout);vendor=checkout/'vendor'/MODULE
   if a.control!='old-vendor':shutil.rmtree(vendor);vendor.mkdir(parents=True)
   # Preserve the original consumer module/version declaration; the derived
   # vendor tree is the declared candidate dependency and never a release.
   for name_,digest in ([] if a.control=='old-vendor' else snapshot['files'].items()):
    target=vendor/name_;target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(ROOT/name_,target)
   if a.control=='missing-file':(vendor/'source/snapshot.go').unlink()
   actual=tree(vendor)
   if actual!=snapshot['files']:raise ValueError('candidate vendor inventory mismatch')
   after=inventory(checkout);changed=sorted(k for k in set(before)|set(after) if before.get(k)!=after.get(k))
   if not changed or any(not k.startswith('vendor/'+MODULE+'/') for k in changed):raise ValueError('unexplained derived source changes')
   patch={k:[before.get(k),after.get(k)] for k in changed}
   row.update(injection='replace-vendor-runtime',allowedPaths=['vendor/'+MODULE+'/'],originalCommitObject=commit_object,originalInventory=before,derivedInventory=after,sourcePatch=patch,patchDigest=inventory_digest(patch),effectiveSourceTree=inventory_digest(after),vendorDigest=snapshot['sourceSHA256'])
   with tarfile.open(dest/'source.tar.gz','w:gz') as archive:
    for file in sorted(after):archive.add(checkout/file,arcname=file,recursive=False)
   # Inspect actual package compiler inputs, not merely go.mod's old version.
   listing=run(['go','list','-deps','-json','./cmd/'+pin['command']],checkout);decoder=json.JSONDecoder();packages=[]
   while listing.strip():
    package,n=decoder.raw_decode(listing.lstrip());packages.append(package);listing=listing.lstrip()[n:]
   deps=[x for x in packages if x['ImportPath'].startswith(MODULE+'/')]
   if not deps:raise ValueError('candidate not in actual dependency graph')
   for package in deps:
    directory=pathlib.Path(package['Dir']).resolve()
    if not directory.is_relative_to(vendor.resolve()):raise ValueError('compiler resolved old toolkit')
    for file in package.get('GoFiles',[])+package.get('CgoFiles',[])+package.get('CFiles',[])+package.get('HFiles',[]):
     path=directory/file;relative=path.relative_to(vendor).as_posix()
     if snapshot['files'].get(relative)!=sha(path):raise ValueError('compiler input does not match candidate')
   row['compilerDependencies']=deps;binary=dest/pin['command'];run(['go','build','-trimpath','-buildvcs=false','-o',str(binary),'./cmd/'+pin['command']],checkout);row['binaryPath']=str(binary);row['binarySHA256']=sha(binary);row['buildMetadata']=run(['go','version','-m',str(binary)],checkout)
   row['suiteReports']=[]
   for n,suite in enumerate(pin['suites']):
    evidence=dest/('suite-'+str(n));run([sys.executable,suite,str(binary),str(evidence)],checkout,600)
    files=list(evidence.rglob('report.json'))+list(evidence.rglob('results.json'))
    if not files:raise ValueError('consumer did not write a fresh report')
    for f in files:
     value=json.loads(f.read_text())
     if isinstance(value,list):
      if not value or any(v.get('pass') is not True for v in value):raise ValueError('consumer cases failed')
     elif value.get('passed') is not True:raise ValueError('consumer report failed')
    evidence_files=files+([evidence/'manifest.json'] if (evidence/'manifest.json').exists() else [])
    row['suiteReports'].append(dict(suite=suite,files={str(f.relative_to(out)):sha(f) for f in evidence_files},evidence={f.name:{'rawJSON':f.read_text(),'sha256':sha(f)} for f in evidence_files}))
   if tree(vendor)!=snapshot['files']:raise ValueError('candidate changed during consumption')
   row['passed']=True;save()
  report['passed']=len(reports)==2 and {r['consumer'] for r in reports}==set(pins['consumers']) and all(r['passed'] and r['tuple']==tuple_ for r in reports)
 except Exception as e:report['error']=repr(e)
 finally:save()
 print(json.dumps({'passed':report['passed'],'error':report.get('error'),'report':str(out/'report.json')}));return not report['passed']
if __name__=='__main__':sys.exit(main())
