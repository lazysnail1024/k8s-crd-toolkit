#!/usr/bin/env python3
"""Run the five named property faults through the real public consumer."""
import pathlib,subprocess,json,shutil,sys,os,hashlib
root=pathlib.Path(__file__).resolve().parents[1];out=root/'artifacts/property-effectiveness';out.mkdir(parents=True,exist_ok=False)
variants=[('source','source/snapshot.go','return New(s.uri, next, text)','return New(s.uri, next, text + "x")','property-0-change-0'),('cache','index/index.go','e.serial = 0\n\ti.attach(e.scope, e.uri, false)\n\treturn Change{Generation:', 'e.serial = 0\n\treturn Change{Generation:', 'property-0-warm-cold'),('scope','index/query.go','i.definitions[lookup{scope, key}]','i.definitions[lookup{"b", key}]','property-0-scope-isolation'),('delete','index/index.go','func (i *Index) Remove(uri source.URI) (Change, error) {','func (i *Index) Remove(uri source.URI) (Change, error) {\n return Change{}, nil','property-0-delete-index'),('overlay','workspace/documents.go','if d.entries[uri].open == nil {','if true {','property-0-delete-overlay')]
rows=[]
summary=dict(schemaVersion=1,executionIdentity={k:os.getenv(k,'local') for k in ('GITHUB_SHA','GITHUB_RUN_ID','GITHUB_RUN_ATTEMPT')},sourceCommit=subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip(),runnerSHA256=hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),cases=rows,passed=False)
def save(): (out/'report.json').write_text(json.dumps(summary,indent=2))
save()
for name,file,before,after,oracle in [('baseline','','','','')]+variants:
 target=root
 if name!='baseline':
  target=out/name;shutil.copytree(root,target,ignore=shutil.ignore_patterns('.git','artifacts','__pycache__'),dirs_exist_ok=True);p=target/file;s=p.read_text();assert before in s;p.write_text(s.replace(before,after,1))
 cmd=[sys.executable,str(root/'tests/e2e.py'),'properties',str(target),'--output',str(out/(name+'-evidence'))];p=subprocess.run(cmd,text=True,capture_output=True);report=json.loads((out/(name+'-evidence')/'report.json').read_text());passed=report['passed'] if name=='baseline' else report.get('buildSucceeded') is True and report.get('consumerExitCode')==1 and report['assertions'].get(oracle) is False
 rows.append(dict(name=name,mutation=dict(file=file,before=before,after=after),oracle=oracle,command=cmd,stdout=p.stdout,stderr=p.stderr,exitCode=p.returncode,passed=passed,evidence=str(out/(name+'-evidence')/'report.json')));save()

summary["passed"]=len(rows)==6 and all(x["passed"] for x in rows);save();sys.exit(not summary["passed"])
