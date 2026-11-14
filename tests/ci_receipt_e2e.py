#!/usr/bin/env python3
"""External CLI evidence controls; these test the verifier, not product behavior."""
import copy,hashlib,json,os,pathlib,subprocess,sys,tempfile
ROOT=pathlib.Path(__file__).resolve().parents[1];out=ROOT/'artifacts/receipt-controls';out.mkdir(parents=True,exist_ok=True)
identity={'GITHUB_SHA':'a'*40,'GITHUB_RUN_ID':'17','GITHUB_RUN_ATTEMPT':'2'};env=os.environ.copy()|identity;rows=[]
with tempfile.TemporaryDirectory() as directory:
 d=pathlib.Path(directory);report={'schemaVersion':1,'passed':True,'executionIdentity':identity,'assertions':{x:True for x in json.loads((ROOT/'tests/property-contract.json').read_text())}};raw=json.dumps(report);receipt={'schemaVersion':1,'identity':identity,'lane':'properties','platform':'linux/amd64','reports':[{'path':'artifacts/properties/report.json','sha256':hashlib.sha256(raw.encode()).hexdigest(),'rawJSON':raw}]}
 for case in ['valid','old-attempt','wrong-sha','missing','duplicate','failed','unknown-schema','empty','old-report','missing-case']:
  r=copy.deepcopy(receipt)
  if case=='old-attempt':r['identity']['GITHUB_RUN_ATTEMPT']='1'
  if case=='wrong-sha':r['identity']['GITHUB_SHA']='c'*40
  if case=='failed':r['reports'][0]['rawJSON']=r['reports'][0]['rawJSON'].replace('true','false')
  if case=='unknown-schema':r['schemaVersion']=99
  if case=='empty':r['reports']=[]
  if case in ('old-report','missing-case'):
   value=json.loads(r['reports'][0]['rawJSON'])
   if case=='old-report':value['executionIdentity']['GITHUB_RUN_ATTEMPT']='1'
   else:value['assertions'].pop(next(iter(value['assertions'])))
   changed=json.dumps(value);r['reports'][0].update(rawJSON=changed,sha256=hashlib.sha256(changed.encode()).hexdigest())
  for f in d.glob('*.json'):f.unlink()
  if case!='missing':(d/'a.json').write_text(json.dumps(r))
  if case=='duplicate':(d/'b.json').write_text(json.dumps(r))
  cmd=[sys.executable,str(ROOT/'tools/ci_receipt.py'),'verify','--directory',str(d),'--lanes','["properties"]'];p=subprocess.run(cmd,capture_output=True,text=True,env=env)
  rows.append(dict(caseId=case,command=cmd,stdout=p.stdout,stderr=p.stderr,exitCode=p.returncode,passed=(p.returncode==0)==(case=='valid')))
r={'cases':rows,'passed':all(x['passed'] for x in rows)};(out/'report.json').write_text(json.dumps(r,indent=2));sys.exit(not r['passed'])
