#!/usr/bin/env python3
"""Reject damaged real downstream evidence through the public receipt CLI."""
import copy,hashlib,json,os,pathlib,subprocess,sys,tempfile
ROOT=pathlib.Path(__file__).resolve().parents[1];report=json.loads(pathlib.Path(sys.argv[1]).read_text());out=ROOT/'artifacts/downstream-receipt-controls';out.mkdir(parents=True,exist_ok=True);rows=[]
for name in ['valid','missing-binary','missing-build','empty-snapshot','missing-package','missing-consumer','duplicate-consumer','wrong-tuple','empty-suite','missing-case','false-assertion','wrong-runner']:
 value=copy.deepcopy(report)
 if name=='missing-binary':value['consumers'][0].pop('binarySHA256')
 if name=='missing-build':value['consumers'][0]['steps']=[s for s in value['consumers'][0]['steps'] if 'build' not in s['command']]
 if name=='empty-snapshot':value['snapshot']['files']={}
 if name=='missing-package':value['consumers'][0]['compilerDependencies'].pop()
 if name=='missing-consumer':value['consumers'].pop()
 if name=='duplicate-consumer':value['consumers'][1]=value['consumers'][0]
 if name=='wrong-tuple':value['tuple']['toolkit']='0'*40
 if name=='empty-suite':value['consumers'][0]['suiteReports'][0]['evidence']={}
 if name in ('missing-case','false-assertion','wrong-runner'):
  suite=value['consumers'][0]['suiteReports'][0];entry=suite['evidence']['report.json'];old=entry['sha256'];child=json.loads(entry['rawJSON'])
  if name=='missing-case':child['rows'].pop()
  if name=='false-assertion':child['rows'][0]['checks'][next(iter(child['rows'][0]['checks']))]=False
  if name=='wrong-runner':child['runnerSHA256']='0'*64
  entry['rawJSON']=json.dumps(child);entry['sha256']=hashlib.sha256(entry['rawJSON'].encode()).hexdigest()
  suite['files']={k:(entry['sha256'] if v==old else v) for k,v in suite['files'].items()}
 with tempfile.TemporaryDirectory() as tmp:
  tmp=pathlib.Path(tmp);path=tmp/'artifacts/downstream/report.json';path.parent.mkdir(parents=True);path.write_text(json.dumps(value));env=os.environ|report['executionIdentity']
  cmd=[sys.executable,str(ROOT/'tools/ci_receipt.py'),'write','--lane','downstream','--report','artifacts/downstream/report.json','--directory','receipts'];p=subprocess.run(cmd,cwd=tmp,env=env,capture_output=True,text=True)
  rows.append(dict(caseId=name,command=cmd,exitCode=p.returncode,stdout=p.stdout,stderr=p.stderr,passed=(p.returncode==0)==(name=='valid')))
r=dict(cases=rows,passed=all(x['passed'] for x in rows));(out/'report.json').write_text(json.dumps(r,indent=2));print(json.dumps({'passed':r['passed'],'failed':[x['caseId'] for x in rows if not x['passed']]}));sys.exit(not r['passed'])
