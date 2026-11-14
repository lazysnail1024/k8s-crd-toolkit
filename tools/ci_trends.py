#!/usr/bin/env python3
"""Summarize comparable raw timings without claiming optimization acceptance."""
import argparse,json,math,pathlib,statistics
p=argparse.ArgumentParser();p.add_argument('samples',type=pathlib.Path);p.add_argument('--output',type=pathlib.Path,required=True);a=p.parse_args()
rows=json.loads(a.samples.read_text());groups={};pairs={}
if not isinstance(rows,list) or not rows:raise ValueError('nonempty timing samples required')
# Keep every dimension that can change preparation cost. Baseline and candidate
# are separate summaries, compared only by explicit matching pair IDs.
fields=('platform','toolchain','selection','fixture','resources','cacheState','resultType','phase')
for row in rows:
 if row.get('cacheState') not in ('warm','cold'):raise ValueError('explicit compilation cache state required')
 if row.get('variant') not in ('baseline','candidate'):raise ValueError('explicit baseline/candidate variant required')
 for field in ('platform','toolchain','selection','fixture','resources','runID','attempt','sourceCommit','evidence'):
  if not isinstance(row.get(field),str) or not row[field]:raise ValueError('missing sample provenance: '+field)
 if type(row.get('passed')) is not bool:raise ValueError('explicit success/failure required')
 result='success' if row['passed'] else row.get('failureType')
 if not isinstance(result,str) or not result:raise ValueError('failure classification required')
 if not isinstance(row.get('phases'),dict) or not row['phases']:raise ValueError('nonempty phases required')
 for phase,value in row['phases'].items():
  if not isinstance(phase,str) or not phase or type(value) not in (int,float) or not math.isfinite(value) or value<0:raise ValueError('invalid timing sample')
  meta=row|dict(resultType=result,phase=phase);key=tuple(meta[k] for k in fields);variant=row['variant'];groups.setdefault((key,variant),[]).append(value)
  if row.get('pairID') is not None:
   if not isinstance(row['pairID'],str) or not row['pairID']:raise ValueError('invalid pair identity')
   pairs.setdefault(key,{}).setdefault(row['pairID'],[]).append((variant,value))
summary=[]
for (key,variant),values in sorted(groups.items()):
 paired=list(pairs.get(key,{}).values())
 valid=bool(paired) and all(len(v)==2 and {x[0] for x in v}=={'baseline','candidate'} for v in paired)
 alternating=valid and all(paired[n][0][0]!=paired[n-1][0][0] for n in range(1,len(paired)))
 complete=len(paired) if valid else 0
 # Unpaired or duplicate observations must not be hidden behind enough pairs.
 covered=valid and len(values)==complete
 state=key[fields.index('cacheState')];result=key[fields.index('resultType')]
 enough=covered and alternating and complete>=(5 if state=='warm' else 3) and result=='success'
 summary.append(dict(zip(fields,key))|dict(variant=variant,n=len(values),median=statistics.median(values),minimum=min(values),maximum=max(values),pairedSamples=complete,alternatingOrder=bool(alternating),sufficient=enough,sufficiencyScope='paired median and outlier inspection only'))
a.output.parent.mkdir(parents=True,exist_ok=True);a.output.write_text(json.dumps(dict(schemaVersion=1,samples=rows,summary=summary,firstFailures=[r for r in rows if r['passed'] is False],observationWindow='at most seven days; insufficient natural samples remain explicitly insufficient',productThresholdsChanged=False,optimizationAccepted=False,percentilePolicy='No p95 estimate: bounded paired samples support median and outlier inspection only; full matrix and product acceptance remain separate.'),indent=2))
