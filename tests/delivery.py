#!/usr/bin/env python3
"""Build the public consumer from an explicit module zip with a fresh offline cache."""
import hashlib,json,os,pathlib,shutil,subprocess,sys,tempfile,time,zipfile
from assertions import parse_assertions
root=pathlib.Path(__file__).resolve().parents[1]
manifest_path=pathlib.Path(sys.argv[1]).resolve();proxy_path=pathlib.Path(sys.argv[2]).resolve();manifest=json.loads(manifest_path.read_text())
env=os.environ.copy();env.update(GOWORK='off',CGO_ENABLED='1',GOTOOLCHAIN='local');go=env.get('GO') or (env['GOROOT']+'/bin/go' if env.get('GOROOT') else shutil.which('go'))
cache=pathlib.Path(env['GOMODCACHE'])/'cache/download';out=root/'artifacts/delivery';out.mkdir(parents=True,exist_ok=True)
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
report={'replay':f'python3 tests/delivery.py {manifest_path} {proxy_path}','manifest':manifest,'steps':[],'assertions':{},'stageResults':{},'testHashes':{str(p.relative_to(root)):sha(p) for p in (root/'tests').rglob('*') if p.is_file()},'proxyHashes':{}}
def run(argv,cwd):
 try:
  p=subprocess.run(argv,cwd=cwd,env=env,text=True,capture_output=True,timeout=120);r=dict(command=argv,stdout=p.stdout,stderr=p.stderr,exitCode=p.returncode)
 except subprocess.TimeoutExpired as e:r=dict(command=argv,stdout=str(e.stdout),stderr=str(e.stderr),exitCode=124)
 report['steps'].append(r);return r
try:
 with tempfile.TemporaryDirectory(prefix='toolkit-delivery-') as tmp:
  tmp=pathlib.Path(tmp);proxy=tmp/'proxy';consumer=tmp/'consumer';shutil.copytree(root/'tests/consumer',consumer)
  # Graph-only transitive dependencies can require .info records without
  # appearing in the runtime package's checked-in go.sum. Resolve that graph
  # in a disposable module, preserving the source snapshot and repository
  # locks, before switching the consumer to its empty offline cache.
  seed=tmp/'dependency-seed';seed.mkdir()
  for name in ['go.mod','go.sum']:shutil.copyfile(root/name,seed/name)
  seeded=run([go,'mod','download','all'],seed)
  if seeded['exitCode']:raise RuntimeError('dependency graph preparation failed')
  report['dependencySeedGoSum']=(seed/'go.sum').read_text()
  module=manifest['module'];version=manifest['version'];base=proxy_path/module/'@v';z=base/(version+'.zip')
  report['assertions']['snapshot-zip-hash']=sha(z)==manifest['zipSHA256']
  # Copy only immutable downloaded artifacts named by the checked-in go.sum,
  # plus the selected local module. No source checkout is available to Go.
  modules={(line.split()[0],line.split()[1].removesuffix('/go.mod')) for line in report['dependencySeedGoSum'].splitlines()}
  for mod,ver in sorted(modules|{(module,version)}):
   src=base if mod==module else cache/mod/'@v';dst=proxy/mod/'@v';dst.mkdir(parents=True,exist_ok=True)
   for ext in ['mod','info','zip']:
    p=src/(ver+'.'+ext)
    if p.exists():shutil.copyfile(p,dst/p.name);report['proxyHashes'][str((dst/p.name).relative_to(proxy))]=sha(p)
   (dst/'list').write_text(ver+'\n')
  (consumer/'go.mod').write_text('module toolkit-consumer\n\ngo 1.27.3\n\nrequire '+module+' '+version+'\n')
  (consumer/'go.sum').write_bytes((root/'go.sum').read_bytes())
  bundle=out/('inputs-'+str(time.time_ns())+'.zip')
  with zipfile.ZipFile(bundle,'w',compression=zipfile.ZIP_DEFLATED) as z:
   z.write(manifest_path,'manifest.json')
   z.write(seed/'go.mod','dependency-seed/go.mod');z.write(seed/'go.sum','dependency-seed/go.sum')
   for prefix,tree in [('proxy',proxy),('consumer',consumer)]:
    for p in tree.rglob('*'):
     if p.is_file():z.write(p,prefix+'/'+str(p.relative_to(tree)))
  report['inputArchive']={'path':str(bundle),'sha256':sha(bundle)}
  env.update(GOMODCACHE=str(tmp/'empty-mod-cache'),GOPROXY=proxy.as_uri(),GOSUMDB='off',GOWORK='off',CGO_ENABLED='1',GOTOOLCHAIN='local')
  report['environment']={k:env.get(k) for k in ['GOROOT','GO','GOMODCACHE','GOCACHE','GOPROXY','GOSUMDB','GOWORK','CGO_ENABLED','GOTOOLCHAIN']}
  run([go,'version'],consumer);run([env.get('CC') or 'cc','--version'],consumer)
  tidy=run([go,'mod','tidy'],consumer)
  report['consumerGoMod']=(consumer/'go.mod').read_text();report['consumerGoSum']=(consumer/'go.sum').read_text()
  listing=run([go,'list','-m','all'],consumer)
  report['assertions']['no-replace']='replace' not in report['consumerGoMod'] and '=>' not in listing['stdout']
  report['assertions']['exact-version']=module+' '+version in listing['stdout']
  build=run([go,'build','-mod=readonly','-trimpath','-buildvcs=false','-o',str(out/'consumer'),'.'],consumer)
  run([go,'mod','verify'],consumer)
  if build['exitCode']==0:
   report['binarySHA256']=sha(out/'consumer');shutil.copyfile(out/'consumer',out/('consumer-'+report['binarySHA256']));run([go,'version','-m',str(out/'consumer')],consumer)
   for stage in ['source','edit','yaml','yamlvalue','schemaquery','audit','resources','index','workspace']:
    result=run([str(out/'consumer'),stage],consumer)
    try:assertions=parse_assertions(result['stdout']);report['stageResults'][stage]=assertions;report['assertions'][stage]=result['exitCode']==0 and all(assertions.values())
    except Exception:report['assertions'][stage]=False
except Exception as e:report['error']=repr(e)
finally:
 report['assertions']['requiredNonemptySuites']=set(report['stageResults'])=={'source','edit','yaml','yamlvalue','schemaquery','audit','resources','index','workspace'}
 report['passed']='error' not in report and bool(report['assertions']) and all(report['assertions'].values()) and all(s['exitCode']==0 for s in report['steps'])
 text=json.dumps(report,ensure_ascii=False,indent=2);(out/'report.json').write_text(text);(out/('report-'+str(time.time_ns())+'.json')).write_text(text)
 print(json.dumps({'passed':report['passed'],'assertions':report['assertions'],'error':report.get('error')}));sys.exit(0 if report['passed'] else 1)
