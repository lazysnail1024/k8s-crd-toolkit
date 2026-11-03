#!/usr/bin/env python3
import pathlib,tempfile,shutil,os,subprocess,json,hashlib,time
root=pathlib.Path(__file__).resolve().parents[1];env=os.environ|{'GOWORK':'off','CGO_ENABLED':'1'};go=env.get('GO') or env['GOROOT']+'/bin/go';steps=[];out=root/'artifacts/vendor-probe';out.mkdir(exist_ok=True)
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
def run(args,c):
 p=subprocess.run([go]+args,cwd=c,env=env,text=True,capture_output=True,timeout=120);r={'argv':[go]+args,'stdout':p.stdout,'stderr':p.stderr,'exit':p.returncode};steps.append(r);return r
with tempfile.TemporaryDirectory(prefix='toolkit-vendor-e2e-') as d:
 d=pathlib.Path(d);shutil.copytree(root/'tests/consumer',d/'consumer');c=d/'consumer'
 with (c/'go.mod').open('a') as f:f.write('\nreplace github.com/lazysnail1024/k8s-crd-toolkit => '+str(root)+'\n')
 run(['version'],c);run(['mod','tidy'],c);run(['mod','vendor'],c)
 restored={}
 for module,dirs in {'github.com/tree-sitter/go-tree-sitter':['include','src'],'github.com/lazysnail1024/k8s-crd-toolkit':['yamlview/internal/parser/grammar/tree_sitter']}.items():
  meta=json.loads(run(['list','-mod=mod','-m','-json',module],c)['stdout'])
  for directory in dirs:
   src=pathlib.Path(meta['Dir'])/directory;dest=c/'vendor'/module/directory;shutil.copytree(src,dest,dirs_exist_ok=True)
   for p in dest.rglob('*'):
    if p.is_file():restored[str(p.relative_to(c))]=sha(p)
 build=run(['build','-mod=vendor','-buildvcs=false','-o',str(out/'consumer'),'.'],c)
 if build['exit']==0:
  for stage in ['budget','yaml','audit']:
   p=subprocess.run([str(out/'consumer'),stage],cwd=c,text=True,capture_output=True,timeout=120);steps.append({'argv':[str(out/'consumer'),stage],'stdout':p.stdout,'stderr':p.stderr,'exit':p.returncode})
 report={'replay':'python3 tests/vendor_e2e.py','steps':steps,'restoredHashes':restored,'testSHA256':sha(pathlib.Path(__file__)),'binarySHA256':sha(out/'consumer') if (out/'consumer').exists() else None,'passed':all(x['exit']==0 for x in steps)}
 for name in ['report.json','report-'+str(time.time_ns())+'.json']:(out/name).write_text(json.dumps(report,indent=2))
 print(json.dumps({'passed':report['passed'],'steps':len(steps)}));raise SystemExit(0 if report['passed'] else 1)
