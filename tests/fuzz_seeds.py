#!/usr/bin/env python3
"""Run deterministic fuzz seeds on the actual native Go host; never claim mutation."""
import argparse,hashlib,json,os,pathlib,subprocess,sys,time
ROOT=pathlib.Path(__file__).resolve().parents[1]
identity={k:os.getenv(k,'local') for k in ('GITHUB_SHA','GITHUB_RUN_ID','GITHUB_RUN_ATTEMPT')}
p=argparse.ArgumentParser();p.add_argument('--platform',required=True);p.add_argument('--output',type=pathlib.Path,default=ROOT/'artifacts/fuzz-seeds');a=p.parse_args();a.output.mkdir(parents=True,exist_ok=True)
report=dict(schemaVersion=1,executionIdentity=identity,buildRole='fuzz-seed-replay',platform=a.platform,steps=[],passed=False)
try:
 env=os.environ|{'GOWORK':'off','CGO_ENABLED':'1','GOTOOLCHAIN':'local'}
 host=json.loads(subprocess.check_output(['go','env','-json','GOHOSTOS','GOHOSTARCH','GOOS','GOARCH','GOVERSION'],env=env,text=True));report['goEnvironment']=host
 if host['GOHOSTOS']+'/'+host['GOHOSTARCH']!=a.platform or host['GOOS']+'/'+host['GOARCH']!=a.platform:raise ValueError('seed replay is not native')
 command=['go','test','-json','-count=1','-run=^Fuzz','./source','./edit','./yamlview'];start=time.monotonic();r=subprocess.run(command,cwd=ROOT,env=env,capture_output=True,text=True,timeout=240)
 report['steps'].append(dict(command=command,stdout=r.stdout,stderr=r.stderr,exitCode=r.returncode,elapsedSeconds=time.monotonic()-start))
 events=[json.loads(line) for line in r.stdout.splitlines()];report['completed']=[x['Test'] for x in events if x.get('Action')=='pass' and 'Test' in x]
 wanted={'FuzzCoordinates':4,'FuzzAtomicEdit':2,'FuzzParseRecovery':3}
 report['passed']=r.returncode==0 and all(target in report['completed'] and all(target+'/seed#'+str(i) in report['completed'] for i in range(count)) for target,count in wanted.items())
 report['inputHashes']={str(f.relative_to(ROOT)):hashlib.sha256(f.read_bytes()).hexdigest() for f in ROOT.glob('*/*_fuzz_test.go')}
except Exception as e:report['error']=repr(e)
finally:(a.output/'report.json').write_text(json.dumps(report,indent=2))
print(json.dumps({'passed':report['passed'],'error':report.get('error')}));sys.exit(not report['passed'])
