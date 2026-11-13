#!/usr/bin/env python3
"""Exercise runner contracts with controlled processes, not product claims."""
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import time

ROOT = pathlib.Path(__file__).resolve().parents[1]
OUT = ROOT / 'artifacts/runner-effectiveness'
OUT.mkdir(parents=True, exist_ok=True)
report = {'runID': str(time.time_ns()), 'cases': [], 'passed': False}


def invoke(name, command, cwd, expected, env=None):
    child = subprocess.run(command, cwd=cwd, env=env, text=True,
                           capture_output=True, timeout=180)
    row = {'case': name, 'argv': command, 'exitCode': child.returncode,
           'stdout': child.stdout, 'stderr': child.stderr,
           'passed': expected(child)}
    report['cases'].append(row)
    return child


# This test-only process implements the consumer-runner protocol so each
# ablation failure has one independent cause. Real source mutations are tested
# separately by ablate.py, against compiled public consumers.
PROTOCOL_RUNNER = '''import json,pathlib,sys
stage,target=sys.argv[1:3]
out=pathlib.Path(sys.argv[sys.argv.index('--output')+1]);out.mkdir(parents=True,exist_ok=True)
mode=pathlib.Path(__file__).with_name('protocol-mode').read_text()
baseline=stage.endswith('-baseline')
if not baseline and mode=='missing-report': sys.exit(1)
if not baseline and mode=='malformed-report':
 (out/'report.json').write_text('{broken');sys.exit(1)
values={'revision':baseline}
if not baseline and mode=='unrelated-assertion':values={'revision':True,'unrelated':False}
passed=baseline and mode!='baseline-failure'
code=0 if passed else 1
if not baseline and mode=='success-exit':code=0
result={'stage':stage,'target':str(pathlib.Path(target).resolve()),'passed':passed,
        'buildSucceeded':mode!='build-failure','consumerExitCode':1 if not baseline else 0,
        'steps':[{'exitCode':0},{'exitCode':1 if not baseline else 0}],
        'assertions':values}
if not baseline and mode=='setup-failure':result['steps'][0]['exitCode']=127
if not baseline and mode=='wrong-target':result['target']='unrelated-checkout'
(out/'report.json').write_text(json.dumps(result));sys.exit(code)
'''

try:
    # Compile a small controlled program through the real e2e runner. These
    # payloads validate result parsing and exit status, not toolkit semantics.
    for name, payload, status, should_pass in [
        ('boolean-success', '{"probe":true}', 0, True),
        ('empty-object', '{}', 0, False),
        ('non-object', '[true]', 0, False),
        ('truthy-string', '{"probe":"false"}', 0, False),
        ('truthy-number', '{"probe":1}', 0, False),
        ('duplicate-key', '{"probe":false,"probe":true}', 0, False),
        ('false-with-success-exit', '{"probe":false}', 0, False),
        ('true-with-failure-exit', '{"probe":true}', 1, False),
    ]:
        with tempfile.TemporaryDirectory(prefix='toolkit-runner-protocol-') as temporary:
            fixture = pathlib.Path(temporary)
            tests = fixture / 'tests'
            consumer = tests / 'consumer'
            consumer.mkdir(parents=True)
            for file in ['e2e.py', 'assertions.py']:
                shutil.copyfile(ROOT / 'tests' / file, tests / file)
            shutil.copyfile(ROOT / 'tests/consumer/go.mod', consumer / 'go.mod')
            source = 'package main\nimport ("fmt"; "os")\nfunc main(){fmt.Println(' + json.dumps(payload) + ');os.Exit(' + str(status) + ')}\n'
            (consumer / 'main.go').write_text(source)
            output = OUT / name
            def expected(child):
                receipt = json.loads((output / 'report.json').read_text())
                return (child.returncode == 0) == should_pass and receipt.get('passed') is should_pass and receipt.get('buildSucceeded') is True
            invoke(name, [sys.executable, str(tests / 'e2e.py'), 'source',
                          '--output', str(output)], fixture, expected)

    # A previous successful mutation receipt must not survive a missing tool.
    with tempfile.TemporaryDirectory(prefix='toolkit-stale-report-') as temporary:
        fixture = pathlib.Path(temporary) / 'repo'
        shutil.copytree(ROOT, fixture, ignore=shutil.ignore_patterns('.git', 'artifacts', '__pycache__'))
        old = fixture / 'artifacts/source-ablation/report.json'
        old.parent.mkdir(parents=True)
        old.write_text(json.dumps({'previousRun': 'sentinel', 'steps': [
            {'exitCode': 0, 'stdout': ''},
            {'exitCode': 1, 'stdout': '{"revision":false}'}]}))
        env = os.environ.copy()
        env['GO'] = str(fixture / 'missing-go')
        def rejected(child):
            receipt = json.loads((fixture / 'artifacts/ablations/report.json').read_text())
            return child.returncode != 0 and receipt.get('passed') is False and 'baseline failed' in receipt.get('error', '')
        invoke('stale-report-missing-tool', [sys.executable, 'tests/ablate.py', 'source'], fixture, rejected, env)
        shutil.copytree(fixture / 'artifacts', OUT / ('stale-' + report['runID']))

    # Hold the expected source mutation and failure assertion constant while
    # varying exactly one runner condition per row.
    for mode in ['named-behavior-failure', 'baseline-failure', 'missing-report',
                 'malformed-report', 'unrelated-assertion', 'success-exit',
                 'build-failure', 'setup-failure', 'wrong-target']:
        with tempfile.TemporaryDirectory(prefix='toolkit-ablation-protocol-') as temporary:
            fixture = pathlib.Path(temporary)
            (fixture / 'tests').mkdir()
            (fixture / 'source').mkdir()
            shutil.copyfile(ROOT / 'tests/ablate.py', fixture / 'tests/ablate.py')
            (fixture / 'tests/e2e.py').write_text(PROTOCOL_RUNNER)
            (fixture / 'tests/protocol-mode').write_text(mode)
            (fixture / 'source/snapshot.go').write_text('next <= s.revision\n')
            def expected(child):
                receipt = json.loads((fixture / 'artifacts/ablations/report.json').read_text())
                wanted = mode == 'named-behavior-failure'
                return (child.returncode == 0) == wanted and receipt.get('passed') is wanted
            invoke('ablation-' + mode, [sys.executable, 'tests/ablate.py', 'source'], fixture, expected)
            shutil.copytree(fixture / 'artifacts', OUT / (mode + '-' + report['runID']))
            if mode == 'named-behavior-failure':
                for suffix in [['source', 'typo'], ['source', 'source']]:
                    invoke('selection-' + '-'.join(suffix),
                           [sys.executable, 'tests/ablate.py'] + suffix, fixture,
                           lambda child: child.returncode != 0 and ('unknown' in child.stderr or 'duplicate' in child.stderr))

    # Execute the actual assertion accumulator with repeated fixture checks.
    # This uses its unmodified source definition, but fixed false/true inputs.
    source = (ROOT / 'tests/consumer/main.go').read_text()
    function = source[source.index('func check('):source.index('\nfunc main()')]
    with tempfile.TemporaryDirectory(prefix='toolkit-assertion-accumulator-') as temporary:
        fixture = pathlib.Path(temporary)
        (fixture / 'main.go').write_text('package main\nimport "os"\nvar results=map[string]bool{}\n' + function + '\nfunc main(){check("repeat",false);check("repeat",true);if results["repeat"]{os.Exit(1)}}\n')
        go = os.environ.get('GO') or (os.environ['GOROOT'] + '/bin/go' if os.environ.get('GOROOT') else shutil.which('go'))
        invoke('failure-cannot-be-overwritten', [go, 'run', str(fixture / 'main.go')], fixture,
               lambda child: child.returncode == 0, os.environ | {'GOWORK': 'off'})

        # Build each real observational probe once. Invalid parameter rows
        # must fail inside that executable, rather than at compile time.
        for probe in ['index-query', 'index-scope']:
            binary = fixture / (probe + ('.exe' if os.name == 'nt' else ''))
            child = invoke('build-' + probe, [go, 'build', '-o', str(binary),
                           './tests/probes/' + probe], ROOT, lambda child: child.returncode == 0,
                           os.environ | {'GOWORK': 'off'})
            if child.returncode:
                raise RuntimeError('probe did not build: ' + probe)
        for probe, arguments, valid in [
            ('index-query', ['4', '2', '3'], True),
            ('index-query', ['4', '8', '1'], True),
            ('index-query', ['0', '2', '1'], False),
            ('index-query', ['4', '0', '1'], False),
            ('index-query', ['4', '2', '0'], False),
            ('index-query', ['invalid', '2', '1'], False),
            ('index-query', [], False),
            ('index-scope', ['4'], True),
            ('index-scope', ['0'], False),
            ('index-scope', [], False),
        ]:
            binary = fixture / (probe + ('.exe' if os.name == 'nt' else ''))
            def expected(child):
                if not valid:
                    return child.returncode != 0 and ('positive' in child.stderr or 'usage:' in child.stderr)
                result = json.loads(child.stdout)
                return child.returncode == 0 and (result.get('correct') is True if probe == 'index-query'
                    else result.get('files') == 4 and result.get('stateQueries') == 100000)
            invoke(probe + '-' + '-'.join(arguments), [str(binary)] + arguments, ROOT, expected)

    report['passed'] = len(report['cases']) == 33 and all(row['passed'] for row in report['cases'])
except Exception as error:
    report['error'] = repr(error)
finally:
    report['testSHA256'] = hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest()
    (OUT / 'report.json').write_text(json.dumps(report, indent=2))
print(json.dumps({'passed': report['passed'], 'cases': len(report['cases']), 'error': report.get('error')}))
sys.exit(0 if report['passed'] else 1)
