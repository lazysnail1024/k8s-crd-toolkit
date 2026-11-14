#!/usr/bin/env python3
"""Exercise failed reruns through real outer-runner processes, retaining sentinels."""
import hashlib,json,os,pathlib,shutil,subprocess,sys,tempfile,time
import os
CI_IDENTITY = {k: os.getenv(k, 'local') for k in ('GITHUB_SHA', 'GITHUB_RUN_ID', 'GITHUB_RUN_ATTEMPT')}
ROOT=pathlib.Path(__file__).resolve().parents[1]
out=ROOT/'artifacts/delivery-failures';out.mkdir(parents=True,exist_ok=True)
rows=[]
for case in ['missing-go','missing-cc','invalid-report','timeout','rss-guard-deleted']:
 with tempfile.TemporaryDirectory(prefix='delivery-failure-') as d:
  repo=pathlib.Path(d)/'repo';repo.mkdir()
  for name in ['go.mod','go.sum','LICENSE','CONTRACT.md']:shutil.copyfile(ROOT/name,repo/name)
  for name in ['tests','tools','source','edit','yamlview','yamlvalue','schemaquery','index','workspace']:shutil.copytree(ROOT/name,repo/name,ignore=shutil.ignore_patterns('__pycache__'))
  target=repo/'artifacts/native-delivery';target.mkdir(parents=True)
  sentinel={'passed':True,'previousRun':'sentinel-before-failing-rerun'}
  (target/'report.json').write_text(json.dumps(sentinel))
  env=os.environ.copy();argv=[sys.executable,str(repo/'tests/native_delivery_e2e.py'),'--variant','isolated-source']
  if case=='missing-go':env['GO']=str(repo/'nonexistent-go')
  if case=='missing-cc':env['CC']=str(repo/'nonexistent-cc')
  if case=='invalid-report':
   (repo/'tests/native.py').write_text("import pathlib,sys\nprint('child-output',flush=True)\nprint('child-error',file=sys.stderr,flush=True)\np=pathlib.Path(sys.argv[sys.argv.index('--output')+1]);p.mkdir(parents=True,exist_ok=True);(p/'report.json').write_text('{broken')\nsys.exit(7)\n")
  if case=='timeout':
   (repo/'tests/native.py').write_text("import time,sys\nprint('before-timeout',flush=True)\nprint('timeout-stderr',file=sys.stderr,flush=True)\ntime.sleep(30)\n")
   argv+=['--timeout','0.2']
  if case=='rss-guard-deleted':
   f=repo/'tests/consumer/audit.go';text=f.read_text();old='validSamples && after-before';assert old in text;f.write_text(text.replace(old,'before > 0 && after > 0 && after-before'))
   # Retain a compilable variant: the otherwise unused variable remains read.
   f.write_text(f.read_text().replace('check("resident-growth",','_ = validSamples\n\tcheck("resident-growth",'))
   argv=[sys.executable,str(repo/'tests/native_delivery_e2e.py'),'--variant','rss-failure']
  p=subprocess.run(argv,env=env,capture_output=True,text=True,timeout=300)
  observed=json.loads((target/'report.json').read_text())
  passed=p.returncode!=0 and observed.get('passed') is False and 'previousRun' not in observed and bool(list(target.glob('report-*.json')))
  if case=='invalid-report':passed=passed and any(r.get('exitCode')==7 and 'child-output' in r.get('stdout','') and 'child-error' in r.get('stderr','') for r in observed.get('cases',[]))
  if case=='rss-guard-deleted':
   child=json.loads(next(target.glob('rss-failure-*/report.json')).read_text())
   passed=passed and child.get('stageResults',{}).get('resources',{}).get('resident-growth') is True and any('build' in x['argv'] and x['exitCode']==0 for x in child.get('steps',[]))
  if case=='timeout':passed=passed and any(r.get('timedOut') and 'before-timeout' in r.get('stdout','') and 'timeout-stderr' in r.get('stderr','') for r in observed.get('cases',[]))
  dest=out/(case+'-'+str(time.time_ns()));shutil.copytree(target,dest)
  shutil.copytree(repo/'tests',dest/'test-inputs',ignore=shutil.ignore_patterns('__pycache__'))
  row=dict(case=case,argv=argv,stdout=p.stdout,stderr=p.stderr,exitCode=p.returncode,passed=passed,artifact=str(dest),runnerSHA256=hashlib.sha256((repo/'tests/native_delivery_e2e.py').read_bytes()).hexdigest())
  (dest/'invocation.json').write_text(json.dumps(row,indent=2));rows.append(row)
report={'schemaVersion':1,'executionIdentity':CI_IDENTITY,'cases':rows,'passed':all(r['passed'] for r in rows)}
(out/'report.json').write_text(json.dumps(report,indent=2));print(json.dumps(report));sys.exit(0 if report['passed'] else 1)
