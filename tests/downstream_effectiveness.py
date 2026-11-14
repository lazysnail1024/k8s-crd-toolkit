#!/usr/bin/env python3
"""Run actual injection controls and a public type fault against pinned consumers."""
import argparse,hashlib,json,os,pathlib,shutil,subprocess,sys
ROOT=pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'tools'))
from ci_report_contracts import downstream
from source_inventory import inventory,digest
MODULE='github.com/lazysnail1024/k8s-crd-toolkit'
p=argparse.ArgumentParser();p.add_argument('--baseline',type=pathlib.Path,default=ROOT/'artifacts/downstream/report.json');p.add_argument('--checkout-cache',type=pathlib.Path);a=p.parse_args()
out=ROOT/'artifacts/downstream-effectiveness';out.mkdir(parents=True,exist_ok=False)
identity={k:os.getenv(k,'local') for k in ('GITHUB_SHA','GITHUB_RUN_ID','GITHUB_RUN_ATTEMPT')}
report=dict(schemaVersion=1,executionIdentity=identity,sourceCommit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=ROOT,text=True).strip(),runnerSHA256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),controls=[],publicBehavior=dict(expectedFailure='hoverType',localSelftestScope='go test -count=1 ./... (package-local tests, not external consumer suites)',steps=[],passed=False),passed=False)
def save(): (out/'report.json').write_text(json.dumps(report,indent=2))
save()
def run(cmd,cwd,steps):
 try:
  p=subprocess.run(cmd,cwd=cwd,env=os.environ|{'GOWORK':'off','CGO_ENABLED':'1'},text=True,capture_output=True,timeout=600)
 except (subprocess.TimeoutExpired,OSError) as error:
  steps.append(dict(command=cmd,cwd=str(cwd),stdout=str(getattr(error,'stdout','')),stderr=str(getattr(error,'stderr',''))+repr(error),exitCode=124 if isinstance(error,subprocess.TimeoutExpired) else 127));save();raise
 steps.append(dict(command=cmd,cwd=str(cwd),stdout=p.stdout,stderr=p.stderr,exitCode=p.returncode));save();return p
try:
 baseline=json.loads(a.baseline.read_text())
 if baseline['executionIdentity']!=identity or baseline['tuple']['toolkit']!=report['sourceCommit']:raise ValueError('baseline is not this candidate execution')
 downstream(baseline,identity);report['tuple']=baseline['tuple'];report['baselineSHA256']=hashlib.sha256(a.baseline.read_bytes()).hexdigest();save()
 for control,reason in [('old-vendor','candidate vendor inventory mismatch'),('wrong-sha','wrong toolkit SHA'),('missing-file','candidate vendor inventory mismatch'),('missing-consumer',None)]:
  dest=out/'controls'/control;row=dict(control=control,expectedReason=reason,steps=[],passed=False);report['controls'].append(row);save()
  cmd=[sys.executable,str(ROOT/'tests/downstream.py'),'--control',control,'--output',str(dest)]
  if a.checkout_cache:cmd+=['--checkout-cache',str(a.checkout_cache.resolve())]
  child=run(cmd,ROOT,row['steps']);value=json.loads((dest/'report.json').read_text())
  row['reportSHA256']=hashlib.sha256((dest/'report.json').read_bytes()).hexdigest();row['passed']=child.returncode==1 and value['passed'] is False and ((reason in value.get('error','')) if reason else len(value['consumers'])==1 and value['consumers'][0]['passed']);save()
 # Independent oracle: CD alias projection's hoverType check detects lost public schema type information.
 fault=report['publicBehavior'];steps=fault['steps'];candidate=out/'candidate';shutil.copytree(ROOT,candidate,ignore=shutil.ignore_patterns('.git','artifacts','__pycache__'))
 path=candidate/'schemaquery/query.go';text=path.read_text();old='return strings.Join(names, " | "), nil';new='return strings.Join(names[:0], " | "), nil'
 if old not in text:raise ValueError('public behavior mutation target absent')
 path.write_text(text.replace(old,new,1));fault['mutation']=dict(file='schemaquery/query.go',before=old,after=new,sha256=hashlib.sha256(path.read_bytes()).hexdigest());save()
 if run(['go','test','-count=1','./...'],candidate,steps).returncode!=0:raise ValueError('mutant local selftests did not pass')
 normal=next(row for row in baseline['consumers'] if row['consumer']=='zed-argocd');source=pathlib.Path(normal['binaryPath']).parent/'checkout';checkout=out/'checkout';shutil.copytree(source,checkout,ignore=shutil.ignore_patterns('.git','__pycache__'));shutil.copyfile(path,checkout/'vendor'/MODULE/'schemaquery/query.go')
 fault['baselineBinarySHA256']=normal['binarySHA256'];fault['baselineInventory']=normal['derivedInventory'];fault['mutantInventory']=inventory(checkout);fault['mutantSourceDigest']=digest(fault['mutantInventory']);save()
 binary=out/'argocd-ls'
 if run(['go','build','-mod=vendor','-trimpath','-buildvcs=false','-o',str(binary),'./cmd/argocd-ls'],checkout,steps).returncode!=0:raise ValueError('public consumer mutant build failed')
 fault['binarySHA256']=hashlib.sha256(binary.read_bytes()).hexdigest();save()
 suite=run([sys.executable,'tests/alias_projection_lsp_e2e.py',str(binary),str(out/'suite')],checkout,steps);value=json.loads((out/'suite/report.json').read_text())
 spec=json.loads((ROOT/'tests/downstream-contract.json').read_text())['zed-argocd']['suites']['tests/alias_projection_lsp_e2e.py']
 expected_failures={'3.0.0-scalar-alias-value','3.5.6-scalar-alias-value'}
 cases=value.get('rows',[])
 if suite.returncode!=1 or suite.stderr or value.get('passed') is not False:raise ValueError('public mutant failed outside the designated behavior')
 if value.get('binarySHA256')!=fault['binarySHA256'] or value.get('runnerSHA256')!=spec['runnerSHA256']:raise ValueError('public mutant binary or runner identity mismatch')
 if len(cases)!=len(spec['cases']) or {row.get('id') for row in cases}!=set(spec['cases']):raise ValueError('public mutant case inventory mismatch')
 pending=[value]
 while pending:
  item=pending.pop()
  if isinstance(item,dict):
   if 'error' in item or 'errors' in item:raise ValueError('public mutant contains an unrelated execution or protocol error')
   pending.extend(item.values())
  elif isinstance(item,list):pending.extend(item)
 for row in cases:
  name=row['id'];checks=row.get('checks',{})
  if set(checks)!=set(spec['cases'][name]):raise ValueError('public mutant assertion inventory mismatch: '+name)
  if row.get('passed') is not (name not in expected_failures):raise ValueError('public mutant unexpected case outcome: '+name)
  for key,actual in checks.items():
   expected=not (name in expected_failures and key=='hoverType')
   if actual is not expected:raise ValueError('public mutant unexpected assertion outcome: '+name+'/'+key)
 if {row['id'] for row in cases if row['checks'].get('hoverType') is False}!=expected_failures:raise ValueError('public mutant did not fail both designated version cases')
 fault['failedCases']=sorted(expected_failures);fault['passed']=True;save()
 report['passed']=len(report['controls'])==4 and all(row['passed'] for row in report['controls']) and fault['passed']
except Exception as error:report['error']=repr(error)
finally:save()
print(json.dumps(dict(passed=report['passed'],error=report.get('error'),report=str(out/'report.json'))));sys.exit(not report['passed'])
