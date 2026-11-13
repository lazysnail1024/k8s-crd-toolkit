#!/usr/bin/env python3
"""Consume the standard vendor tree plus its required native headers."""
import hashlib
import json
import os
import pathlib
import shutil
import subprocess
import sys
import tempfile
import time

from assertions import parse_assertions

root = pathlib.Path(__file__).resolve().parents[1]
env = os.environ | {'GOWORK': 'off', 'CGO_ENABLED': '1'}
go = env.get('GO') or (env['GOROOT'] + '/bin/go' if env.get('GOROOT') else shutil.which('go'))
out = root / 'artifacts/vendor-probe'
out.mkdir(parents=True, exist_ok=True)
report = {'replay': sys.argv, 'steps': [], 'restoredHashes': {}, 'stageResults': {}, 'passed': False}


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def run(argv, cwd):
    child = subprocess.run(argv, cwd=cwd, env=env, text=True, capture_output=True, timeout=120)
    report['steps'].append({'argv': argv, 'stdout': child.stdout,
                            'stderr': child.stderr, 'exit': child.returncode})
    if child.returncode:
        raise RuntimeError('command failed: ' + repr(argv))
    return child.stdout


try:
    if not go:
        raise RuntimeError('Go not found')
    with tempfile.TemporaryDirectory(prefix='toolkit-vendor-e2e-') as directory:
        consumer = pathlib.Path(directory) / 'consumer'
        shutil.copytree(root / 'tests/consumer', consumer)
        with (consumer / 'go.mod').open('a') as file:
            file.write('\nreplace github.com/lazysnail1024/k8s-crd-toolkit => ' + str(root) + '\n')
        run([go, 'version'], consumer)
        run([go, 'mod', 'tidy'], consumer)
        run([go, 'mod', 'vendor'], consumer)
        directories = {
            'github.com/tree-sitter/go-tree-sitter': ['include', 'src'],
            'github.com/lazysnail1024/k8s-crd-toolkit': ['yamlview/internal/parser/grammar/tree_sitter'],
        }
        for module, includes in directories.items():
            metadata = json.loads(run([go, 'list', '-mod=mod', '-m', '-json', module], consumer))
            for directory in includes:
                source = pathlib.Path(metadata['Dir']) / directory
                destination = consumer / 'vendor' / module / directory
                shutil.copytree(source, destination, dirs_exist_ok=True)
                for path in destination.rglob('*'):
                    if path.is_file():
                        report['restoredHashes'][str(path.relative_to(consumer))] = sha(path)
        binary = consumer / ('consumer.exe' if os.name == 'nt' else 'consumer')
        run([go, 'build', '-mod=vendor', '-trimpath', '-buildvcs=false', '-o', str(binary), '.'], consumer)
        report['binarySHA256'] = sha(binary)
        shutil.copy2(binary, out / ('consumer-' + report['binarySHA256'] + ('.exe' if os.name == 'nt' else '')))
        for stage in ['budget', 'yaml', 'audit']:
            report['stageResults'][stage] = parse_assertions(run([str(binary), stage], consumer))
        report['passed'] = set(report['stageResults']) == {'budget', 'yaml', 'audit'} and all(
            all(values.values()) for values in report['stageResults'].values())
except Exception as error:
    report['error'] = repr(error)
finally:
    report['testSHA256'] = sha(pathlib.Path(__file__))
    text = json.dumps(report, indent=2)
    (out / 'report.json').write_text(text)
    (out / ('report-' + str(time.time_ns()) + '.json')).write_text(text)
print(json.dumps({'passed': report['passed'], 'steps': len(report['steps']), 'error': report.get('error')}))
sys.exit(0 if report['passed'] else 1)
