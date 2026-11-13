#!/usr/bin/env python3
"""Reproduce behavior ablations in unique temporary copies; never edit baseline."""
import pathlib,tempfile,shutil,subprocess,json,hashlib,sys,time
root=pathlib.Path(__file__).resolve().parents[1]
variants=[('source','source/snapshot.go','next <= s.revision','false','revision'),('edit','edit/plan.go','text != r.ExpectedText','text != text','expected'),('yaml','yamlview/view.go','len(s.Text()) > 5*1024*1024','false','bytes-limit'),('yaml-depth','yamlview/internal/parser/parser.go','depth > 256','false','depth-limit'),('resources','yamlview/internal/parser/parser.go','defer value.tree.Close()','// tree release deliberately removed','resident-growth'),('budget','yamlview/internal/parser/grammar/scanner.c.inc','unsigned size = 0;\n    // Zig-zag','unsigned size = 0;\n    if (scanner->ind_typ_stk.size > 250) { toolkit_yaml_budget_exceeded = true; return 0; }\n    // Zig-zag','mapping-256-complete')]
variants.append(('retention','yamlview/internal/parser/parser.go','tree := call.parser.Parse(call.data, nil)','tree := call.parser.ParseWithOptions(func(i int, _ sitter.Point) []byte { if i >= len(call.data) { return nil }; return call.data[i:] }, nil, &sitter.ParseOptions{ProgressCallback: func(s sitter.ParseState) bool { return call.ctx.Err() != nil || yaml.Exceeded() }})','success-context-released'))
variants.extend([
 ('index-prefix','index/query.go','!strings.HasPrefix(string(s.Key), string(prefix))','false','prefix-before-limit'),
 ('index-query-budget','index/query_context.go','if used == budget {','if false {','query-no-match-budget'),
 ('index-query-files','index/query_context.go','used++ // A pending or symbol-free file still costs one visit.','// File visits deliberately uncharged.','query-pending-files-cost-work'),
 ('index-query-scope','index/query_context.go','valid := i.symbolScopeUnchanged(scope, result.State.Generation)','valid := true','query-final-scope-change-rejected'),
 ('schemaquery-validation','schemaquery/query.go','valid := s.Validate(value) == nil','valid := true','schemaquery-leaf-validation'),
 ('yamlvalue-origin','yamlvalue/project.go','copy.Kind = "derived"','copy.Kind = "exact"','derived-use-location'),
 ('yamlvalue-comment-input','yamlvalue/project.go','raw, err := documentText(snapshot, span)','span = doc.Span()\n\traw, err := documentText(snapshot, span)','trailing-comments-project-allocation'),
 ('yamlvalue-comment-characters','yamlview/view.go',"text[i] < ' ' || text[i] > '~'",'false','comment-unicode-break-0085'),
 ('yamlvalue-comment-tabs','yamlview/view.go',"strings.IndexByte(d.raw, '\\t') >= 0",'false','comment-invalid-tab-gap-blank'),
 ('index-ticket','index/index.go','e == nil || e.serial != ticket.serial','e == nil','stale-ticket-cannot-commit'),
 ('index-reuse','index/index.go','if e.data == nil || e.data.scope != e.scope {\n\t\treturn Change{}, ErrNoPrevious','if e.data == nil {\n\t\treturn Change{}, ErrNoPrevious','reuse-rejects-different-scope'),
 ('index-missing','index/index.go','i.affected(&stored{scope: e.scope, facts: facts}, affected)','// New-definition dependency propagation deliberately removed.','new-target-wakes-missing-caller'),
 ('index-position','index/facts.go','s.Span, s.Selection = source.Span{}, source.Span{}','// Position normalization deliberately removed.','location-only-not-semantic'),
 ('index-fresh','index/query.go','if e.serial != 0 {','if false {','pending-definitions-not-published'),
 ('index-retention','index/facts.go','s.Name, s.Kind = strings.Clone(s.Name), strings.Clone(s.Kind)','s.Name, s.Kind = s.Name, strings.Clone(s.Kind)','summaries-do-not-retain-source-storage'),
 ('workspace-overlay','workspace/documents.go','return ReadTicket{}, ErrOverlay','return ReadTicket{}, nil','overlay-prevents-disk-read'),
 ('workspace-ticket','workspace/documents.go',' || entry.read != ticket.read','', 'superseded-read-rejected'),
 ('workspace-discard','workspace/documents.go','if ticket.owner != d || !exists || entry.open != nil || ticket.read == 0 || entry.read != ticket.read {\n\t\treturn false','if ticket.owner != d || !exists || entry.open != nil || ticket.read == 0 {\n\t\treturn false','cancelled-old-read-keeps-new-ticket'),
 ('workspace-coalescing','workspace/queue.go','if element, exists := q.pending[job.URI]; exists {','if element, exists := q.pending[job.URI]; exists && false {','queue-coalesces-uri'),
 ('workspace-fairness','workspace/queue.go','var turns = [...]Priority{Current, Current, Current, Current, Current, Current, Current, Current, Related, Related, Related, Related, Background}','var turns = [...]Priority{Current, Related}','background-progress-under-continuous-edits'),
 ('workspace-workqueue-fairness','workspace/workqueue.go','return q.ready[p].Len() != 0 || q.overflow[p].Len() != 0','return q.ready[p].Len() != 0','workqueue-small-ready-fairness-1'),
 ('workspace-workqueue-followup','workspace/workqueue.go','if e.waiting {\n\t\tq.place(e, false, false)','if false {\n\t\tq.place(e, false, false)','workqueue-running-update-survives-completion'),
 ('workspace-membership-complete','workspace/membership.go','!pass.state.active && pass.state.complete &&','!pass.state.active &&','membership-failed-pass-cannot-delete'),
 ('workspace-priority','workspace/scanner.go','if s.active != nil && !s.active.work.priority && s.urgent() {','if false {','current-directory-served-before-bulk'),
 ('workspace-complete','workspace/scanner.go','batch.Complete = batch.Done && !s.incomplete && !s.closed','batch.Complete = batch.Done && !s.closed','directory-limit-not-false-complete'),
 ('workspace-filter','workspace/scanner.go','if len(s.extensions) != 0 && !s.extensions[strings.ToLower(filepath.Ext(entry.Name()))] {','if false {','scanner-candidates-exact-once'),
])
requested=sys.argv[1:]
known={v[0] for v in variants}
if set(requested)-known:raise SystemExit('unknown ablation variants: '+', '.join(sorted(set(requested)-known)))
if len(requested)!=len(set(requested)):raise SystemExit('duplicate ablation variants')
if requested:variants=[v for v in variants if v[0] in requested]
run_id=str(time.time_ns());out=root/'artifacts/ablations'/run_id;out.mkdir(parents=True)
summary={'runID':run_id,'replay':sys.argv,'cases':[],'passed':False}
baselines={}

def execute(stage,target,destination):
 argv=[sys.executable,str(root/'tests/e2e.py'),stage,str(target),'--output',str(destination)]
 child=subprocess.run(argv,text=True,capture_output=True,timeout=900)
 receipt=destination/'report.json'
 if not receipt.is_file():raise RuntimeError('this attempt produced no report: '+repr(argv)+'\n'+child.stderr)
 observed=json.loads(receipt.read_text())
 if observed.get('target')!=str(target.resolve()) or observed.get('stage')!=stage:raise RuntimeError('report does not belong to the requested consumer')
 return child,observed

try:
 for stage,file,old,new,assertion in variants:
  row={'variant':stage,'expectedFailedAssertion':assertion,'passed':False};summary['cases'].append(row)
  suite=stage.split('-')[0]
  if suite not in baselines:
   child,baseline=execute(suite+'-ablation-baseline',root,out/('baseline-'+suite))
   if child.returncode!=0 or baseline.get('passed') is not True:raise RuntimeError('baseline failed for '+suite)
   baselines[suite]=baseline
  if baselines[suite]['assertions'].get(assertion) is not True:raise RuntimeError('baseline never passed the named assertion: '+assertion)
  with tempfile.TemporaryDirectory(prefix='toolkit-ablation-') as directory:
   target=pathlib.Path(directory)/'toolkit';shutil.copytree(root,target,ignore=shutil.ignore_patterns('.git','artifacts','__pycache__'))
   path=target/file;before=path.read_text()
   if old not in before:raise RuntimeError('mutation no longer matches source: '+stage)
   path.write_text(before.replace(old,new,1))
   destination=out/stage
   child,observed=execute(stage+'-ablation',target,destination)
   row.update(file=file,old=old,new=new,baselineSHA256=hashlib.sha256(before.encode()).hexdigest(),variantSHA256=hashlib.sha256(path.read_bytes()).hexdigest(),report=str(destination/'report.json'),baselineReport=str(out/('baseline-'+suite)/'report.json'))
   row['passed']=child.returncode==1 and observed.get('passed') is False and 'error' not in observed and observed.get('buildSucceeded') is True and observed.get('consumerExitCode')==1 and observed.get('assertions',{}).get(assertion) is False and all(s['exitCode']==0 for s in observed['steps'][:-1])
   if not row['passed']:raise RuntimeError('mutation was not killed by the named behavior assertion: '+stage)
   print(stage,assertion,'killed',flush=True)
 summary['passed']=len(summary['cases'])==len(variants) and all(row['passed'] for row in summary['cases'])
except Exception as error:summary['error']=repr(error)
finally:
 text=json.dumps(summary,indent=2);(out/'report.json').write_text(text);(out.parent/'report.json').write_text(text)
print(json.dumps({'passed':summary['passed'],'variants':len(summary['cases']),'report':str(out/'report.json'),'error':summary.get('error')}))
sys.exit(0 if summary['passed'] else 1)
