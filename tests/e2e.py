#!/usr/bin/env python3
import hashlib,json,os,pathlib,subprocess,sys,tempfile,shutil,time,zipfile
root=pathlib.Path(__file__).resolve().parents[1]; stage=sys.argv[1]; target=pathlib.Path(sys.argv[2]).resolve() if len(sys.argv)>2 else root
started=time.monotonic()
out=root/'artifacts'/stage;out.mkdir(parents=True,exist_ok=True)
env=os.environ.copy();env.update(GOWORK='off',CGO_ENABLED='1');go=env.get('GO') or (env['GOROOT']+'/bin/go' if env.get('GOROOT') else shutil.which('go'))
def run(cmd,cwd):
 started=time.monotonic()
 try:
  p=subprocess.run(cmd,cwd=cwd,env=env,text=True,capture_output=True,timeout=120);return dict(command=cmd,stdout=p.stdout,stderr=p.stderr,exitCode=p.returncode,elapsedSeconds=round(time.monotonic()-started,6))
 except subprocess.TimeoutExpired as e:return dict(command=cmd,stdout=str(e.stdout),stderr=str(e.stderr),exitCode=124,elapsedSeconds=round(time.monotonic()-started,6))
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def input_file(p):return p.is_file() and not any(name in p.parts for name in ('.git','artifacts','__pycache__'))
with tempfile.TemporaryDirectory() as d:
 d=pathlib.Path(d);shutil.copytree(root/'tests/consumer',d/'consumer');c=d/'consumer'
 with (c/'go.mod').open('a') as f:f.write('\nreplace github.com/lazysnail1024/k8s-crd-toolkit => '+str(target)+'\n')
 # A fresh binary path prevents a failed build from retaining an old success.
 # -trimpath also makes unchanged consumer inputs reusable across temporary roots.
 built=c/('consumer.exe' if os.name=='nt' else 'consumer')
 steps=[run([go,'version'],c),run([env.get('CC') or 'cc','--version'],c),run([go,'mod','tidy'],c),run([go,'build','-trimpath','-buildvcs=false','-o',str(built),'.'],c)]
 if steps[-1]['exitCode']==0:steps.append(run([str(built),stage.split('-')[0]],c))
 report=dict(environment={k:env.get(k) for k in ['GO','GOROOT','GOMODCACHE','GOCACHE','GOWORK','CGO_ENABLED','GOFLAGS','GOTOOLCHAIN','CC']}, targetHashes={str(p.relative_to(target)):sha(p) for p in target.rglob('*') if input_file(p)}, replay='python3 tests/e2e.py '+stage+' '+str(target),stage=stage,steps=steps,hashes={str(p.relative_to(root)):sha(p) for p in root.rglob('*') if input_file(p)},binarySHA256=sha(built) if built.exists() else None)
 # Keep the precise source/test input beside each historical report, including
 # ablated source, so hashes remain verifiable after the checkout changes.
 archive=out/('inputs-'+str(time.time_ns())+'.zip')
 with zipfile.ZipFile(archive,'w',compression=zipfile.ZIP_DEFLATED) as z:
  for prefix,tree in [('target',target),('tests',root/'tests')]:
   for p in tree.rglob('*'):
    if input_file(p):z.write(p,prefix+'/'+str(p.relative_to(tree)))
 report['inputArchive']={'path':str(archive),'sha256':sha(archive)}
 if report['binarySHA256']:
  binary=out/('consumer-'+report['binarySHA256']+('.exe' if os.name=='nt' else ''));shutil.copy2(built,binary);report['retainedBinary']=str(binary)
 report['elapsedSeconds']=round(time.monotonic()-started,6)
 (out/('report-'+str(time.time_ns())+'.json')).write_text(json.dumps(report,ensure_ascii=False,indent=2));(out/'report.json').write_text(json.dumps(report,ensure_ascii=False,indent=2));print(json.dumps(steps[-1],ensure_ascii=False));sys.exit(0 if all(x['exitCode']==0 for x in steps) else 1)
