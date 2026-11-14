#!/usr/bin/env python3
"""Run existing public consumers natively from a standalone content-pinned module."""
import argparse,hashlib,json,os,pathlib,platform,shutil,subprocess,sys,tempfile,time,zipfile
from assertions import parse_assertions, exact_stage_inventory
import os
CI_IDENTITY = {k: os.getenv(k, 'local') for k in ('GITHUB_SHA', 'GITHUB_RUN_ID', 'GITHUB_RUN_ATTEMPT')}
ROOT=pathlib.Path(__file__).resolve().parents[1];sys.path.insert(0,str(ROOT/'tools'));from snapshot import create
p=argparse.ArgumentParser();p.add_argument('--expected-platform',required=True);p.add_argument('--cc-version');p.add_argument('--source-commit',help='require module bytes to match this Git commit');p.add_argument('--output',type=pathlib.Path,default=ROOT/'artifacts/native');args=p.parse_args();out=args.output.resolve();out.mkdir(parents=True,exist_ok=True)
env=os.environ.copy();env.update(GOWORK='off',CGO_ENABLED='1',GOTOOLCHAIN='local');go=env.get('GO') or (str(pathlib.Path(env['GOROOT'])/'bin/go') if env.get('GOROOT') else shutil.which('go'));cc=env.get('CC') or ('gcc' if os.name=='nt' else 'cc')
report={'executionIdentity':CI_IDENTITY,'schemaVersion':1,'buildRole':'ordinary','buildMode':'public-consumer','productVersion':((ROOT/'VERSION').read_text().strip() if (ROOT/'VERSION').exists() else 'unavailable'),'producer':{'runID':os.getenv('GITHUB_RUN_ID','local'),'attempt':os.getenv('GITHUB_RUN_ATTEMPT','1'),'job':os.getenv('GITHUB_JOB','local')},'sourceCommit':args.source_commit or (subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip() if (ROOT/'.git').exists() else None),'contractDigest': hashlib.sha256((ROOT/'CONTRACT.md').read_bytes()).hexdigest(),'expectedPlatform':args.expected_platform,'python':sys.version,'host':platform.platform(),'runnerImage':{k:env.get(k) for k in ('ImageOS','ImageVersion','RUNNER_OS','RUNNER_ARCH')},'steps':[],'assertions':{},'stageResults':{}}
started=time.monotonic()
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(argv,cwd,timeout=180):
 started=time.monotonic()
 try:
  r=subprocess.run(argv,cwd=cwd,env=env,text=True,capture_output=True,timeout=timeout);step=dict(argv=argv,stdout=r.stdout,stderr=r.stderr,exitCode=r.returncode)
 except Exception as e:step=dict(argv=argv,stdout=str(getattr(e,'stdout','')),stderr=str(getattr(e,'stderr',''))+'\n'+repr(e),exitCode=124 if isinstance(e,subprocess.TimeoutExpired) else 127)
 step['elapsedSeconds']=round(time.monotonic()-started,6);report['steps'].append(step)
 return step
def required(argv,cwd):
 r=run(argv,cwd)
 if r['exitCode']:raise RuntimeError('command failed: '+repr(argv))
 return r['stdout']
try:
 if not go:raise RuntimeError('Go not found')
 info=json.loads(required([go,'env','-json','GOHOSTOS','GOHOSTARCH','GOOS','GOARCH','GOVERSION','GOROOT','GOMODCACHE','CC'],ROOT));report['goEnvironment']=info
 expected='/'.join([info['GOHOSTOS'],info['GOHOSTARCH']]);report['assertions']['nativeHost']=expected==args.expected_platform and expected==info['GOOS']+'/'+info['GOARCH']
 if not report['assertions']['nativeHost']:raise RuntimeError('native platform mismatch; cross compilation is not acceptance')
 version=next(x.split()[1] for x in (ROOT/'go.mod').read_text().splitlines() if x.startswith('go '));report['assertions']['goVersion']=info['GOVERSION']=='go'+version
 if not report['assertions']['goVersion']:raise RuntimeError('Go toolchain must match go.mod')
 cc_output=required([cc,'--version'],ROOT);report['compilerVersion']=cc_output
 if args.cc_version:
  report['assertions']['cCompilerVersion']=args.cc_version in cc_output
  if not report['assertions']['cCompilerVersion']:raise RuntimeError('C compiler version does not match pinned CI toolchain')
 manifest=create(out/'module-proxy');report['snapshot']=manifest;report['dependencyDigest']=sha(ROOT/'go.sum');report['sourceTree']=subprocess.check_output(['git','rev-parse','HEAD^{tree}'],cwd=ROOT,text=True).strip() if (ROOT/'.git').exists() else None;report['dirty']=bool(subprocess.check_output(['git','status','--porcelain','--untracked-files=no'],cwd=ROOT,text=True).strip()) if (ROOT/'.git').exists() else True
 report['gitProvenance']={'available':(ROOT/'.git').exists()}
 if args.source_commit:
  if not report['gitProvenance']['available']:raise RuntimeError('source commit verification requires Git metadata')
  provenance=report['gitProvenance'];provenance['commit']=required(['git','rev-parse','--verify',args.source_commit+'^{commit}'],ROOT).strip();provenance['files']={}
  # Read the complete committed runtime inventory once. Comparing both path
  # sets also rejects deleted non-Go inputs which a successful build can miss.
  listing=required(['git','ls-tree','-r','-z','--full-tree',provenance['commit'],'--','go.mod','go.sum','LICENSE','CONTRACT.md','source','edit','yamlview','yamlvalue','schemaquery','index','workspace'],ROOT)
  committed={}
  for entry in filter(None,listing.split('\0')):
   metadata,name=entry.split('\t',1);mode,kind,object_id=metadata.split()
   if name.endswith('_test.go'):continue
   if kind!='blob' or name in committed:raise RuntimeError('invalid committed runtime entry: '+name)
   committed[name]=object_id
  names=sorted(manifest['files'])
  if not names:raise RuntimeError('empty runtime snapshot')
  objects=required(['git','hash-object','--no-filters','--']+[str(ROOT/name) for name in names],ROOT).splitlines()
  if len(objects)!=len(names):raise RuntimeError('incomplete checkout blob hashes')
  checked=dict(zip(names,objects))
  for name in sorted(set(committed)|set(names)):
   expected=committed.get(name);actual=checked.get(name);digest=manifest['files'].get(name)
   provenance['files'][name]={'commitObject':expected,'checkoutObject':actual,'matchesCommit':expected is not None and expected==actual and digest is not None and sha(ROOT/name)==digest}
  report['assertions']['snapshotMatchesCommit']=bool(provenance['files']) and all(v['matchesCommit'] for v in provenance['files'].values())
  if not report['assertions']['snapshotMatchesCommit']:raise RuntimeError('module snapshot differs from committed source bytes')
 with tempfile.TemporaryDirectory(prefix='toolkit-native-') as directory:
  c=pathlib.Path(directory)/'consumer';shutil.copytree(ROOT/'tests/consumer',c)
  (c/'go.mod').write_text('module toolkit-native-consumer\n\ngo '+version+'\n\nrequire '+manifest['module']+' '+manifest['version']+'\n')
  (c/'go.sum').write_bytes((ROOT/'go.sum').read_bytes());env['GOPROXY']=(out/'module-proxy').as_uri()+','+env.get('GOPROXY','https://proxy.golang.org');env['GONOSUMDB']=manifest['module'];env['CC']=cc
  report['environment']={k:env.get(k) for k in ('CC','GOWORK','CGO_ENABLED','GOTOOLCHAIN','GOPROXY','GONOSUMDB')}
  required([go,'mod','tidy'],c);required([go,'mod','verify'],c);report['moduleGraph']=required([go,'list','-m','all'],c);report['assertions']['noReplace']='=>' not in report['moduleGraph'] and 'replace' not in (c/'go.mod').read_text()
  report['consumerGoMod']=(c/'go.mod').read_text();report['consumerGoSum']=(c/'go.sum').read_text()
  archive=out/'inputs.zip'
  with zipfile.ZipFile(archive,'w',compression=zipfile.ZIP_DEFLATED) as z:
   for prefix,tree in [('consumer',c),('tools',ROOT/'tools'),('tests',ROOT/'tests')]:
    for f in tree.rglob('*'):
     if f.is_file() and '__pycache__' not in f.parts:z.write(f,prefix+'/'+f.relative_to(tree).as_posix())
  report['inputArchiveSHA256']=sha(archive)
  binary=out/('consumer.exe' if os.name=='nt' else 'consumer');required([go,'build','-trimpath','-mod=readonly','-buildvcs=false','-o',str(binary),'.'],c);report['binarySHA256']=sha(binary);required([go,'version','-m',str(binary)],c)
  licenses=out/'licenses';licenses.mkdir(exist_ok=True);shutil.copyfile(ROOT/'LICENSE',licenses/'toolkit-MIT');shutil.copyfile(pathlib.Path(info['GOROOT'])/'LICENSE',licenses/'Go-BSD');shutil.copyfile(ROOT/'yamlview/internal/parser/grammar/LICENSE',licenses/'yaml-grammar-MIT')
  for module in ['github.com/tree-sitter/go-tree-sitter','github.com/mattn/go-pointer','github.com/santhosh-tekuri/jsonschema/v6','gopkg.in/yaml.v3','sigs.k8s.io/yaml','go.yaml.in/yaml/v2','golang.org/x/text']:
   meta=json.loads(required([go,'list','-m','-json',module],c));source=pathlib.Path(meta['Dir']);name=module.replace('/','-')
   shutil.copyfile(source/'LICENSE',licenses/(name+'-LICENSE'))
   if (source/'src/unicode/LICENSE').is_file():shutil.copyfile(source/'src/unicode/LICENSE',licenses/'tree-sitter-Unicode')
  for stage in ['source','edit','yaml','yamlvalue','schemaquery','audit','budget','retention','resume','resources','index','workspace']:
   r=run([str(binary),stage],c)
   try:values=parse_assertions(r['stdout']);report['stageResults'][stage]=values;report['assertions'][stage]=r['exitCode']==0 and all(values.values())
   except Exception:report['assertions'][stage]=False
except Exception as e:report['error']=repr(e)
finally:
 report['elapsedSeconds']=round(time.monotonic()-started,6)
 stage_contract=json.loads((ROOT/'tests/native-contract.json').read_text())
 report['assertions']['exactAssertionInventory']=exact_stage_inventory(report['stageResults'],stage_contract,args.expected_platform)
 report['assertions']['requiredNonemptySuites']=set(report['stageResults'])=={'source','edit','yaml','yamlvalue','schemaquery','audit','budget','retention','resume','resources','index','workspace'} and all(bool(v) for v in report['stageResults'].values())
 report['passed']='error' not in report and bool(report['assertions']) and all(report['assertions'].values()) and all(s['exitCode']==0 for s in report['steps']);report['replay']=sys.argv
 text=json.dumps(report,indent=2);(out/'report.json').write_text(text);(out/('report-'+str(time.time_ns())+'.json')).write_text(text);print(json.dumps({'passed':report['passed'],'assertions':report['assertions'],'error':report.get('error')}));sys.exit(0 if report['passed'] else 1)
