"""Validate campaign execution against actual Go fuzz output and fixed inputs."""
import hashlib,pathlib,re
TARGETS={'source':'FuzzCoordinates','edit':'FuzzAtomicEdit','yamlview':'FuzzParseRecovery'}
def hashes(root):
 paths=list(root.glob('*/*_fuzz_test.go'))+list(root.glob('*/testdata/fuzz/**/*'))
 return {p.relative_to(root).as_posix():hashlib.sha256(p.read_bytes()).hexdigest() for p in paths if p.is_file()}
def count(output):
 rows=re.findall(r'^fuzz: elapsed: [^\n]*?execs: (\d+)[^\n]*$',output,re.M)
 if not rows or not re.search(r'^fuzz: elapsed: .*now fuzzing with 2 workers$',output,re.M):raise ValueError('no actual two-worker mutation phase')
 values=[int(x) for x in rows]
 if values[-1]<=0 or values!=sorted(values):raise ValueError('invalid fuzz execution counts')
 return values[-1]
def verify(report,identity,root,platform,event):
 if report.get('sourceCommit')!=identity['GITHUB_SHA'] or report.get('buildRole')!='fuzz' or not re.fullmatch(r'go version go\d+\.\d+[^ ]* [\w]+/[\w]+',report.get('go','')):raise ValueError('fuzz source/role/toolchain identity mismatch')
 required_go=re.search(r'^go (\S+)$',(root/'go.mod').read_text(),re.M).group(1)
 if platform!='linux/amd64' or report['go']!='go version go'+required_go+' '+platform:raise ValueError('fuzz toolchain version/platform differs from required build identity')
 required_seconds=600 if event=='schedule' else 60
 if report.get('runID')!=identity['GITHUB_RUN_ID'] or report.get('attempt')!=identity['GITHUB_RUN_ATTEMPT']:raise ValueError('fuzz run identity mismatch')
 if report.get('testHashes')!=hashes(root):raise ValueError('fuzz target/corpus identity mismatch')
 rows=report.get('campaigns',[])
 if len(rows)!=3 or {r.get('target') for r in rows}!=set(TARGETS.values()):raise ValueError('fuzz target inventory mismatch')
 if len({r.get('budgetSeconds') for r in rows})!=1:raise ValueError('inconsistent fuzz budgets')
 for package,target in TARGETS.items():
  row=next(r for r in rows if r['target']==target);seconds=row.get('budgetSeconds')
  if seconds!=required_seconds:raise ValueError('fuzz budget differs from production event requirement: '+str(required_seconds)+' seconds')
  expected=['go','test','./'+package,'-run=^$','-fuzz=^'+target+'$','-fuzztime='+str(seconds)+'s','-parallel=2','-fuzzminimizetime=30s']
  if row.get('command')!=expected:raise ValueError('fuzz argv does not describe required mutation campaign')
  if row.get('executionIdentity')!=identity or row.get('package')!=package:raise ValueError('fuzz campaign identity mismatch')
  for stream in ('stdout','stderr'):
   raw=row.get(stream)
   if not isinstance(raw,str) or hashlib.sha256(raw.encode()).hexdigest()!=row.get(stream+'SHA256'):raise ValueError('missing or changed raw fuzz '+stream)
  executions=count(row['stdout'])
  stamps=re.findall(r'^fuzz: elapsed: (?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s), execs:',row['stdout'],re.M)
  if not stamps or sum(int(n or 0)*factor for n,factor in zip(stamps[-1],(3600,60,1)))<seconds:raise ValueError('raw fuzz output does not substantiate budget')
  if type(row.get('executions')) is not int or row['executions']!=executions:raise ValueError('fuzz execution count contradicts output')
  if row.get('exitCode')!=0 or row.get('passed') is not True or row.get('mutationObserved') is not True or not re.search(r'^PASS$',row['stdout'],re.M):raise ValueError('fuzz execution failed')
  if row.get('elapsedSeconds',0)<seconds:raise ValueError('fuzz campaign did not run required budget')
