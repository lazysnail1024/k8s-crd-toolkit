#!/usr/bin/env python3
"""Reproduce behavior ablations in unique temporary copies; never edit baseline."""
import pathlib,tempfile,shutil,subprocess,json,hashlib,sys
root=pathlib.Path(__file__).resolve().parents[1]
variants=[('source','source/snapshot.go','next <= s.revision','false','revision'),('edit','edit/plan.go','text != r.ExpectedText','text != text','expected'),('yaml','yamlview/view.go','len(s.Text()) > 5*1024*1024','false','bytes-limit'),('yaml-depth','yamlview/internal/parser/parser.go','depth > 256','false','depth-limit'),('resources','yamlview/internal/parser/parser.go','defer t.Close()','// tree release deliberately removed','resident-growth'),('budget','yamlview/internal/parser/grammar/scanner.c.inc','unsigned size = 0;\n    // Zig-zag','unsigned size = 0;\n    if (scanner->ind_typ_stk.size > 250) { toolkit_yaml_budget_exceeded = true; return 0; }\n    // Zig-zag','mapping-256-complete')]
variants.append(('retention','yamlview/internal/parser/parser.go','t = p.Parse(data, nil)','t = p.ParseWithOptions(func(i int, _ sitter.Point) []byte { if i >= len(data) { return nil }; return data[i:] }, nil, &sitter.ParseOptions{ProgressCallback: func(s sitter.ParseState) bool { return ctx.Err() != nil || yaml.Exceeded() }})','success-context-released'))
if len(sys.argv)>1: variants=[v for v in variants if v[0] in sys.argv[1:]]
for stage,file,old,new,assertion in variants:
 with tempfile.TemporaryDirectory(prefix='toolkit-ablation-') as d:
  target=pathlib.Path(d)/'toolkit';shutil.copytree(root,target,ignore=shutil.ignore_patterns('.git','artifacts'))
  p=target/file;before=p.read_text();assert old in before;p.write_text(before.replace(old,new,1))
  run=subprocess.run([sys.executable,str(root/'tests/e2e.py'),stage+'-ablation',str(target)],text=True,capture_output=True)
  path=root/'artifacts'/(stage+'-ablation')/'report.json';report=json.loads(path.read_text());report['ablation']={'replay':'python3 tests/ablate.py','file':file,'old':old,'new':new,'baselineSHA256':hashlib.sha256(before.encode()).hexdigest(),'variantSHA256':hashlib.sha256(p.read_bytes()).hexdigest(),'expectedFailedAssertion':assertion};path.write_text(json.dumps(report,ensure_ascii=False,indent=2))
  steps=report['steps'];observed=json.loads(steps[-1]['stdout']) if steps[-1]['exitCode']==1 and steps[-2]['exitCode']==0 else {}
  print(stage,observed);assert observed.get(assertion) is False
