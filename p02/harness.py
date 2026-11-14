#!/usr/bin/env python3
"""Synthetic gate orchestration probe; never builds or executes a product."""
import argparse,copy,hashlib,json,os,pathlib,subprocess,sys,time
ROOT=pathlib.Path(__file__).resolve().parents[1]
SCENARIOS=['success','upstream-failure','upstream-cancel','unexpected-skip','missing-report','wrong-sha','manual-green']
LANES=['consumer','index_regression','delivery_regression','properties','fuzz','downstream']
def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def save(p,v):p.parent.mkdir(parents=True,exist_ok=True);p.write_text(json.dumps(v,indent=2))
def frozen():
 for name,digest in json.loads((ROOT/'p02/frozen-inputs.json').read_text()).items():
  if sha(ROOT/name)!=digest:raise ValueError('frozen verifier input changed: '+name)
def cli(out,label,script,*args):
 frozen();out.mkdir(parents=True,exist_ok=True);cmd=[sys.executable,str(ROOT/script),*map(str,args)];r=subprocess.run(cmd,cwd=ROOT,capture_output=True);(out/(label+'.stdout')).write_bytes(r.stdout);(out/(label+'.stderr')).write_bytes(r.stderr);save(out/(label+'.command.json'),{'command':cmd,'exitCode':r.returncode,'stdoutSHA256':hashlib.sha256(r.stdout).hexdigest(),'stderrSHA256':hashlib.sha256(r.stderr).hexdigest(),'scope':'Synthetic fixture verifier command; no product acceptance'});return r.returncode

def prepare(out,mode):
 frozen();scenario=json.loads((ROOT/'p02/scenario.json').read_text());assert scenario['schemaVersion']==1 and scenario['scenario'] in SCENARIOS
 if mode!='scenario' and os.environ.get('GITHUB_EVENT_NAME')!='workflow_dispatch':raise ValueError('override requires dispatch')
 if mode=='cancel-upstream' and not os.environ.get('P02_TARGET_PR','').isdigit():raise ValueError('numeric cancellation target required')
 effective=scenario['scenario'] if mode=='scenario' else 'success';save(out/'scenario.json',scenario|{'effectiveScenario':effective,'mode':mode,'scope':'Synthetic fixtures and real GitHub orchestration; zero product builds/tests','event':os.environ.get('GITHUB_EVENT_NAME')});save(out/'paths.json',['p02/scenario.json'])
 code=cli(out/'commands','plan','tools/ci_plan.py','plan','--paths',out/'paths.json','--output',out/'plan.json','--sha',os.environ['GITHUB_SHA'],'--base',os.environ.get('P02_BASE_SHA','unknown'),'--head',os.environ.get('P02_HEAD_SHA',os.environ['GITHUB_SHA']),'--full')
 if os.environ.get('GITHUB_OUTPUT'):
  with open(os.environ['GITHUB_OUTPUT'],'a') as f:f.write('scenario='+effective+'\n')
 return code

def fixtures(out):
 frozen();templates=json.loads((ROOT/'p02/fixture-templates.json').read_text());identity={k:os.environ[k] for k in ['GITHUB_SHA','GITHUB_RUN_ID','GITHUB_RUN_ATTEMPT']};config=json.loads((out/'scenario.json').read_text());scenario=config['effectiveScenario'];receipts=out/'receipts';receipts.mkdir(parents=True,exist_ok=True)
 for report in templates.values():report.update(schemaVersion=1,executionIdentity=identity,passed=True)
 properties={'schemaVersion':1,'executionIdentity':identity,'passed':True,'diagnosticFixture':True,'assertions':{k:True for k in json.loads((ROOT/'tests/property-contract.json').read_text())}};templates['artifacts/properties/report.json']=properties
 templates['artifacts/fuzz/report.json']={'schemaVersion':1,'executionIdentity':identity,'passed':True,'diagnosticFixture':True,'campaigns':[{'target':t,'passed':True,'mutationObserved':True,'exitCode':0} for t in ['FuzzCoordinates','FuzzAtomicEdit','FuzzParseRecovery']]}
 for path,value in templates.items():save(ROOT/path,value)
 native=templates['artifacts/native/report.json'];native.update(sourceCommit=identity['GITHUB_SHA'],producer={'runID':identity['GITHUB_RUN_ID'],'attempt':identity['GITHUB_RUN_ATTEMPT']},contractDigest=sha(ROOT/'CONTRACT.md'),dependencyDigest=sha(ROOT/'go.sum'))
 # Deliberately non-executable bytes: this check validates only receipt binding.
 for name in ['consumer','consumer.exe','inputs.zip']:(ROOT/'artifacts/native'/name).write_bytes(b'SYNTHETIC P02 FIXTURE: NOT AN EXECUTABLE OR PRODUCT ARCHIVE\n')
 native.update(binarySHA256=sha(ROOT/'artifacts/native/consumer'),inputArchiveSHA256=sha(ROOT/'artifacts/native/inputs.zip'))
 seeds=templates['artifacts/fuzz-seeds/report.json'];seeds['inputHashes']={p.relative_to(ROOT).as_posix():sha(p) for p in ROOT.glob('*/*_fuzz_test.go')}
 for platform in ['linux/amd64','linux/arm64','darwin/amd64','darwin/arm64','windows/amd64']:
  native['expectedPlatform']=platform;seeds['platform']=platform;save(ROOT/'artifacts/native/report.json',native);save(ROOT/'artifacts/fuzz-seeds/report.json',seeds)
  if cli(out/'commands','consumer-'+platform.replace('/','-'),'tools/ci_receipt.py','write','--lane','consumer','--platform',platform,'--report','artifacts/native/report.json','--report','artifacts/fuzz-seeds/report.json','--directory',receipts):return 1
 downstream=templates['artifacts/downstream/report.json'];tuple_=downstream['tuple']|{'toolkit':identity['GITHUB_SHA']};downstream['tuple']=tuple_
 for row in downstream['consumers']:
  row['tuple']=tuple_;old=row['binaryPath'];binary=out/'synthetic-binaries'/pathlib.Path(old).name;binary.parent.mkdir(exist_ok=True);binary.write_bytes(b'SYNTHETIC P02 FIXTURE: NOT EXECUTABLE\n');row['binaryPath']=str(binary);row['binarySHA256']=sha(binary)
  for step in row['steps']:step['command']=[str(binary) if x==old else x for x in step['command']]
  for suite in row['suiteReports']:
   for evidence in suite['evidence'].values():
    child=json.loads(evidence['rawJSON'])
    if isinstance(child,dict):
     for key in ['binarySHA256','binarySha256']:
      if key in child:child[key]=row['binarySHA256']
    oldhash=evidence['sha256'];evidence['rawJSON']=json.dumps(child);evidence['sha256']=hashlib.sha256(evidence['rawJSON'].encode()).hexdigest();suite['files']={k:evidence['sha256'] if v==oldhash else v for k,v in suite['files'].items()}
 save(ROOT/'artifacts/downstream/report.json',downstream)
 inventories={
 'index_regression':['index-race','workspace-race','ablations'],
 'delivery_regression':['runner-effectiveness','snapshot','checkout','vendor-probe','delivery','native-delivery','delivery-failures'],
 'properties':['properties'],'fuzz':['fuzz'],'downstream':['downstream']}
 for lane,paths in inventories.items():
  args=sum([['--report','artifacts/'+name+'/report.json'] for name in paths],[])
  if cli(out/'commands',lane,'tools/ci_receipt.py','write','--lane',lane,'--directory',receipts,*args):return 1
 first=receipts/'properties-linux-amd64.json';original=json.loads(first.read_text());save(out/'original-properties-receipt.json',original)
 if scenario=='missing-report':first.unlink()
 if scenario=='wrong-sha':save(first,original|{'identity':original['identity']|{'GITHUB_SHA':'0'*40}})
 save(out/'fixture-audit.json',{'diagnosticFixture':True,'scope':'Complete synthetic verifier fixtures, not actual source/platform/build/test evidence','fixtureTemplatesSHA256':sha(ROOT/'p02/fixture-templates.json'),'actualBuilds':0,'actualProductTests':0,'actualFuzzCampaigns':0,'consumerPlatforms':5,'lanes':LANES,'scenario':scenario})
 return 0

def finish(out):return 11 if json.loads((out/'scenario.json').read_text())['effectiveScenario'] in ['upstream-failure','manual-green'] else 0

def wait_cancel(out,seconds):
 save(out/'waiting-for-cancel.json',{'scope':'Real job-level concurrency cancellation control','runID':os.environ.get('GITHUB_RUN_ID'),'seconds':seconds});print('Ready for real upstream cancellation',flush=True);end=time.monotonic()+seconds
 while time.monotonic()<end:time.sleep(min(5,end-time.monotonic()))
 raise RuntimeError('Cancellation did not occur; elapsed wait never counts as cancelled')

def gate(inputs,out):
 frozen();raw=json.loads(os.environ['P02_NEEDS_JSON']);assert set(raw)=={'prepare','upstream'};save(out/'needs.raw.json',raw)
 # One explicitly synthetic upstream produces every fixture. Preserve the real
 # result for each selected production lane; this tests the frozen CLI truth
 # table, not execution of the production job graph or native consumers.
 projected={lane:raw['upstream'] for lane in LANES};save(out/'needs.control.json',projected)
 plan_exit=cli(out/'commands','plan-gate','tools/ci_plan.py','gate','--plan',inputs/'plan.json','--results',out/'needs.control.json','--sha',os.environ['GITHUB_SHA'])
 receipt_exit=cli(out/'commands','receipt-gate','tools/ci_receipt.py','verify','--directory',inputs/'receipts','--lanes',json.dumps(LANES))
 final=0 if plan_exit==receipt_exit==0 and raw['prepare'].get('result')=='success' else 1
 save(out/'diagnostic.json',{'scope':'Synthetic fixture validation and actual GitHub upstream state only; no product acceptance','checkName':'Toolkit required','actualUpstreamResult':raw['upstream'].get('result'),'planGateRawExitCode':plan_exit,'receiptGateRawExitCode':receipt_exit,'checkExitCode':final,'event':os.environ.get('GITHUB_EVENT_NAME'),'testedSHA':os.environ.get('GITHUB_SHA'),'runID':os.environ.get('GITHUB_RUN_ID'),'attempt':os.environ.get('GITHUB_RUN_ATTEMPT'),'manualEventNote':'Exact production check name retained for dispatch; no diagnostic-only event guard is added'})
 return final

def main():
 p=argparse.ArgumentParser();sub=p.add_subparsers(dest='action',required=True);s=sub.add_parser('prepare');s.add_argument('--mode',choices=['scenario','manual-green','cancel-upstream'],default='scenario');sub.add_parser('fixtures');sub.add_parser('finish');s=sub.add_parser('wait-cancel');s.add_argument('--seconds',type=int,default=600);s=sub.add_parser('gate');s.add_argument('--inputs',type=pathlib.Path,required=True)
 for s in sub.choices.values():s.add_argument('--output',type=pathlib.Path,required=True)
 a=p.parse_args();out=a.output.resolve()
 if a.action=='prepare':return prepare(out,a.mode)
 if a.action=='fixtures':return fixtures(out)
 if a.action=='finish':return finish(out)
 if a.action=='wait-cancel':return wait_cancel(out,a.seconds)
 return gate(a.inputs.resolve(),out)
if __name__=='__main__':raise SystemExit(main())
