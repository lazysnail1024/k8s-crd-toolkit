#!/usr/bin/env python3
"""Exercise native delivery and deliberate failures in isolated source copies."""
import argparse,hashlib,json,os,pathlib,shutil,subprocess,sys,tempfile,time,traceback
root=pathlib.Path(__file__).resolve().parents[1]
out=root/'artifacts/native-delivery';out.mkdir(parents=True,exist_ok=True)
variants=['isolated-source','empty-suite','rss-failure','wrong-platform','missing-compiler']
p=argparse.ArgumentParser();p.add_argument('--variant',action='append',choices=variants);p.add_argument('--timeout',type=float,default=300);args=p.parse_args()
run_id=str(time.time_ns());rows=[];report={'runID':run_id,'replay':sys.argv,'cases':rows,'passed':False}
# Preserve any previous report before replacing it, including failures before a case starts.
if (out/'report.json').exists():shutil.copyfile(out/'report.json',out/('report-previous-'+run_id+'.json'))
def save():
 text=json.dumps(report,indent=2)
 temporary=out/('report-'+run_id+'.tmp');temporary.write_text(text);temporary.replace(out/'report.json')
 (out/('report-'+run_id+'.json')).write_text(text)
def decoded(value):return value.decode(errors='replace') if isinstance(value,bytes) else (value or '')
save()
try:
 for variant in args.variant or variants:
  row={'variant':variant,'passed':False};rows.append(row)
  try:
   with tempfile.TemporaryDirectory(prefix='toolkit-clean-') as d:
    repo=pathlib.Path(d)/'checkout';repo.mkdir()
    for name in ['go.mod','go.sum','LICENSE','CONTRACT.md']:shutil.copyfile(root/name,repo/name)
    for name in ['source','edit','yamlview','yamlvalue','schemaquery','index','workspace','tests','tools']:shutil.copytree(root/name,repo/name,ignore=shutil.ignore_patterns('__pycache__'))
    script=repo/'tests/native.py';original=script.read_bytes()
    if variant=='empty-suite':
     text=script.read_text();old="for stage in ['source','edit','yaml','yamlvalue','schemaquery','audit','budget','retention','resume','resources','index','workspace']:";assert old in text;script.write_text(text.replace(old,'for stage in []:'))
    if variant=='rss-failure':
     f=repo/'tests/consumer/rss_linux.go';text=f.read_text();old='func rss() int64 {';assert old in text
     f.write_text(text.replace(old,'var rssCalls int\nfunc rss() int64 {\n rssCalls++\n if rssCalls == 2 { return -1 }'))
    expected='windows/amd64' if variant=='wrong-platform' else 'linux/amd64'
    env=os.environ.copy()
    if variant=='missing-compiler':env['CC']='definitely-not-a-C-compiler'
    target=out/(variant+'-'+run_id);target.mkdir()
    shutil.make_archive(str(target/'source-inputs'),'zip',repo)
    row.update(originalScriptSHA256=hashlib.sha256(original).hexdigest(),executedScriptSHA256=hashlib.sha256(script.read_bytes()).hexdigest(),inputArchiveSHA256=hashlib.sha256((target/'source-inputs.zip').read_bytes()).hexdigest(),artifact=str(target))
    argv=[sys.executable,str(script),'--expected-platform',expected,'--output',str(target)];row['argv']=argv
    child=subprocess.Popen(argv,cwd=d,env=env,text=True,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
    try:stdout,stderr=child.communicate(timeout=args.timeout)
    except subprocess.TimeoutExpired:
     child.kill();stdout,stderr=child.communicate();row['timedOut']=True
    row.update(stdout=decoded(stdout),stderr=decoded(stderr),exitCode=child.returncode)
    if row.get('timedOut'):raise TimeoutError('child exceeded '+str(args.timeout)+' seconds')
    observed=json.loads((target/'report.json').read_text())
    assertions=observed.get('assertions',{})
    passed=(child.returncode==0 and observed.get('passed') is True) if variant=='isolated-source' else (child.returncode!=0 and observed.get('passed') is False)
    if variant=='wrong-platform':
     passed=passed and assertions.get('nativeHost') is False and bool(observed.get('goEnvironment')) and not observed.get('stageResults') and 'native platform mismatch' in observed.get('error','')
    if variant=='missing-compiler':
     passed=passed and assertions.get('nativeHost') is True and assertions.get('goVersion') is True and not observed.get('stageResults')
     passed=passed and any(s['argv']==['definitely-not-a-C-compiler','--version'] and s['exitCode']==127 for s in observed.get('steps',[]))
    if variant in ('empty-suite','rss-failure'):
     passed=passed and 'error' not in observed and assertions.get('noReplace') is True and any('build' in s['argv'] and s['exitCode']==0 for s in observed.get('steps',[]))
    if variant=='empty-suite':passed=passed and assertions.get('requiredNonemptySuites') is False
    if variant=='rss-failure':
     passed=passed and observed.get('stageResults',{}).get('resources',{}).get('resident-growth') is False
     passed=passed and any(s['argv'][-1]=='resources' and s['exitCode']!=0 and -1 in json.loads(s['stderr'])['rssBytes'] for s in observed.get('steps',[]))
    row['passed']=bool(passed)
  except Exception as error:row.update(error=repr(error),traceback=traceback.format_exc())
  finally:save()
 report['passed']=bool(rows) and all(r['passed'] for r in rows)
except Exception as error:report.update(error=repr(error),traceback=traceback.format_exc())
finally:save()
print(json.dumps(report));sys.exit(0 if report['passed'] else 1)
