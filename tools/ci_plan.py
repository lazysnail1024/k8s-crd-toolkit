#!/usr/bin/env python3
"""Conservative execution planning and fail-closed same-workflow aggregation."""
import argparse,hashlib,json,pathlib,sys
LANES=['consumer','index_regression','delivery_regression','properties','fuzz','downstream']
DOCS={'README.md','DEVELOPMENT.md','RELEASE_NOTES.md'}
def digest(value):return hashlib.sha256(json.dumps(value,sort_keys=True,separators=(',',':')).encode()).hexdigest()
def validate(p,sha):
 if p.get('planVersion')!=1 or p.get('testedSHA')!=sha:raise ValueError('plan schema or tested SHA mismatch')
 selected=p['selectedLanes'];excluded=p['excludedLanes']
 if len(set(selected))!=len(selected) or set(selected)&set(excluded) or set(selected)|set(excluded)!=set(LANES):raise ValueError('lane inventory mismatch')
 if excluded and (selected or p.get('reason')!='allowlisted-prose' or not p['changedPaths'] or not set(p['changedPaths'])<=DOCS):raise ValueError('invalid exclusion')
 if p['changedPathsDigest']!=digest(p['changedPaths']):raise ValueError('path digest mismatch')
def main():
 p=argparse.ArgumentParser();p.add_argument('command',choices=['plan','gate']);p.add_argument('--paths',type=pathlib.Path);p.add_argument('--output',type=pathlib.Path);p.add_argument('--plan',type=pathlib.Path);p.add_argument('--results',type=pathlib.Path);p.add_argument('--sha',required=True);p.add_argument('--base',default='unknown');p.add_argument('--head');p.add_argument('--full',action='store_true');a=p.parse_args()
 if len(a.sha)!=40 or any(c not in '0123456789abcdef' for c in a.sha):raise ValueError('full tested SHA required')
 if a.command=='plan':
  try:
   paths=json.loads(a.paths.read_text());assert isinstance(paths,list) and all(isinstance(x,str) for x in paths)
  except Exception:paths=['<unavailable-diff>']
  prose=bool(paths) and set(paths)<=DOCS and not a.full
  value=dict(planVersion=1,baseSHA=a.base,headSHA=a.head or a.sha,testedSHA=a.sha,changedPaths=paths,changedPathsDigest=digest(paths),selectedLanes=[] if prose else LANES,excludedLanes={x:'allowlisted-prose' for x in LANES} if prose else {},reason='allowlisted-prose' if prose else 'conservative-full')
  validate(value,a.sha);a.output.parent.mkdir(parents=True,exist_ok=True);a.output.write_text(json.dumps(value,indent=2))
 else:
  value=json.loads(a.plan.read_text());validate(value,a.sha);results=json.loads(a.results.read_text())
  if set(results)!=set(LANES):raise ValueError('missing or unexpected job')
  for lane in LANES:
   result=results[lane];result=result.get('result') if isinstance(result,dict) else result
   if result!=('success' if lane in value['selectedLanes'] else 'skipped'):raise ValueError(lane+': '+str(result))
  print('required plan complete')
if __name__=='__main__':
 try:main()
 except Exception as e:print(str(e),file=sys.stderr);sys.exit(1)
