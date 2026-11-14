#!/usr/bin/env python3
"""Verify exact lane coverage and original report bytes from this execution."""
import argparse,hashlib,json,os,pathlib,sys
ROOT=pathlib.Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'tests'))
from assertions import exact_stage_inventory
from ci_report_contracts import regression, downstream
PLATFORMS={'linux/amd64','linux/arm64','darwin/amd64','darwin/arm64','windows/amd64'}
REPORTS={
 'consumer':['artifacts/native/report.json','artifacts/fuzz-seeds/report.json'],
 'index_regression':['artifacts/index-race/report.json','artifacts/workspace-race/report.json','artifacts/ablations/report.json'],
 'delivery_regression':['artifacts/runner-effectiveness/report.json','artifacts/snapshot/report.json','artifacts/checkout/report.json','artifacts/vendor-probe/report.json','artifacts/delivery/report.json','artifacts/native-delivery/report.json','artifacts/delivery-failures/report.json'],
 **{lane:['artifacts/'+lane+'/report.json'] for lane in ('properties','fuzz','downstream')}}
def decode(text):
 def unique(pairs):
  result={}
  for key,value in pairs:
   if key in result:raise ValueError('duplicate JSON key')
   result[key]=value
  return result
 return json.loads(text,object_pairs_hook=unique,parse_constant=lambda v:(_ for _ in ()).throw(ValueError('nonfinite JSON '+v)))
def sha(path):return hashlib.sha256(path.read_bytes()).hexdigest()
def identity():return {k:os.environ[k] for k in ('GITHUB_SHA','GITHUB_RUN_ID','GITHUB_RUN_ATTEMPT')}
def check_report(report,lane,platform,path):
 if report.get('schemaVersion')!=1 or 'error' in report or report.get('passed') is not True or report.get('executionIdentity')!=identity():raise ValueError('failed, stale or unbound report')
 for field in ('assertions','stageResults','cases','campaigns','consumers'):
  if field in report and not report[field]:raise ValueError('empty '+field)
 if lane=='consumer' and path=='artifacts/fuzz-seeds/report.json':
  if report.get('buildRole')!='fuzz-seed-replay' or report.get('platform')!=platform:raise ValueError('seed replay identity mismatch')
  required={target+'/seed#'+str(i) for target,count in [('FuzzCoordinates',4),('FuzzAtomicEdit',2),('FuzzParseRecovery',3)] for i in range(count)}|{'FuzzCoordinates','FuzzAtomicEdit','FuzzParseRecovery'}
  if not required<=set(report.get('completed',[])) or len(report.get('steps',[]))!=1 or report['steps'][0].get('exitCode')!=0:raise ValueError('missing deterministic fuzz seeds')
  events=[decode(line) for line in report['steps'][0]['stdout'].splitlines()]
  actual=[x['Test'] for x in events if x.get('Action')=='pass' and 'Test' in x]
  if actual!=report['completed']:raise ValueError('seed result not bound to Go output')
  expected={f.relative_to(ROOT).as_posix():sha(f) for f in ROOT.glob('*/*_fuzz_test.go')}
  if report.get('inputHashes')!=expected:raise ValueError('seed test inputs mismatch')
 elif lane=='consumer':
  expected=json.loads((ROOT/'tests/native-contract.json').read_text())
  if report.get('schemaVersion')!=1 or report.get('buildRole')!='ordinary' or report.get('buildMode')!='public-consumer' or report.get('productVersion')!=(ROOT/'VERSION').read_text().strip():raise ValueError('native role/version mismatch')
  if report.get('sourceCommit')!=os.environ['GITHUB_SHA'] or report.get('expectedPlatform')!=platform:raise ValueError('native source/platform mismatch')
  if report.get('contractDigest')!=sha(ROOT/'CONTRACT.md') or report.get('dependencyDigest')!=sha(ROOT/'go.sum'):raise ValueError('native dependencies mismatch')
  if report['producer']['runID']!=os.environ['GITHUB_RUN_ID'] or report['producer']['attempt']!=os.environ['GITHUB_RUN_ATTEMPT']:raise ValueError('native attempt mismatch')
  if not exact_stage_inventory(report['stageResults'],expected,platform):raise ValueError('native case/assertion coverage mismatch')
 if lane=='properties':
  expected=json.loads((ROOT/'tests/property-contract.json').read_text())
  if set(report['assertions'])!=set(expected) or not all(report['assertions'].values()):raise ValueError('property inventory mismatch')
 if lane=='fuzz':
  campaigns=report.get('campaigns',[])
  if len(campaigns)!=3 or {x['target'] for x in campaigns}!={'FuzzCoordinates','FuzzAtomicEdit','FuzzParseRecovery'} or any(x.get('passed') is not True or x.get('mutationObserved') is not True or x.get('exitCode')!=0 for x in campaigns):raise ValueError('missing actual fuzz campaign')
 if lane in ('index_regression','delivery_regression'):regression(report,path)
 if lane=='downstream':downstream(report,identity())

def main():
 p=argparse.ArgumentParser();p.add_argument('command',choices=['write','verify']);p.add_argument('--lane');p.add_argument('--platform',default='linux/amd64');p.add_argument('--report',action='append',type=pathlib.Path);p.add_argument('--directory',type=pathlib.Path,required=True);p.add_argument('--lanes');a=p.parse_args()
 if a.command=='write':
  if [p.as_posix() for p in a.report or []]!=REPORTS.get(a.lane):raise ValueError('required report inventory mismatch')
  reports=[]
  for path in a.report:
   raw=path.read_text();value=decode(raw);check_report(value,a.lane,a.platform,path.as_posix())
   if a.lane=='consumer' and path.as_posix()=='artifacts/native/report.json':
    binary=path.parent/('consumer.exe' if a.platform.startswith('windows/') else 'consumer')
    if sha(binary)!=value['binarySHA256'] or sha(path.parent/'inputs.zip')!=value['inputArchiveSHA256']:raise ValueError('actual binary or inputs changed')
   if a.lane=='downstream':
    for consumer in value['consumers']:
     if sha(pathlib.Path(consumer['binaryPath']))!=consumer['binarySHA256']:raise ValueError('actual downstream binary changed')
   reports.append(dict(path=path.as_posix(),sha256=hashlib.sha256(raw.encode()).hexdigest(),rawJSON=raw))
  a.directory.mkdir(parents=True,exist_ok=True);receipt=dict(schemaVersion=1,identity=identity(),lane=a.lane,platform=a.platform,reports=reports)
  (a.directory/(a.lane+'-'+a.platform.replace('/','-')+'.json')).write_text(json.dumps(receipt,indent=2))
 else:
  lanes=set(json.loads(a.lanes));receipts=[decode(p.read_text()) for p in a.directory.rglob('*.json')];expected={(l,p) for l in lanes for p in (PLATFORMS if l=='consumer' else ['linux/amd64'])};seen=set()
  for r in receipts:
   key=(r.get('lane'),r.get('platform'))
   if r.get('schemaVersion')!=1 or r.get('identity')!=identity() or key not in expected or key in seen:raise ValueError('stale, duplicate or unexpected receipt')
   if [x['path'] for x in r['reports']]!=REPORTS[key[0]]:raise ValueError('missing or extra reports')
   for entry in r['reports']:
    if hashlib.sha256(entry['rawJSON'].encode()).hexdigest()!=entry['sha256']:raise ValueError('report byte digest mismatch')
    check_report(decode(entry['rawJSON']),*key,entry['path'])
   seen.add(key)
  if seen!=expected:raise ValueError('missing lane/platform receipt: '+repr(expected-seen))
  print('verified',len(receipts),'fresh lane receipts')
if __name__=='__main__':
 try:main()
 except Exception as e:print(str(e),file=sys.stderr);sys.exit(1)
