#!/usr/bin/env python3
"""Exercise receipt portability with Windows path spelling and real local I/O.

The path-model subprocess is a verifier environment control, not native Windows
product acceptance. Actual Windows consumer execution remains the hosted matrix.
"""
import hashlib,json,os,pathlib,subprocess,sys,tempfile
ROOT=pathlib.Path(__file__).resolve().parents[1];OUT=ROOT/'artifacts/receipt-portability';OUT.mkdir(parents=True,exist_ok=True);rows=[]
model='''import argparse,pathlib,runpy,sys
sys.path.insert(0,str(pathlib.Path(sys.argv[1]).parent))
class WindowsFile(pathlib.PureWindowsPath):
 def read_text(self,*a,**kw):return pathlib.Path(self.as_posix()).read_text(*a,**kw)
 def read_bytes(self):return pathlib.Path(self.as_posix()).read_bytes()
original=argparse.ArgumentParser.add_argument
def add(self,*args,**kw):
 if '--report' in args:kw['type']=WindowsFile
 return original(self,*args,**kw)
argparse.ArgumentParser.add_argument=add
sys.argv=sys.argv[1:]
runpy.run_path(sys.argv[0],run_name='__main__')
'''
seed_model='''import pathlib,runpy,sys
class WindowsFile(pathlib.PureWindowsPath):
 def relative_to(self,other):return super().relative_to(pathlib.PureWindowsPath(str(other)))
 def read_bytes(self):return pathlib.Path(self.as_posix()).read_bytes()
original=pathlib.Path.glob
def glob(self,pattern,*args,**kwargs):
 paths=original(self,pattern,*args,**kwargs)
 return (WindowsFile(p.as_posix()) for p in paths) if pattern=='*/*_fuzz_test.go' else paths
pathlib.Path.glob=glob
sys.argv=sys.argv[1:]
runpy.run_path(sys.argv[0],run_name='__main__')
'''
with tempfile.TemporaryDirectory(prefix='toolkit-receipt-portability-') as tmp:
 tmp=pathlib.Path(tmp);identity={'GITHUB_SHA':'a'*40,'GITHUB_RUN_ID':'17','GITHUB_RUN_ATTEMPT':'2'};env=os.environ|identity
 fixture={'schemaVersion':1,'executionIdentity':identity,'passed':True,'assertions':{name:True for name in json.loads((ROOT/'tests/property-contract.json').read_text())}}
 path=tmp/'artifacts/properties/report.json';path.parent.mkdir(parents=True);path.write_text(json.dumps(fixture))
 for name,prefix in [('native-spelling',[sys.executable]),('windows-spelling',[sys.executable,'-c',model])]:
  directory=tmp/name;cmd=prefix+[str(ROOT/'tools/ci_receipt.py'),'write','--lane','properties','--platform','linux/amd64','--report','artifacts/properties/report.json','--directory',str(directory)];r=subprocess.run(cmd,cwd=tmp,env=env,text=True,capture_output=True);checks={'writerSucceeded':r.returncode==0}
  if r.returncode==0:
   receipt=json.loads(next(directory.glob('*.json')).read_text());checks['canonicalReportPaths']=[x['path'] for x in receipt['reports']]==['artifacts/properties/report.json']
  rows.append(dict(caseId=name,command=cmd,stdout=r.stdout,stderr=r.stderr,exitCode=r.returncode,checks=checks,passed=all(checks.values())))
 seed_out=tmp/'modeled-seeds';cmd=[sys.executable,'-c',seed_model,str(ROOT/'tests/fuzz_seeds.py'),'--platform','linux/amd64','--output',str(seed_out)];r=subprocess.run(cmd,cwd=ROOT,text=True,capture_output=True);value=json.loads((seed_out/'report.json').read_text());expected={p.relative_to(ROOT).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in ROOT.glob('*/*_fuzz_test.go')};checks={'seedReplaySucceeded':r.returncode==0,'canonicalSourcePaths':value.get('inputHashes')==expected};rows.append(dict(caseId='windows-source-paths',command=cmd,stdout=r.stdout,stderr=r.stderr,exitCode=r.returncode,checks=checks,passed=all(checks.values())))
report=dict(schemaVersion=1,cases=rows,passed=all(x['passed'] for x in rows));(OUT/'report.json').write_text(json.dumps(report,indent=2));print(json.dumps({'passed':report['passed'],'failed':[x['caseId'] for x in rows if not x['passed']]}));sys.exit(not report['passed'])
