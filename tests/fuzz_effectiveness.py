#!/usr/bin/env python3
"""Run known behavioral defects in isolated copies through the real fuzz engine."""
import json,os,pathlib,shutil,subprocess,sys,tempfile,hashlib,tarfile
ROOT=pathlib.Path(__file__).resolve().parents[1];out=ROOT/'artifacts/fuzz-effectiveness';out.mkdir(parents=True,exist_ok=True);rows=[]
summary=dict(schemaVersion=1,executionIdentity={k:os.getenv(k,'local') for k in ('GITHUB_SHA','GITHUB_RUN_ID','GITHUB_RUN_ATTEMPT')},runnerSHA256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),cases=rows,passed=False)
def save(): (out/'report.json').write_text(json.dumps(summary,indent=2))
save()
variants=[('edit','edit/plan.go','text != r.ExpectedText','text != r.ExpectedText && false','FuzzAtomicEdit','wrong source accepted'),('source','source/snapshot.go','if r > 0xffff {','if r > 0xffff && false {','FuzzCoordinates','position ')]
variants.append(('yamlview','yamlview/view.go','return n.raw','return ""','FuzzParseRecovery','known mapping changed'))
for package,file,old,new,target,reason in variants:
 row=dict(target=target,expectedFailure=reason,passed=False,sourceCommit=subprocess.run(['git','rev-parse','HEAD'],cwd=ROOT,text=True,capture_output=True).stdout.strip() or 'unavailable',mutation=dict(file=file,before=old,after=new),go=subprocess.check_output(['go','version'],text=True).strip());rows.append(row)
 try:
  with tempfile.TemporaryDirectory(prefix='toolkit-fuzz-defect-') as tmp:
   copy=pathlib.Path(tmp)/'source';shutil.copytree(ROOT,copy,ignore=shutil.ignore_patterns('.git','artifacts','__pycache__'))
   path=copy/file;text=path.read_text()
   if old not in text:raise ValueError('mutation target absent')
   path.write_text(text.replace(old,new,1))
   archive=out/(package+'-mutant.tar.gz')
   with tarfile.open(archive,'w:gz') as bundle:bundle.add(copy,arcname='source')
   row['replayArchive']=str(archive);row['replayArchiveSHA256']=hashlib.sha256(archive.read_bytes()).hexdigest()
   row['mutantFileSHA256']=hashlib.sha256(path.read_bytes()).hexdigest()
   cmd=['go','test','./'+package,'-run=^$','-fuzz=^'+target+'$','-fuzztime=5s','-fuzzminimizetime=5s','-parallel=2'];r=subprocess.run(cmd,cwd=copy,env=os.environ|{'GOWORK':'off','CGO_ENABLED':'1','GOMAXPROCS':'2'},text=True,capture_output=True,timeout=90)
   output=r.stdout+r.stderr
   compiled='fuzz:' in r.stdout and '[build failed]' not in output and 'declared and not used' not in output
   row.update(command=cmd,stdout=r.stdout,stderr=r.stderr,exitCode=r.returncode,buildSucceeded=compiled,passed=compiled and r.returncode==1 and reason in r.stdout)
   row['replayCommand']=['go','test','./'+package,'-run=^'+target+'$']
   replay=subprocess.run(row['replayCommand'],cwd=copy,env=os.environ|{'GOWORK':'off','CGO_ENABLED':'1'},text=True,capture_output=True,timeout=90)
   row['replay']=dict(stdout=replay.stdout,stderr=replay.stderr,exitCode=replay.returncode)
   row['passed']=row['passed'] and replay.returncode==1 and reason in replay.stdout
   corpus=copy/package/'testdata/fuzz'
   if corpus.exists():shutil.copytree(corpus,out/package,dirs_exist_ok=True)
 except Exception as e:row['error']=repr(e)
 save()
summary['passed']=len(rows)==3 and all(r['passed'] for r in rows);save();sys.exit(not summary['passed'])
