#!/usr/bin/env python3
"""Exercise producer receipt rejection using independently damaged real reports."""
import copy,json,pathlib,subprocess,sys,os,shutil,hashlib
ROOT=pathlib.Path(__file__).resolve().parents[1]
p=pathlib.Path(sys.argv[1]);lane=sys.argv[2];report=json.loads(p.read_text());out=ROOT/'artifacts/evidence-semantic-controls'/lane;out.mkdir(parents=True,exist_ok=True);rows=[]
names=['valid','seed-only','wrong-budget','wrong-parallel','missing-output','wrong-count','wrong-source','wrong-toolchain-platform','wrong-event-budget'] if lane=='fuzz' else ['valid','wrong-tree','wrong-patch','missing-source','outside-vendor','old-vendor']
for name in names:
 value=copy.deepcopy(report)
 environment=os.environ|value['executionIdentity']
 if lane=='fuzz':
  row=value['campaigns'][0]
  if name=='seed-only':row['command']=['go','test','./source','-run=^FuzzCoordinates$']
  if name=='wrong-budget':row['command']=['-fuzztime=1s' if x.startswith('-fuzztime=') else x for x in row['command']]
  if name=='wrong-parallel':row['command']=[x.replace('-parallel=2','-parallel=1') for x in row['command']]
  if name=='missing-output':row.pop('stdout',None)
  if name=='wrong-count':row['executions']=0
  if name=='wrong-source':value['sourceCommit']='0'*40
  if name=='wrong-toolchain-platform':value['go']=value['go'].rsplit(' ',1)[0]+' windows/amd64'
  if name=='wrong-event-budget':environment['GITHUB_EVENT_NAME']='schedule' if row['budgetSeconds']==60 else 'pull_request'
 else:
  row=value['consumers'][0]
  if name=='wrong-tree':row['effectiveSourceTree']='0'*64
  if name=='wrong-patch':row['patchDigest']='1'*64
  if name=='missing-source':row.get('originalInventory',{}).pop('go.mod',None)
  if name=='outside-vendor':row.get('derivedInventory',{})['go.mod']={'mode':'100644','object':'0'*40}
  if name=='old-vendor':row.get('derivedInventory',{}).pop('vendor/github.com/lazysnail1024/k8s-crd-toolkit/source/snapshot.go',None)
 dest=out/name;path=dest/'artifacts'/lane/'report.json';path.parent.mkdir(parents=True,exist_ok=True);path.write_text(json.dumps(value))
 if lane=='fuzz':
  for stream in p.parent.glob('*.std*'):shutil.copyfile(stream,path.parent/stream.name)
 cmd=[sys.executable,str(ROOT/'tools/ci_receipt.py'),'write','--lane',lane,'--report','artifacts/'+lane+'/report.json','--directory','receipts'];r=subprocess.run(cmd,cwd=dest,env=environment,capture_output=True,text=True)
 independent=dest/'independent-receipts';independent.mkdir(exist_ok=True)
 raw=path.read_text();envelope=dict(schemaVersion=1,identity=value['executionIdentity'],lane=lane,platform='linux/amd64',reports=[dict(path='artifacts/'+lane+'/report.json',sha256=hashlib.sha256(raw.encode()).hexdigest(),rawJSON=raw)])
 (independent/(lane+'.json')).write_text(json.dumps(envelope))
 verify_command=[sys.executable,str(ROOT/'tools/ci_receipt.py'),'verify','--directory',str(independent),'--lanes',json.dumps([lane])]
 verified=subprocess.run(verify_command,cwd=ROOT,env=environment,text=True,capture_output=True)
 verification=dict(command=verify_command,exitCode=verified.returncode,stdout=verified.stdout,stderr=verified.stderr)
 chain=[]
 if len(sys.argv)>3:
  receipts=dest/'full-receipts';shutil.copytree(pathlib.Path(sys.argv[3]),receipts,dirs_exist_ok=True)
  for receipt in receipts.rglob('*.json'):
   envelope=json.loads(receipt.read_text())
   if envelope['lane']==lane:
    import hashlib
    raw=path.read_text();envelope['reports'][0].update(rawJSON=raw,sha256=hashlib.sha256(raw.encode()).hexdigest());receipt.write_text(json.dumps(envelope))
  plan=pathlib.Path(sys.argv[4]).resolve();results=pathlib.Path(sys.argv[5]).resolve()
  commands=[[sys.executable,str(ROOT/'tools/ci_plan.py'),'gate','--plan',str(plan),'--results',str(results),'--sha',value['executionIdentity']['GITHUB_SHA']],[sys.executable,str(ROOT/'tools/ci_receipt.py'),'verify','--directory',str(receipts.resolve()),'--lanes',json.dumps(json.loads(plan.read_text())['selectedLanes'])]]
  for command in commands:
   child=subprocess.run(command,cwd=ROOT,env=environment,text=True,capture_output=True);chain.append(dict(command=command,stdout=child.stdout,stderr=child.stderr,exitCode=child.returncode))
 rows.append(dict(environment={k:environment.get(k) for k in ('GITHUB_SHA','GITHUB_RUN_ID','GITHUB_RUN_ATTEMPT','GITHUB_EVENT_NAME')},verification=verification,chain=chain,case=name,command=cmd,exitCode=r.returncode,stdout=r.stdout,stderr=r.stderr,passed=(r.returncode==0)==(name=='valid') and (verified.returncode==0)==(name=='valid') and (not chain or (chain[0]['exitCode']==0 and (chain[1]['exitCode']==0)==(name=='valid')))))
(out/'report.json').write_text(json.dumps(dict(cases=rows,passed=all(x['passed'] for x in rows)),indent=2));print([(x['case'],x['passed']) for x in rows]);sys.exit(not all(x['passed'] for x in rows))
