#!/usr/bin/env python3
"""Compare real snapshot CLI output under observed native ZIP creator and path ordering defaults.

The Windows-default subprocess is an isolated environment experiment, not a
native Windows run. Native platform acceptance remains the CI matrix's job.
"""
import argparse,hashlib,json,pathlib,platform,subprocess,sys,time,zipfile,zlib
ROOT=pathlib.Path(__file__).resolve().parents[1]
p=argparse.ArgumentParser();p.add_argument('--output',type=pathlib.Path,default=ROOT/'artifacts/snapshot');args=p.parse_args()
out=args.output.resolve();out.mkdir(parents=True,exist_ok=True);run_id=str(time.time_ns())
report={'replay':sys.argv,'python':sys.version,'platform':platform.platform(),'zlibCompileVersion':zlib.ZLIB_VERSION,'zlibRuntimeVersion':zlib.ZLIB_RUNTIME_VERSION,'steps':[],'assertions':{},'passed':False}
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
# Use Python's Windows path comparison without replacing filesystem access.
windows_order='''import pathlib,runpy,sys
path_type=type(pathlib.Path())
path_type.__lt__=lambda self,other: pathlib.PureWindowsPath(self.as_posix()) < pathlib.PureWindowsPath(other.as_posix())
sys.argv=['tools/snapshot.py','--output',sys.argv[1]]
runpy.run_path('tools/snapshot.py',run_name='__main__')
'''
# A valid alternate deflate strategy probes compressor dependence, not a
# specific Windows zlib implementation. Stored archives must be unaffected.
alternate_deflate='''import runpy,sys,zlib
original=zlib.compressobj
def compress(*args,**kwargs):
 kwargs['strategy']=zlib.Z_FIXED
 return original(*args,**kwargs)
zlib.compressobj=compress
sys.argv=['tools/snapshot.py','--output',sys.argv[1]]
runpy.run_path('tools/snapshot.py',run_name='__main__')
'''
try:
 manifests=[];archives=[]
 for name,command in [('native-default',[sys.executable,'tools/snapshot.py','--output']),('observed-windows-default',[sys.executable,'-c',windows_default]),('observed-windows-order',[sys.executable,'-c',windows_order]),('alternate-deflate',[sys.executable,'-c',alternate_deflate])]:
  target=out/(name+'-'+run_id);argv=command+[str(target)]
  r=subprocess.run(argv,cwd=ROOT,text=True,capture_output=True,timeout=60)
  report['steps'].append({'argv':argv,'stdout':r.stdout,'stderr':r.stderr,'exitCode':r.returncode})
  if r.returncode:raise RuntimeError('snapshot CLI failed')
  m=json.loads((target/'manifest.json').read_text());manifests.append(m);archives.append(target/m['module']/'@v'/(m['version']+'.zip'))
 report['manifests']=manifests
 report['archiveEntries']=[]
 for archive in archives:
  with zipfile.ZipFile(archive) as z:report['archiveEntries'].append(z.namelist())
 report['assertions']={label:all(m[key]==manifests[0][key] for m in manifests[1:]) for label,key in [('sameRuntimeFiles','files'),('sameVersion','version'),('sameSourceIdentity','sourceSHA256'),('sameContractIdentity','contractSHA256')]}
 report['assertions']['sameZIPBytes']=all(a.read_bytes()==archives[0].read_bytes() for a in archives[1:])
 with zipfile.ZipFile(out/'inputs.zip','w',compression=zipfile.ZIP_DEFLATED) as z:
  z.write(ROOT/'tools/snapshot.py','tools/snapshot.py');z.write(__file__,'tests/snapshot_e2e.py');z.write(archives[0],'runtime-module.zip')
 report['inputArchiveSHA256']=hashlib.sha256((out/'inputs.zip').read_bytes()).hexdigest();report['passed']=all(report['assertions'].values())
except Exception as e:report['error']=repr(e)
finally:
 text=json.dumps(report,indent=2)+'\n';(out/'report.json').write_text(text);(out/('report-'+run_id+'.json')).write_text(text)
print(json.dumps({'passed':report['passed'],'assertions':report['assertions'],'error':report.get('error')}));sys.exit(0 if report['passed'] else 1)
