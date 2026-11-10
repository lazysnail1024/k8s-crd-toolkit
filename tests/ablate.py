#!/usr/bin/env python3
"""Reproduce behavior ablations in unique temporary copies; never edit baseline."""
import pathlib,tempfile,shutil,subprocess,json,hashlib,sys
root=pathlib.Path(__file__).resolve().parents[1]
variants=[('source','source/snapshot.go','next <= s.revision','false','revision'),('edit','edit/plan.go','text != r.ExpectedText','text != text','expected'),('yaml','yamlview/view.go','len(s.Text()) > 5*1024*1024','false','bytes-limit'),('yaml-depth','yamlview/internal/parser/parser.go','depth > 256','false','depth-limit'),('resources','yamlview/internal/parser/parser.go','defer t.Close()','// tree release deliberately removed','resident-growth'),('budget','yamlview/internal/parser/grammar/scanner.c.inc','unsigned size = 0;\n    // Zig-zag','unsigned size = 0;\n    if (scanner->ind_typ_stk.size > 250) { toolkit_yaml_budget_exceeded = true; return 0; }\n    // Zig-zag','mapping-256-complete')]
variants.append(('retention','yamlview/internal/parser/parser.go','t = p.Parse(data, nil)','t = p.ParseWithOptions(func(i int, _ sitter.Point) []byte { if i >= len(data) { return nil }; return data[i:] }, nil, &sitter.ParseOptions{ProgressCallback: func(s sitter.ParseState) bool { return ctx.Err() != nil || yaml.Exceeded() }})','success-context-released'))
variants.extend([
 ('index-ticket','index/index.go','e == nil || e.serial != ticket.serial','e == nil','stale-ticket-cannot-commit'),
 ('index-missing','index/index.go','i.affected(&stored{scope: e.scope, facts: facts}, affected)','// New-definition dependency propagation deliberately removed.','new-target-wakes-missing-caller'),
 ('index-position','index/facts.go','s.Span, s.Selection = source.Span{}, source.Span{}','// Position normalization deliberately removed.','location-only-not-semantic'),
 ('index-fresh','index/query.go','if e.serial != 0 {','if false {','pending-definitions-not-published'),
 ('index-retention','index/facts.go','s.Name, s.Kind = strings.Clone(s.Name), strings.Clone(s.Kind)','s.Name, s.Kind = s.Name, strings.Clone(s.Kind)','summaries-do-not-retain-source-storage'),
 ('workspace-overlay','workspace/documents.go','return ReadTicket{}, ErrOverlay','return ReadTicket{}, nil','overlay-prevents-disk-read'),
 ('workspace-ticket','workspace/documents.go',' || entry.read != ticket.read','', 'superseded-read-rejected'),
 ('workspace-discard','workspace/documents.go','if ticket.owner != d || !exists || entry.open != nil || ticket.read == 0 || entry.read != ticket.read {\n\t\treturn false','if ticket.owner != d || !exists || entry.open != nil || ticket.read == 0 {\n\t\treturn false','cancelled-old-read-keeps-new-ticket'),
 ('workspace-coalescing','workspace/queue.go','if element, exists := q.pending[job.URI]; exists {','if element, exists := q.pending[job.URI]; exists && false {','queue-coalesces-uri'),
 ('workspace-fairness','workspace/queue.go','var turns = [...]Priority{Current, Current, Current, Current, Current, Current, Current, Current, Related, Related, Related, Related, Background}','var turns = [...]Priority{Current, Related}','background-progress-under-continuous-edits'),
 ('workspace-priority','workspace/scanner.go','if s.active != nil && !s.active.work.priority && s.urgent() {','if false {','current-directory-served-before-bulk'),
 ('workspace-complete','workspace/scanner.go','batch.Complete = batch.Done && !s.incomplete && !s.closed','batch.Complete = batch.Done && !s.closed','directory-limit-not-false-complete'),
 ('workspace-filter','workspace/scanner.go','if len(s.extensions) != 0 && !s.extensions[strings.ToLower(filepath.Ext(entry.Name()))] {','if false {','scanner-candidates-exact-once'),
])
if len(sys.argv)>1: variants=[v for v in variants if v[0] in sys.argv[1:]]
if not variants:raise SystemExit('no matching ablation variants')
for stage,file,old,new,assertion in variants:
 with tempfile.TemporaryDirectory(prefix='toolkit-ablation-') as d:
  target=pathlib.Path(d)/'toolkit';shutil.copytree(root,target,ignore=shutil.ignore_patterns('.git','artifacts'))
  p=target/file;before=p.read_text();assert old in before;p.write_text(before.replace(old,new,1))
  run=subprocess.run([sys.executable,str(root/'tests/e2e.py'),stage+'-ablation',str(target)],text=True,capture_output=True)
  path=root/'artifacts'/(stage+'-ablation')/'report.json';report=json.loads(path.read_text());report['ablation']={'replay':'python3 tests/ablate.py','file':file,'old':old,'new':new,'baselineSHA256':hashlib.sha256(before.encode()).hexdigest(),'variantSHA256':hashlib.sha256(p.read_bytes()).hexdigest(),'expectedFailedAssertion':assertion};path.write_text(json.dumps(report,ensure_ascii=False,indent=2))
  steps=report['steps'];observed=json.loads(steps[-1]['stdout']) if steps[-1]['exitCode']==1 and steps[-2]['exitCode']==0 else {}
  print(stage,observed);assert observed.get(assertion) is False
