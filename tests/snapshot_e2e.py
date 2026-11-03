#!/usr/bin/env python3
"""Compare real snapshot CLI output under observed native ZIP creator defaults.

The Windows-default subprocess is an isolated environment experiment, not a
native Windows run. Native platform acceptance remains the CI matrix's job.
"""
import argparse,hashlib,json,pathlib,platform,subprocess,sys,time,zipfile
ROOT=pathlib.Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--output',type=pathlib.Path,default=ROOT/'artifacts/snapshot');args=p.parse_args()
out=args.output.resolve();out.mkdir(parents=True,exist_ok=True);run_id=str(time.time_ns())
report={'replay':sys.argv,'python':sys.version,'platform':platform.platform(),'steps':[],'assertions':{},'passed':False}
# Only reproduce the constructor default observed in Windows CI; run the real CLI.
windows_default='''import runpy,sys,zipfile
original=zipfile.ZipInfo.__init__
def init(self,*args,**kwargs):
 original(self,*args,**kwargs)
 self.create_system=0
zipfile.ZipInfo.__init__=init
sys.argv=['tools/snapshot.py','--output',sys.argv[1]]
runpy.run_path('tools/snapshot.py',run_name='__main__')
'''
try:
 manifests=[];archives=[]
 for name,command in [('native-default',[sys.executable,'tools/snapshot.py','--output']),('observed-windows-default',[sys.executable,'-c',windows_default])]:
  target=out/(name+'-'+run_id);argv=command+[str(target)]
  r=subprocess.run(argv,cwd=ROOT,text=True,capture_output=True,timeout=60)
  report['steps'].append({'argv':argv,'stdout':r.stdout,'stderr':r.stderr,'exitCode':r.returncode})
  if r.returncode:raise RuntimeError('snapshot CLI failed')
  m=json.loads((target/'manifest.json').read_text());manifests.append(m);archives.append(target/m['module']/'@v'/(m['version']+'.zip'))
 report['manifests']=manifests
 report['assertions']={'sameRuntimeFiles':manifests[0]['files']==manifests[1]['files'],'sameVersion':manifests[0]['version']==manifests[1]['version'],'sameSourceIdentity':manifests[0]['sourceSHA256']==manifests[1]['sourceSHA256'],'sameContractIdentity':manifests[0]['contractSHA256']==manifests[1]['contractSHA256'],'sameZIPBytes':archives[0].read_bytes()==archives[1].read_bytes()}
 with zipfile.ZipFile(out/'inputs.zip','w',compression=zipfile.ZIP_DEFLATED) as z:
  z.write(ROOT/'tools/snapshot.py','tools/snapshot.py');z.write(__file__,'tests/snapshot_e2e.py');z.write(archives[0],'runtime-module.zip')
 report['inputArchiveSHA256']=hashlib.sha256((out/'inputs.zip').read_bytes()).hexdigest();report['passed']=all(report['assertions'].values())
except Exception as e:report['error']=repr(e)
finally:
 text=json.dumps(report,indent=2)+'\n';(out/'report.json').write_text(text);(out/('report-'+run_id+'.json')).write_text(text)
print(json.dumps({'passed':report['passed'],'assertions':report['assertions'],'error':report.get('error')}));sys.exit(0 if report['passed'] else 1)
