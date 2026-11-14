#!/usr/bin/env python3
"""Execute the real workflow planning entry against independent diff outcomes."""
import base64,hashlib,json,os,pathlib,re,shutil,subprocess,sys,tempfile,textwrap
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'artifacts/workflow-plan-controls';OUT.mkdir(parents=True,exist_ok=True)
workflow=(ROOT/'.github/workflows/native.yml').read_text();match=re.search(r"python - <<'PYCODE'\n(.*?)\n\s*PYCODE",workflow,re.S)
if not match:raise ValueError('workflow planning entry unavailable')
entry=textwrap.dedent(match.group(1));lanes=['consumer','index_regression','delivery_regression','properties','fuzz','downstream'];rows=[]
scenarios=[
 ('pr-docs','pull_request',b'M\0README.md\0',0,False),
 ('pr-mixed','pull_request',b'M\0README.md\0M\0source/snapshot.go\0',0,True),
 ('pr-header','pull_request',b'M\0yamlview/parser.h\0',0,True),
 ('pr-unknown','pull_request',b'M\0new-path\0',0,True),
 ('pr-ci','pull_request',b'M\0.github/workflows/native.yml\0',0,True),
 ('pr-test','pull_request',b'M\0tests/e2e.py\0',0,True),
 ('pr-delete','pull_request',b'D\0README.md\0',0,True),
 ('pr-rename','pull_request',b'D\0README.md\0A\0DEVELOPMENT.md\0',0,True),
 ('pr-truncated-tail','pull_request',b'M\0README.md',0,True),
 ('pr-truncated-pair','pull_request',b'M\0README.md\0M\0',0,True),
 ('pr-empty','pull_request',b'',0,True),
 ('pr-diff-failure','pull_request',b'M\0README.md\0',1,True),
 ('main-docs','push',b'M\0README.md\0',0,True),
 ('scheduled-docs','schedule',b'M\0README.md\0',0,True),
 ('manual-docs','workflow_dispatch',b'M\0README.md\0',0,True),
]
for name,event,diff,exit_,full in scenarios:
 with tempfile.TemporaryDirectory(prefix='toolkit-workflow-plan-') as tmp:
  tmp=pathlib.Path(tmp);(tmp/'tools').mkdir();shutil.copyfile(ROOT/'tools/ci_plan.py',tmp/'tools/ci_plan.py');bin_=tmp/'bin';bin_.mkdir();git=bin_/'git';git.write_text('#!'+sys.executable+'\nimport os,sys,base64\nsys.stdout.buffer.write(base64.b64decode(os.environ["P02_DIFF_BYTES"]))\nsys.exit(int(os.environ["P02_DIFF_EXIT"]))\n');git.chmod(0o755)
  env=os.environ|{'PATH':str(bin_)+os.pathsep+os.environ['PATH'],'EVENT':event,'BASE':'a'*40,'HEAD':'b'*40,'GITHUB_SHA':'c'*40,'GITHUB_OUTPUT':str(tmp/'outputs'),'P02_DIFF_BYTES':base64.b64encode(diff).decode(),'P02_DIFF_EXIT':str(exit_)}
  command=[sys.executable,'-c',entry];r=subprocess.run(command,cwd=tmp,env=env,capture_output=True,text=True);plan=json.loads((tmp/'artifacts/plan.json').read_text()) if (tmp/'artifacts/plan.json').exists() else {};checks={'planningSucceeded':r.returncode==0,'selection':set(plan.get('selectedLanes',[]))==(set(lanes) if full else set()),'identity':plan.get('testedSHA')=='c'*40 and plan.get('baseSHA')=='a'*40 and plan.get('headSHA')=='b'*40,'output':(tmp/'outputs').exists() and (tmp/'outputs').read_text()=='full='+str(full).lower()+'\n'}
  steps=[dict(command=command,stdout=r.stdout,stderr=r.stderr,exitCode=r.returncode)]
  if r.returncode==0:
   needs={lane:'success' if full else 'skipped' for lane in lanes};results=tmp/'results.json';results.write_text(json.dumps(needs));gate=[sys.executable,str(tmp/'tools/ci_plan.py'),'gate','--plan',str(tmp/'artifacts/plan.json'),'--results',str(results),'--sha','c'*40];p=subprocess.run(gate,cwd=tmp,env=env,capture_output=True,text=True);steps.append(dict(command=gate,stdout=p.stdout,stderr=p.stderr,exitCode=p.returncode));checks['gateAcceptsDeclaredSelection']=p.returncode==0
   if not full:
    results.write_text(json.dumps(needs|{'consumer':'success'}));p=subprocess.run(gate,cwd=tmp,env=env,capture_output=True,text=True);steps.append(dict(command=gate,stdout=p.stdout,stderr=p.stderr,exitCode=p.returncode));checks['gateRejectsUndeclaredExecution']=p.returncode!=0
  rows.append(dict(caseId=name,event=event,expectedFull=full,inputDiffBase64=base64.b64encode(diff).decode(),diffExitCode=exit_,plan=plan,steps=steps,checks=checks,passed=all(checks.values())))
report=dict(schemaVersion=1,scope='Real workflow entry and planner CLI; controlled Git diff subprocess, not hosted execution',workflowSHA256=hashlib.sha256(workflow.encode()).hexdigest(),entrySHA256=hashlib.sha256(entry.encode()).hexdigest(),cases=rows,passed=all(x['passed'] for x in rows));(OUT/'report.json').write_text(json.dumps(report,indent=2));print(json.dumps({'passed':report['passed'],'failed':[x['caseId'] for x in rows if not x['passed']]}));sys.exit(not report['passed'])
