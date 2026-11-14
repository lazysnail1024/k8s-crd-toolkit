#!/usr/bin/env python3
"""Build and execute a public consumer, retaining this attempt even on failure."""
import argparse
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import time
import zipfile

from assertions import parse_assertions

import os
CI_IDENTITY = {k: os.getenv(k, 'local') for k in ('GITHUB_SHA', 'GITHUB_RUN_ID', 'GITHUB_RUN_ATTEMPT')}
root = pathlib.Path(__file__).resolve().parents[1]
parser = argparse.ArgumentParser()
parser.add_argument('stage')
parser.add_argument('target', nargs='?', type=pathlib.Path, default=root)
parser.add_argument('--output', type=pathlib.Path)
args = parser.parse_args()
target = args.target.resolve()
out = (args.output or root / 'artifacts' / args.stage).resolve()
out.mkdir(parents=True, exist_ok=True)
started = time.monotonic()
run_id = str(time.time_ns())
env = os.environ.copy()
env.update(GOWORK='off', CGO_ENABLED='1')
go = env.get('GO') or (env['GOROOT'] + '/bin/go' if env.get('GOROOT') else shutil.which('go'))
report = dict(schemaVersion=1,executionIdentity=CI_IDENTITY,runID=run_id, stage=args.stage, target=str(target), replay=sys.argv,
              steps=[], assertions={}, passed=False,
              environment={k: env.get(k) for k in
                           ['GO', 'GOROOT', 'GOMODCACHE', 'GOCACHE', 'GOWORK',
                            'CGO_ENABLED', 'GOFLAGS', 'GOTOOLCHAIN', 'CC']})


def save():
    report['elapsedSeconds'] = round(time.monotonic() - started, 6)
    text = json.dumps(report, ensure_ascii=False, indent=2)
    temporary = out / ('report-' + run_id + '.tmp')
    temporary.write_text(text)
    temporary.replace(out / 'report.json')
    (out / ('report-' + run_id + '.json')).write_text(text)


def run(command, cwd):
    start = time.monotonic()
    try:
        child = subprocess.run(command, cwd=cwd, env=env, text=True,
                               capture_output=True, timeout=120)
        step = dict(command=command, stdout=child.stdout, stderr=child.stderr,
                    exitCode=child.returncode)
    except Exception as error:
        step = dict(command=command, stdout=decoded(getattr(error, 'stdout', '')),
                    stderr=decoded(getattr(error, 'stderr', '')) + '\n' + repr(error),
                    exitCode=124 if isinstance(error, subprocess.TimeoutExpired) else 127)
    step['elapsedSeconds'] = round(time.monotonic() - start, 6)
    report['steps'].append(step)
    save()
    return step


def decoded(value):
    return value.decode(errors='replace') if isinstance(value, bytes) else (value or '')


def required(command, cwd):
    step = run(command, cwd)
    if step['exitCode']:
        raise RuntimeError('setup command failed: ' + repr(command))
    return step


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def input_file(path):
    return path.is_file() and not any(name in path.parts for name in
                                     ('.git', 'artifacts', '__pycache__'))


# Replace the latest receipt before tools, copying or compilation can fail.
save()
try:
    if not go:
        raise RuntimeError('Go not found')
    # Go does not track arbitrary #included .inc/header bytes when reusing a
    # CGO package. Bind those inputs to the compiler invocation so a warm
    # cache cannot turn a native-source mutation back into the baseline.
    native_files = {path.relative_to(target).as_posix(): sha(path)
                    for path in (target / 'yamlview').rglob('*')
                    if path.is_file() and path.suffix in ('.inc', '.h', '.c')}
    native_digest = hashlib.sha256(json.dumps(native_files, sort_keys=True).encode()).hexdigest()
    # Preserve the selected toolchain's effective defaults (normally -O2 -g),
    # including caller overrides; setting only the macro would disable them.
    effective_cflags = required([go, 'env', 'CGO_CFLAGS'], target)['stdout'].strip()
    env['CGO_CFLAGS'] = (effective_cflags +
                        ' -DTOOLKIT_NATIVE_INPUTS_SHA256=' + native_digest).strip()
    report['nativeInputSHA256'] = native_digest
    report['environment']['CGO_CFLAGS'] = env['CGO_CFLAGS']
    with tempfile.TemporaryDirectory() as directory:
        consumer = pathlib.Path(directory) / 'consumer'
        shutil.copytree(root / 'tests/consumer', consumer)
        with (consumer / 'go.mod').open('a') as file:
            file.write('\nreplace github.com/lazysnail1024/k8s-crd-toolkit => ' + str(target) + '\n')
        built = consumer / ('consumer.exe' if os.name == 'nt' else 'consumer')
        required([go, 'version'], consumer)
        required([env.get('CC') or 'cc', '--version'], consumer)
        required([go, 'mod', 'tidy'], consumer)
        required([go, 'build', '-trimpath', '-buildvcs=false', '-o', str(built), '.'], consumer)
        report['binarySHA256'] = sha(built)
        binary = out / ('consumer-' + report['binarySHA256'] + ('.exe' if os.name == 'nt' else ''))
        shutil.copy2(built, binary)
        report['retainedBinary'] = str(binary)
        report['buildSucceeded'] = True
        result = run([str(built), args.stage.split('-')[0]], consumer)
        report['consumerExitCode'] = result['exitCode']
        report['assertions'] = parse_assertions(result['stdout'])
        report['passed'] = result['exitCode'] == 0 and all(report['assertions'].values())
except Exception as error:
    report['error'] = repr(error)
finally:
    try:
        report['targetHashes'] = {str(p.relative_to(target)): sha(p)
                                  for p in target.rglob('*') if input_file(p)}
        report['hashes'] = {str(p.relative_to(root)): sha(p)
                            for p in root.rglob('*') if input_file(p)}
        archive = out / ('inputs-' + run_id + '.zip')
        with zipfile.ZipFile(archive, 'w', compression=zipfile.ZIP_DEFLATED) as bundle:
            for prefix, tree in [('target', target), ('tests', root / 'tests')]:
                for path in tree.rglob('*'):
                    if input_file(path):
                        bundle.write(path, prefix + '/' + path.relative_to(tree).as_posix())
        report['inputArchive'] = {'path': str(archive), 'sha256': sha(archive)}
    except Exception as error:
        report['error'] = repr(error)
        report['passed'] = False
    save()
print(json.dumps({'passed': report['passed'], 'assertions': report['assertions'],
                  'error': report.get('error'), 'report': str(out / 'report.json')}))
sys.exit(0 if report['passed'] else 1)
