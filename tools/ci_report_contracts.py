"""Authoritative behavior inventories for existing regression and downstream reports."""
import hashlib,json,pathlib
from source_inventory import verify as verify_source_inventory
ROOT=pathlib.Path(__file__).resolve().parents[1]
MODULE='github.com/lazysnail1024/k8s-crd-toolkit'
def require(ok,message):
 if not ok:raise ValueError(message)
def digest(value):return hashlib.sha256(value).hexdigest()
def hash_string(value):return isinstance(value,str) and len(value)==64 and all(x in '0123456789abcdef' for x in value)
def true_map(value,names):
 require(isinstance(value,dict) and set(value)==set(names) and all(type(x) is bool and x for x in value.values()),'missing, extra or false behavior assertions')
def steps(value,success=True):
 require(isinstance(value,list) and bool(value),'missing process execution')
 for row in value:
  require(isinstance(row.get('argv',row.get('command')),list) and bool(row.get('argv',row.get('command'))),'missing command')
  code=row.get('exitCode',row.get('exit'));require(type(code) is int and (not success or code==0),'process did not succeed')
  require(isinstance(row.get('stderr'),str) and (isinstance(row.get('stdout'),str) or hash_string(row.get('stdoutSHA256'))),'missing process output')
def cases(value,names,key='case'):
 require(isinstance(value,list) and len(value)==len(names) and {r.get(key) for r in value}==set(names),'missing, duplicate or unexpected cases')
 require(all(r.get('passed') is True and 'error' not in r for r in value),'failed case')
def stage_sets(report,names):
 contract=json.loads((ROOT/'tests/native-contract.json').read_text())['stages']
 require(set(report.get('stageResults',{}))==set(names),'stage inventory mismatch')
 for name in names:true_map(report['stageResults'][name],contract[name])
def consumer(report,stage):
 true_map(report.get('assertions'),json.loads((ROOT/'tests/native-contract.json').read_text())['stages'][stage])
 require(report.get('buildSucceeded') is True and report.get('consumerExitCode')==0 and hash_string(report.get('binarySHA256')),'missing successful compiled consumer')
 steps(report.get('steps'));require(json.loads(report['steps'][-1]['stdout'])==report['assertions'],'consumer stdout differs from assertions')
def regression(report,path):
 name=path.split('/')[-2]
 if name in ('index-race','workspace-race'):
  consumer(report,name.split('-')[0]);require(report.get('stage')==name and '-race' in report.get('environment',{}).get('GOFLAGS',''),'race consumer not executed')
 elif name=='ablations':
  mutations=json.loads((ROOT/'docs/testing-ci-baseline.json').read_text())['mutations'];cases(report.get('cases'),[m['id'] for m in mutations],'variant')
  for row,mutation in zip(report['cases'],mutations):
   require(row['variant']==mutation['id'] and row.get('expectedFailedAssertion')==mutation['expectedFailure'],'mutation oracle mismatch')
   before=row.get('baselineEvidence',{});after=row.get('variantEvidence',{});oracle=mutation['expectedFailure']
   require(before.get('passed') is True and before.get('buildSucceeded') is True and before.get('consumerExitCode')==0,'mutation baseline failed')
   true_map(before.get('assertions'),json.loads((ROOT/'tests/native-contract.json').read_text())['stages'][mutation['id'].split('-')[0]])
   steps(before.get('steps'));steps(after.get('steps'),False)
   require(after.get('passed') is False and after.get('error') is None and after.get('buildSucceeded') is True and after.get('consumerExitCode')==1 and after.get('assertions',{}).get(oracle) is False,'mutation not detected by named behavior')
   require(all(x.get('exitCode')==0 for x in after['steps'][:-1]) and after['steps'][-1].get('exitCode')==1,'mutation failed outside behavior')
   require(json.loads(after['steps'][-1]['stdout'])==after['assertions'],'mutation stdout mismatch')
 elif name=='runner-effectiveness':
  names=['boolean-success','empty-object','non-object','truthy-string','truthy-number','duplicate-key','false-with-success-exit','true-with-failure-exit','stale-report-missing-tool']
  names+=['ablation-'+x for x in ['named-behavior-failure','baseline-failure','missing-report','malformed-report','unrelated-assertion','success-exit','build-failure','setup-failure','wrong-target']]
  names+=['selection-source-typo','selection-source-source','failure-cannot-be-overwritten','build-index-query','build-index-scope','index-query-4-2-3','index-query-4-8-1','index-query-0-2-1','index-query-4-0-1','index-query-4-2-0','index-query-invalid-2-1','index-query-','index-scope-4','index-scope-0','index-scope-']
  cases(report.get('cases'),names);steps(report['cases'],False)
 elif name=='snapshot':
  true_map(report.get('assertions'),['sameRuntimeFiles','sameVersion','sameSourceIdentity','sameContractIdentity','sameZIPBytes']);steps(report.get('steps'));require(len(report['steps'])==4 and len(report.get('manifests',[]))==4,'snapshot variants missing')
  first=report['manifests'][0];require(bool(first.get('files')),'empty snapshot')
  require(all(all(m[k]==first[k] for k in ('files','version','sourceSHA256','contractSHA256','zipSHA256')) for m in report['manifests']),'snapshot mismatch')
 elif name=='checkout':
  true_map(report.get('assertions'),['bothCheckoutsConsumed','allModuleBytesMatchCommit','batchedProvenance','sameModuleIdentity','sameContractIdentity','sameZIPIdentity','uncommittedSourceConsumed','dirtyCommitRejected','missingRuntimeInputRejected','missingCommitRejected','missingGitRejected'])
  rows=report.get('cases',[]);require(len(rows)==2 and {r.get('autocrlf') for r in rows}=={'true','false'},'missing checkout modes');steps(report.get('steps'),False)
  for row in rows:
   require(all(row.get(k) is True for k in ('consumerPassed','batchedProvenance','bytesMatchCommit')) and bool(row.get('files')) and all(f.get('matchesCommit') is True for f in row['files']),'checkout did not consume exact committed bytes')
 elif name=='vendor-probe':
  stage_sets(report,['budget','yaml','audit']);steps(report.get('steps'));require(bool(report.get('restoredHashes')) and hash_string(report.get('binarySHA256')),'missing vendored native build')
 elif name=='delivery':
  names=['source','edit','yaml','yamlvalue','schemaquery','audit','resources','index','workspace'];stage_sets(report,names);true_map(report.get('assertions'),['snapshot-zip-hash','no-replace','exact-version','requiredNonemptySuites']+names);steps(report.get('steps'));require(hash_string(report.get('binarySHA256')) and report.get('environment',{}).get('GOPROXY','').startswith('file:'),'missing offline build')
 elif name=='native-delivery':
  cases(report.get('cases'),['isolated-source','empty-suite','rss-failure','wrong-platform','missing-compiler'],'variant');steps(report['cases'],False)
  require(all((r['exitCode']==0)==(r['variant']=='isolated-source') and not r.get('timedOut') and hash_string(r.get('inputArchiveSHA256')) for r in report['cases']),'invalid native delivery process outcomes')
 elif name=='delivery-failures':
  cases(report.get('cases'),['missing-go','missing-cc','invalid-report','timeout','rss-guard-deleted']);steps(report['cases'],False);require(all(r['exitCode']!=0 for r in report['cases']),'infrastructure failure not rejected')
 else:raise ValueError('unknown regression report')
def downstream(report,identity):
 pins=json.loads((ROOT/'tests/downstream-pins.json').read_text())['consumers'];contracts=json.loads((ROOT/'tests/downstream-contract.json').read_text());expected={'toolkit':identity['GITHUB_SHA'],**{k:v['commit'] for k,v in pins.items()}}
 require(report.get('buildRole')=='downstream-candidate' and report.get('tuple')==expected,'downstream tuple mismatch')
 runtime=[ROOT/n for n in ('go.mod','go.sum','LICENSE','CONTRACT.md')]
 for directory in ('source','edit','yamlview','yamlvalue','schemaquery','index','workspace'):runtime.extend(p for p in (ROOT/directory).rglob('*') if p.is_file() and not p.name.endswith('_test.go'))
 hashes={p.relative_to(ROOT).as_posix():digest(p.read_bytes()) for p in runtime};snapshot=report.get('snapshot',{})
 require(snapshot.get('files')==hashes and snapshot.get('sourceSHA256')==digest(json.dumps(hashes,sort_keys=True,separators=(',',':')).encode()),'missing or wrong candidate snapshot')
 rows=report.get('consumers',[]);require(len(rows)==2 and {r.get('consumer') for r in rows}==set(pins),'missing or duplicate downstream')
 for row in rows:
  name=row['consumer'];contract=contracts[name];require(row.get('passed') is True and row.get('tuple')==expected and row.get('vendorDigest')==snapshot['sourceSHA256'],'invalid downstream identity')
  require(all(hash_string(row.get(k)) for k in ('binarySHA256','patchDigest','effectiveSourceTree')) and bool(row.get('buildMetadata')),'missing binary or derived source identity');steps(row.get('steps'))
  verify_source_inventory(row,pins[name]['commit'],ROOT,hashes)
  binary=row.get('binaryPath');require(isinstance(binary,str) and pathlib.PurePath(binary).name==pins[name]['command'],'missing actual binary path')
  builds=[x for x in row['steps'] if 'build' in x.get('command',[]) and './cmd/'+pins[name]['command'] in x['command']];require(len(builds)==1 and builds[0]['command'][builds[0]['command'].index('-o')+1]==binary,'binary did not come from expected build')
  deps=row.get('compilerDependencies',[]);require(len(deps)==len(contract['compilerPackages']) and {p.get('ImportPath') for p in deps}==set(contract['compilerPackages']),'compiler package inventory mismatch')
  for package in deps:
   suffix=package['ImportPath'][len(MODULE)+1:];directory=package.get('Dir','').replace('\\','/');require(directory.endswith('/vendor/'+MODULE+'/'+suffix),'compiler resolved another toolkit')
   files=sum([package.get(k,[]) for k in ('GoFiles','CgoFiles','CFiles','HFiles')],[]);require(bool(files) and all(suffix+'/'+f in hashes for f in files),'missing compiler input files')
  suites=row.get('suiteReports',[]);require([x.get('suite') for x in suites]==pins[name]['suites'],'downstream suite inventory mismatch')
  for suite in suites:
   spec=contract['suites'][suite['suite']];files=suite.get('evidence',{});wanted={spec['reportFile']}|({'manifest.json'} if spec['reportFile']=='results.json' else set());require(set(files)==wanted,'missing fresh downstream case evidence')
   for file,evidence in files.items():require(digest(evidence['rawJSON'].encode())==evidence['sha256'] and evidence['sha256'] in suite['files'].values(),'downstream report digest mismatch')
   value=json.loads(files[spec['reportFile']]['rawJSON']);cases_=value.get('rows',[]) if isinstance(value,dict) else value
   require(len(cases_)==len(spec['cases']) and {x.get(spec['caseKey']) for x in cases_}==set(spec['cases']),'missing downstream cases')
   for case in cases_:
    require(case.get(spec['passKey']) is True,'downstream case failed');true_map(case.get(spec['checksKey']),spec['cases'][case[spec['caseKey']]])
   meta=value if isinstance(value,dict) else json.loads(files['manifest.json']['rawJSON']);require(meta.get('binarySHA256',meta.get('binarySha256'))==row['binarySHA256'],'suite used another binary')
   require(meta.get('runnerSHA256',meta.get('testSha256',meta.get('consumerSHA256')))==spec['runnerSHA256'],'suite runner differs from pinned consumer')
   executions=[x for x in row['steps'] if len(x['command'])>=4 and x['command'][1]==suite['suite'] and x['command'][2]==binary];require(len(executions)==1,'suite did not execute exactly once')
