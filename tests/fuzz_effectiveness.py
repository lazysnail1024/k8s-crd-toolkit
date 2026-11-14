#!/usr/bin/env python3
"""Run known behavioral defects in isolated copies through the real fuzz engine."""
import json,os,pathlib,shutil,subprocess,sys,tempfile
ROOT=pathlib.Path(__file__).resolve().parents[1];out=ROOT/'artifacts/fuzz-effectiveness';out.mkdir(parents=True,exist_ok=True);rows=[]
variants=[('edit','edit/plan.go','text != r.ExpectedText','text != r.ExpectedText && false','FuzzAtomicEdit','wrong source accepted'),('source','source/snapshot.go','if r > 0xffff {','if r > 0xffff && false {','FuzzCoordinates','position ')]
variants.append(('yamlview','yamlview/view.go','return n.raw','return ""','FuzzParseRecovery','known mapping changed'))
for package,file,old,new,target,reason in variants:
 row=dict(target=target,expectedFailure=reason,passed=False);rows.append(row)
 try:
  with tempfile.TemporaryDirectory(prefix='toolkit-fuzz-defect-') as tmp:
   copy=pathlib.Path(tmp)/'source';shutil.copytree(ROOT,copy,ignore=shutil.ignore_patterns('.git','artifacts','__pycache__'))
   path=copy/file;text=path.read_text()
   if old not in text:raise ValueError('mutation target absent')
   path.write_text(text.replace(old,new,1));cmd=['go','test','./'+package,'-run=^$','-fuzz=^'+target+'$','-fuzztime=5s','-fuzzminimizetime=5s','-parallel=2'];r=subprocess.run(cmd,cwd=copy,env=os.environ|{'GOWORK':'off','CGO_ENABLED':'1','GOMAXPROCS':'2'},text=True,capture_output=True,timeout=90)
   output=r.stdout+r.stderr
   compiled='fuzz:' in r.stdout and '[build failed]' not in output and 'declared and not used' not in output
   row.update(command=cmd,stdout=r.stdout,stderr=r.stderr,exitCode=r.returncode,buildSucceeded=compiled,passed=compiled and r.returncode==1 and reason in r.stdout)
   corpus=copy/package/'testdata/fuzz'
   if corpus.exists():shutil.copytree(corpus,out/package,dirs_exist_ok=True)
 except Exception as e:row['error']=repr(e)
report=dict(cases=rows,passed=all(r['passed'] for r in rows));(out/'report.json').write_text(json.dumps(report,indent=2));sys.exit(not report['passed'])
