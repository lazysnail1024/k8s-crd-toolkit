#!/usr/bin/env python3
"""Run three bounded mutation campaigns, retaining raw output and failure corpus."""
import argparse,hashlib,json,os,pathlib,shutil,subprocess,sys,time
import os
CI_IDENTITY = {k: os.getenv(k, 'local') for k in ('GITHUB_SHA', 'GITHUB_RUN_ID', 'GITHUB_RUN_ATTEMPT')}
ROOT=pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'tools'))
from fuzz_evidence import hashes, count
p=argparse.ArgumentParser();p.add_argument('--seconds',type=int,default=60);p.add_argument('--output',type=pathlib.Path,default=ROOT/'artifacts/fuzz');a=p.parse_args()
if a.seconds not in (60,600):raise SystemExit('budget must be 60 or 600 seconds')
a.output.mkdir(parents=True,exist_ok=True);rows=[]
# Save incomplete attempts before any tool can fail or the process is interrupted.
checkpoint=dict(schemaVersion=1,executionIdentity=CI_IDENTITY,buildRole='fuzz',campaigns=rows,passed=False,status='incomplete')
def checkpoint_save(): (a.output/'report.json').write_text(json.dumps(checkpoint,indent=2))
checkpoint_save()
env=os.environ.copy();env.update(GOWORK='off',CGO_ENABLED='1',GOMAXPROCS='2')
for package,target in [('source','FuzzCoordinates'),('edit','FuzzAtomicEdit'),('yamlview','FuzzParseRecovery')]:
 cmd=['go','test','./'+package,'-run=^$','-fuzz=^'+target+'$','-fuzztime='+str(a.seconds)+'s','-parallel=2','-fuzzminimizetime=30s'];start=time.monotonic()
 try:
  r=subprocess.run(cmd,cwd=ROOT,env=env,text=True,capture_output=True,timeout=a.seconds+180);stdout,stderr,code=r.stdout,r.stderr,r.returncode
 except subprocess.TimeoutExpired as e:stdout=str(e.stdout);stderr=str(e.stderr);code=124
 except OSError as e:stdout='';stderr=repr(e);code=127
 (a.output/(package+'.stdout')).write_text(stdout);(a.output/(package+'.stderr')).write_text(stderr)
 corpus=ROOT/package/'testdata/fuzz'
 if corpus.exists():shutil.copytree(corpus,a.output/(package+'-failures'),dirs_exist_ok=True)
 try:executions=count(stdout)
 except ValueError:executions=0
 rows.append(dict(package=package,budgetSeconds=a.seconds,executionIdentity=CI_IDENTITY,stdout=stdout,stderr=stderr,stdoutSHA256=hashlib.sha256(stdout.encode()).hexdigest(),stderrSHA256=hashlib.sha256(stderr.encode()).hexdigest(),executions=executions,target=target,command=cmd,exitCode=code,elapsedSeconds=time.monotonic()-start,mutationObserved=executions>0,passed=code==0 and executions>0))
 checkpoint_save()
try:go_version=subprocess.check_output(['go','version'],text=True).strip()
except Exception as error:go_version='unavailable: '+repr(error)
report=dict(executionIdentity=CI_IDENTITY,schemaVersion=1,buildRole='fuzz',sourceCommit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),go=go_version,runID=os.getenv('GITHUB_RUN_ID','local'),attempt=os.getenv('GITHUB_RUN_ATTEMPT','1'),campaigns=rows,passed=all(r['passed'] for r in rows),testHashes=hashes(ROOT))
(a.output/'report.json').write_text(json.dumps(report,indent=2));print(json.dumps(report));sys.exit(not report['passed'])
