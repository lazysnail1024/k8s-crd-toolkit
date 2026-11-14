#!/usr/bin/env python3
"""External verifier controls only: no runtime sources or acceptance are changed."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'artifacts/receipt-negative-controls'
OUT.mkdir(parents=True, exist_ok=True)
VERIFIER = ROOT / 'tools/ci_receipt.py'
IDENTITY = {'GITHUB_SHA': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(), 'GITHUB_RUN_ID': '987654321', 'GITHUB_RUN_ATTEMPT': '2'}
ENV = os.environ | IDENTITY | {'PYTHONDONTWRITEBYTECODE': '1'}
REPORT_PATHS = {
    'index_regression': ['artifacts/index-race/report.json', 'artifacts/workspace-race/report.json', 'artifacts/ablations/report.json'],
    'delivery_regression': ['artifacts/runner-effectiveness/report.json', 'artifacts/snapshot/report.json', 'artifacts/checkout/report.json', 'artifacts/vendor-probe/report.json', 'artifacts/delivery/report.json', 'artifacts/native-delivery/report.json', 'artifacts/delivery-failures/report.json'],
    'properties': ['artifacts/properties/report.json'],
    'downstream': ['artifacts/downstream/report.json'],
}
rows = []

def exercise(case_id, lane, report, should_accept):
    work = OUT / case_id
    work.mkdir(parents=True, exist_ok=True)
    for name in REPORT_PATHS[lane]:
        path = work / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(report, indent=2))
    commands = [[sys.executable, str(VERIFIER), 'write', '--lane', lane, '--platform', 'linux/amd64'] + sum([['--report', x] for x in REPORT_PATHS[lane]], []) + ['--directory', 'receipts'],
                [sys.executable, str(VERIFIER), 'verify', '--directory', 'receipts', '--lanes', json.dumps([lane])]]
    steps = []
    for command in commands:
        process = subprocess.run(command, cwd=work, env=ENV, capture_output=True, text=True)
        steps.append({'command': command, 'cwd': str(work), 'stdout': process.stdout, 'stderr': process.stderr, 'exitCode': process.returncode})
    accepted = all(x['exitCode'] == 0 for x in steps)
    rows.append({'caseId': case_id, 'expected': 'accept' if should_accept else 'reject', 'accepted': accepted, 'controlPassed': accepted == should_accept, 'steps': steps})

minimal = {'schemaVersion': 1, 'passed': True, 'executionIdentity': IDENTITY}
exercise('index-no-cases', 'index_regression', minimal, False)
exercise('delivery-no-cases', 'delivery_regression', minimal, False)
exercise('index-failed-assertion', 'index_regression', minimal | {'assertions': {'required-behavior': False}}, False)
exercise('delivery-unknown-schema', 'delivery_regression', minimal | {'schemaVersion': 999, 'cases': [{'passed': False}]}, False)

properties = minimal | {'assertions': {x: True for x in json.loads((ROOT / 'tests/property-contract.json').read_text())}}
exercise('properties-valid', 'properties', properties, True)
exercise('properties-old-report', 'properties', properties | {'executionIdentity': IDENTITY | {'GITHUB_RUN_ATTEMPT': '1'}}, False)

pins = json.loads((ROOT / 'tests/downstream-pins.json').read_text())['consumers']
tuple_ = {'toolkit': IDENTITY['GITHUB_SHA'], **{k: v['commit'] for k, v in pins.items()}}
downstream = minimal | {
    'schemaVersion': 1, 'buildRole': 'downstream-candidate', 'tuple': tuple_,
    'snapshot': {'files': {}, 'sourceSHA256': 'no-candidate-files'},
    'consumers': [{'consumer': name, 'passed': True, 'tuple': tuple_, 'suiteReports': [{'suite': suite, 'files': {}} for suite in pin['suites']], 'compilerDependencies': [{}], 'vendorDigest': 'no-candidate-files'} for name, pin in pins.items()],
}
exercise('downstream-no-execution', 'downstream', downstream, False)

report = {'purpose': 'Verifier negative controls; no product ablation, build, or compatibility acceptance claimed', 'python': sys.version, 'identity': IDENTITY, 'verifierSHA256': hashlib.sha256(VERIFIER.read_bytes()).hexdigest(), 'replay': [sys.executable, str(Path(__file__).resolve())], 'cases': rows}
(OUT / 'report.json').write_text(json.dumps(report, indent=2))
for row in rows:
    print(row['caseId'], 'expected=' + row['expected'], 'actual=' + ('accept' if row['accepted'] else 'reject'))

sys.exit(0 if all(row["controlPassed"] for row in rows) else 1)
