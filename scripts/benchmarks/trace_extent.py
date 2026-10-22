#!/usr/bin/env python3
"""Time trace including OPC loading; compare complete JSON and input hashes.

Export fixtures with PAPERWORKS_EXTENT_FIXTURES=<existing-empty-dir> go test
./internal/office -run '^TestExportExtentBenchmark$' -count=1 first.
"""
import argparse
import hashlib
import json
import pathlib
import subprocess
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--before', required=True)
parser.add_argument('--after', required=True)
parser.add_argument('--fixtures', type=pathlib.Path, required=True)
parser.add_argument('--output', type=pathlib.Path, required=True)
parser.add_argument('--samples', type=int, default=5)
args = parser.parse_args()
results = []
for rows in (1000, 10000):
    for k in (1, 10, 100):
        path = args.fixtures / f'rows-{rows}-k-{k}.xlsx'
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        expected = None
        for sample in range(args.samples):
            # Alternate order to reduce systematic warm-cache/order bias.
            versions = ('before', 'after') if sample % 2 == 0 else ('after', 'before')
            for version in versions:
                start = time.perf_counter_ns()
                output = subprocess.check_output([
                    getattr(args, version), 'trace', str(path),
                    '--sheet', 'Output', '--cell', 'A1',
                    '--max-nodes', '200', '--max-depth', '20',
                ])
                elapsed = time.perf_counter_ns() - start
                report = json.loads(output)
                if expected is None:
                    expected = report
                assert report == expected, (rows, k, version, 'report changed')
                assert hashlib.sha256(path.read_bytes()).hexdigest() == digest, 'input changed'
                results.append(dict(rows=rows, k=k, sample=sample, version=version,
                                    ns=elapsed, input_sha256=digest))
                args.output.write_text(json.dumps(results, indent=2) + '\n')
                print(rows, k, sample, version, elapsed, flush=True)
