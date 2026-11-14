#!/usr/bin/env python3
"""Replay process outcomes through the real runner; no mutation campaign runs."""
import json,os,pathlib,shutil,subprocess,sys,tempfile
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'artifacts/fuzz-runner-controls';OUT.mkdir(parents=True,exist_ok=True)
rows=[]
for case in ['named-behavior','build-failure','wrong-oracle','wrong-exit']:
 with tempfile.TemporaryDirectory(prefix='toolkit-fuzz-runner-control-') as tmp:
  tmp=pathlib.Path(tmp);repo=tmp/'repo';shutil.copytree(ROOT,repo,ignore=shutil.ignore_patterns('.git','artifacts','__pycache__'));bin_=tmp/'bin';bin_.mkdir()
  peer=bin_/'go';peer.write_text('#!'+sys.executable+'\n'+'''import os,sys
if sys.argv[1]=='version':print('go version go1.27.3 linux/amd64');sys.exit(0)
reasons={'./source':'position 10: got {0 3}, want {0 4}','./edit':'wrong source accepted','./yamlview':'known mapping changed'}
case=os.environ['CONTROL_CASE'];reason=reasons[sys.argv[2]]
if case=='build-failure':print('[build failed]');print('declared and not used',file=sys.stderr)
elif case=='wrong-oracle':print('fuzz: elapsed: 0s');print('unrelated failure')
else:print('fuzz: elapsed: 0s');print(reason)
sys.exit(2 if case=='wrong-exit' else 1)
''');peer.chmod(0o755)
  cmd=[sys.executable,str(repo/'tests/fuzz_effectiveness.py')];r=subprocess.run(cmd,env=os.environ|{'PATH':str(bin_)+os.pathsep+os.environ['PATH'],'CONTROL_CASE':case},capture_output=True,text=True,timeout=30)
  report=json.loads((repo/'artifacts/fuzz-effectiveness/report.json').read_text());expected=case=='named-behavior';checks={'exit':(r.returncode==0)==expected,'exactTargets':{x['target'] for x in report['cases']}=={'FuzzAtomicEdit','FuzzCoordinates','FuzzParseRecovery'},'classified':all(x['passed']==expected for x in report['cases'])}
  rows.append(dict(caseId=case,purpose='Runner control with recorded oracle messages; not product mutation acceptance',command=cmd,stdout=r.stdout,stderr=r.stderr,exitCode=r.returncode,report=report,checks=checks,passed=all(checks.values())))
report=dict(cases=rows,passed=all(x['passed'] for x in rows));(OUT/'report.json').write_text(json.dumps(report,indent=2));print(json.dumps({'passed':report['passed'],'failed':[r['caseId'] for r in rows if not r['passed']]}));sys.exit(not report['passed'])
