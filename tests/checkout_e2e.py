#!/usr/bin/env python3
"""Verify real Git checkouts preserve module bytes and remain consumable."""
import argparse,hashlib,json,os,pathlib,shutil,subprocess,sys,tempfile,time,traceback,zipfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--output',type=pathlib.Path,default=ROOT/'artifacts/checkout');p.add_argument('--without-attributes',action='store_true');args=p.parse_args()
out=args.output.resolve();out.mkdir(parents=True,exist_ok=True);run_id=str(time.time_ns())
report={'replay':sys.argv,'environment':{k:os.environ.get(k) for k in ('GO','GOROOT','GOMODCACHE','GOCACHE','CGO_ENABLED','GOWORK','CC')},'steps':[],'cases':[],'assertions':{},'passed':False}
def save():
 text=json.dumps(report,indent=2)+'\n';(out/'report.json').write_text(text);(out/('report-'+run_id+'.json')).write_text(text)
def run(argv,cwd):
 r=subprocess.run(argv,cwd=cwd,capture_output=True,timeout=300)
 step={'argv':argv,'cwd':str(cwd),'stderr':r.stderr.decode(errors='replace'),'exitCode':r.returncode}
 if argv[:2]==['git','show']:
  digest=hashlib.sha256(r.stdout).hexdigest();blob=out/'blobs'/digest;blob.parent.mkdir(exist_ok=True);blob.write_bytes(r.stdout)
  step.update(stdoutArtifact=str(blob.relative_to(out)),stdoutSHA256=digest)
 else:step['stdout']=r.stdout.decode(errors='replace')
 report['steps'].append(step);save();return r
def required(argv,cwd):
 r=run(argv,cwd)
 if r.returncode:raise RuntimeError('command failed: '+repr(argv))
 return r.stdout
save()
try:
 with tempfile.TemporaryDirectory(prefix='toolkit-checkout-') as tmp:
  tmp=pathlib.Path(tmp);source=tmp/'source';source.mkdir()
  # Stage current candidate files into a temporary repository, never the user's index.
  files=required(['git','ls-files','--cached','--others','--exclude-standard','-z'],ROOT).decode().split('\0')
  for name in filter(None,files+['.gitattributes','tests/checkout_e2e.py','tests/snapshot_e2e.py']):
   src=ROOT/name
   if src.is_file():
    dst=source/name;dst.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(src,dst)
  if args.without_attributes:(source/'.gitattributes').unlink(missing_ok=True)
  required(['git','init','--quiet'],source)
  for key,value in [('core.autocrlf','false'),('user.name','Checkout E2E'),('user.email','checkout@example.invalid')]:required(['git','config','--local',key,value],source)
  required(['git','add','.'],source);required(['git','-c','commit.gpgsign=false','commit','-qm','Temporary checkout fixture'],source)
  report['fixtureCommit']=required(['git','rev-parse','HEAD'],source).decode().strip()
  required(['git','archive','--format=zip','--output='+str(out/'source.zip'),'HEAD'],source)
  report['sourceArchiveSHA256']=hashlib.sha256((out/'source.zip').read_bytes()).hexdigest()
  for setting in ['false','true']:
   checkout=tmp/('checkout-'+setting);target=out/('autocrlf-'+setting)
   required(['git','-c','core.autocrlf='+setting,'clone','--quiet','--no-local',str(source),str(checkout)],tmp)
   required(['git','config','--local','core.autocrlf',setting],checkout)
   row={'autocrlf':setting,'settings':required(['git','config','--show-origin','--get','core.autocrlf'],checkout).decode(),'files':[]};report['cases'].append(row)
   r=run([sys.executable,'tests/native.py','--expected-platform','linux/amd64','--output',str(target),'--source-commit',report['fixtureCommit']],checkout)
   observed=json.loads((target/'report.json').read_text());manifest=observed['snapshot'];row['snapshot']=manifest
   with zipfile.ZipFile(target/'module-proxy'/manifest['module']/'@v'/(manifest['version']+'.zip')) as z:
    for name in manifest['files']:
     blob=required(['git','show','HEAD:'+name],source);packaged=z.read(manifest['module']+'@'+manifest['version']+'/'+name)
     row['files'].append({'path':name,'commitSHA256':hashlib.sha256(blob).hexdigest(),'checkoutSHA256':hashlib.sha256((checkout/name).read_bytes()).hexdigest(),'packagedSHA256':hashlib.sha256(packaged).hexdigest(),'matchesCommit':packaged==blob and (checkout/name).read_bytes()==blob})
   row['consumerPassed']=r.returncode==0 and observed['passed'] is True and set(observed['stageResults'])=={'source','edit','yaml','audit','budget','retention','resume','resources','index','workspace'}
   row['bytesMatchCommit']=bool(row['files']) and all(x['matchesCommit'] for x in row['files']);save()
  report['assertions']={'bothCheckoutsConsumed':all(r['consumerPassed'] for r in report['cases']),'allModuleBytesMatchCommit':all(r['bytesMatchCommit'] for r in report['cases']),'sameModuleIdentity':len({r['snapshot']['version'] for r in report['cases']})==1,'sameContractIdentity':len({r['snapshot']['contractSHA256'] for r in report['cases']})==1,'sameZIPIdentity':len({r['snapshot']['zipSHA256'] for r in report['cases']})==1}
  # An ordinary development run must consume valid, uncommitted source changes.
  changed=checkout/'source/snapshot.go';changed.write_bytes(changed.read_bytes()+b'\n// Uncommitted checkout E2E input.\n')
  target=out/'working-tree';r=run([sys.executable,'tests/native.py','--expected-platform','linux/amd64','--output',str(target)],checkout)
  observed=json.loads((target/'report.json').read_text())
  report['workingTree']={'stdout':r.stdout.decode(errors='replace'),'stderr':r.stderr.decode(errors='replace'),'exitCode':r.returncode,'snapshot':observed['snapshot'],'changedInputSHA256':hashlib.sha256(changed.read_bytes()).hexdigest(),'gitDiff':required(['git','diff','--','source/snapshot.go'],checkout).decode()}
  report['assertions']['uncommittedSourceConsumed']=r.returncode==0 and observed['passed'] is True and bool(report['workingTree']['gitDiff']) and observed['snapshot']['version']!=manifest['version']
  target=out/'missing-commit';r=run([sys.executable,'tests/native.py','--expected-platform','linux/amd64','--output',str(target),'--source-commit','0'*40],checkout)
  observed=json.loads((target/'report.json').read_text());report['assertions']['missingCommitRejected']=r.returncode!=0 and observed['passed'] is False and not observed['stageResults']
  shutil.rmtree(checkout/'.git')
  target=out/'missing-git';r=run([sys.executable,'tests/native.py','--expected-platform','linux/amd64','--output',str(target),'--source-commit',report['fixtureCommit']],checkout)
  observed=json.loads((target/'report.json').read_text());report['assertions']['missingGitRejected']=r.returncode!=0 and observed['passed'] is False and not observed['stageResults']
  report['passed']=len(report['cases'])==2 and all(report['assertions'].values())
except Exception as e:report.update(error=repr(e),traceback=traceback.format_exc())
finally:save()
print(json.dumps({'passed':report['passed'],'assertions':report['assertions'],'error':report.get('error')}));sys.exit(0 if report['passed'] else 1)
