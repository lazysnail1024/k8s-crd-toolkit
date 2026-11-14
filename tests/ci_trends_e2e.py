#!/usr/bin/env python3
"""External controls for timing evidence; these are not performance acceptance."""
import copy,json,pathlib,subprocess,sys,tempfile
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'artifacts/trends-controls';OUT.mkdir(parents=True,exist_ok=True)
base=dict(platform='linux/amd64',toolchain='go1.27.3/gcc13',selection='full-v1',fixture='fixed-fixture',resources='2cpu/8gb',runID='17',attempt='1',sourceCommit='a'*40,evidence='raw-log',passed=True)
rows=[]
for state,count in [('warm',5),('cold',3)]:
 for n in range(count):
  for variant in (['baseline','candidate'] if n%2==0 else ['candidate','baseline']):
   rows.append(base|dict(cacheState=state,variant=variant,pairID=state+str(n),phases={'build':10 if variant=='baseline' else 8}))
cases=[]
for name in ['paired','empty','empty-phases','nonfinite','negative','missing-group','unpaired','mixed-groups','same-order','first-failure']:
 values=copy.deepcopy(rows)
 if name=='empty':values=[]
 if name=='empty-phases':values[0]['phases']={}
 if name=='nonfinite':values[0]['phases']['build']=float('nan')
 if name=='negative':values[0]['phases']['build']=-1
 if name=='missing-group':values[0].pop('toolchain')
 if name=='unpaired':
  for row in values:row.pop('pairID')
 if name=='mixed-groups':
  extra=copy.deepcopy(values)
  for row in extra:row['platform']='darwin/arm64'
  values+=extra
 if name=='same-order':
  values=sorted(values,key=lambda r:(r['cacheState'],r['pairID'],r['variant']))
 if name=='first-failure':values[0].update(passed=False,failureType='product')
 with tempfile.TemporaryDirectory() as tmp:
  tmp=pathlib.Path(tmp);inp=tmp/'samples.json';out=tmp/'summary.json';inp.write_text(json.dumps(values));cmd=[sys.executable,str(ROOT/'tools/ci_trends.py'),str(inp),'--output',str(out)];r=subprocess.run(cmd,capture_output=True,text=True)
  expectedReject=name in ['empty','empty-phases','nonfinite','negative','missing-group'];checks={'exit':(r.returncode!=0)==expectedReject}
  if not expectedReject and r.returncode==0:
   report=json.loads(out.read_text());summary=report['summary'];checks['rawSamples']=report['samples']==values;checks['noSmallP95']=all('p95' not in x for x in summary)
   checks['separateVariants']=all(x.get('variant') in ('baseline','candidate') for x in summary)
   checks['sufficiency']=all(x['sufficient'] for x in summary) if name in ('paired','mixed-groups') else all(not x['sufficient'] for x in summary if x['cacheState']=='warm')
   if name=='mixed-groups':checks['platforms']=len(summary)==8 and {x['platform'] for x in summary}=={'linux/amd64','darwin/arm64'}
   if name=='first-failure':checks['firstFailure']=report['firstFailures']==[values[0]]
  cases.append(dict(caseId=name,command=cmd,stdout=r.stdout,stderr=r.stderr,exitCode=r.returncode,checks=checks,passed=all(checks.values())))
report=dict(schemaVersion=1,cases=cases,passed=all(x['passed'] for x in cases));(OUT/'report.json').write_text(json.dumps(report,indent=2));print(json.dumps({'passed':report['passed'],'failed':[x['caseId'] for x in cases if not x['passed']]}));sys.exit(not report['passed'])
