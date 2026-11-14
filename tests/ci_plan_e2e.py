#!/usr/bin/env python3
"""Exercise the selection and evidence gate through their real CLI boundary."""
import json, pathlib, subprocess, sys, tempfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
out=ROOT/'artifacts/ci-plan-controls';out.mkdir(parents=True,exist_ok=True)
rows=[]
def run(name,args,expected):
 p=subprocess.run([sys.executable,str(ROOT/'tools/ci_plan.py'),*args],capture_output=True,text=True)
 rows.append(dict(caseId=name,argv=args,stdout=p.stdout,stderr=p.stderr,exitCode=p.returncode,passed=(p.returncode==0)==expected))
with tempfile.TemporaryDirectory() as d:
 d=pathlib.Path(d);paths=d/'paths.json';plan=d/'plan.json';results=d/'results.json'
 for name,changes,full in [('docs',['README.md'],False),('mixed',['README.md','source/snapshot.go'],True),('header',['yamlview/a.h'],True),('unknown',['new'],True),('deleted',['D:README.md'],True)]:
  paths.write_text(json.dumps(changes));run(name,['plan','--paths',str(paths),'--output',str(plan),'--sha','a'*40],True)
  p=json.loads(plan.read_text());rows[-1]['passed'] &= bool(p['selectedLanes'])==full
 paths.write_text('["source/snapshot.go"]');run('full',['plan','--paths',str(paths),'--output',str(plan),'--sha','a'*40],True)
 p=json.loads(plan.read_text()); good={x:'success' for x in p['selectedLanes']}
 for name,change in [('success',{}),('cancelled',{'consumer':'cancelled'}),('skipped',{'consumer':'skipped'}),('unknown',{'extra':'success'})]:
  results.write_text(json.dumps(good|change));run(name,['gate','--plan',str(plan),'--results',str(results),'--sha','a'*40],name=='success')
 results.write_text(json.dumps(good));run('wrong-sha',['gate','--plan',str(plan),'--results',str(results),'--sha','b'*40],False)
report={'cases':rows,'passed':all(x['passed'] for x in rows)};(out/'report.json').write_text(json.dumps(report,indent=2));sys.exit(not report['passed'])
