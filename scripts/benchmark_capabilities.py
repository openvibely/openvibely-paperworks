#!/usr/bin/env python3
"""Measure paperworks capability latency and LibreOffice launch counts.

The benchmark compares a pre-change CLI binary with the current binary, then
measures the current CLI against the installed LibreOffice discovery path. Each
sample starts a fresh paperworks process. The cold cohort is the first 30
processes after environment setup; the warm cohort is the next 30. OS filesystem
caches are not flushed, so "cold" refers to a fresh CLI process, not a cold
kernel page cache.
"""

from __future__ import annotations

import argparse
import json
import math
import os
from pathlib import Path
import statistics
import subprocess
import tempfile
import time
from typing import Any


def run_cli(binary: Path, args: list[str], env: dict[str, str]) -> dict[str, Any]:
    completed = subprocess.run(
        [str(binary), *args],
        check=False,
        capture_output=True,
        text=True,
        env=env,
    )
    if completed.returncode != 0:
        raise RuntimeError(
            f"{binary} {' '.join(args)} failed ({completed.returncode}): "
            f"{completed.stderr.strip()}"
        )
    try:
        return json.loads(completed.stdout)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"invalid JSON from {binary}: {completed.stdout!r}") from exc


def capability_report(binary: Path, args: list[str], env: dict[str, str]) -> dict[str, Any]:
    report = run_cli(binary, ["capabilities", *args], env)
    office = report.get("libreoffice")
    if not isinstance(office, dict):
        raise RuntimeError(f"capabilities report has no LibreOffice object: {report!r}")
    return office


def sample_cohort(
    binary: Path, args: list[str], env: dict[str, str], count: int
) -> list[float]:
    samples_ms: list[float] = []
    for _ in range(count):
        started = time.perf_counter_ns()
        capability_report(binary, args, env)
        samples_ms.append((time.perf_counter_ns() - started) / 1_000_000)
    return samples_ms


def summarize(samples_ms: list[float]) -> dict[str, float]:
    ordered = sorted(samples_ms)
    p95_index = math.ceil(0.95 * len(ordered)) - 1
    return {
        "samples": len(ordered),
        "median_ms": round(statistics.median(ordered), 3),
        "p95_ms_nearest_rank": round(ordered[p95_index], 3),
    }


def marker_count(marker: Path) -> int:
    if not marker.exists():
        return 0
    return len(marker.read_text(encoding="utf-8").splitlines())


def write_wrapper(path: Path) -> None:
    path.write_text(
        "#!/bin/sh\n"
        "printf 'launch\\n' >> \"$CAPABILITIES_LAUNCH_MARKER\"\n"
        "exec \"$CAPABILITIES_REAL_SOFFICE\" \"$@\"\n",
        encoding="utf-8",
    )
    path.chmod(0o700)


def controlled_env(root: Path, wrapper: Path, marker: Path, soffice: Path) -> dict[str, str]:
    env = os.environ.copy()
    env["DOCUMENT_SOFFICE"] = str(wrapper)
    env["CAPABILITIES_LAUNCH_MARKER"] = str(marker)
    env["CAPABILITIES_REAL_SOFFICE"] = str(soffice)
    env["HOME"] = str(root / "home")
    env["XDG_CACHE_HOME"] = str(root / "xdg-cache")
    Path(env["HOME"]).mkdir(parents=True, exist_ok=True)
    Path(env["XDG_CACHE_HOME"]).mkdir(parents=True, exist_ok=True)
    return env


def measure_comparison(
    before: Path, after: Path, soffice: Path, count: int
) -> dict[str, Any]:
    results: dict[str, Any] = {}
    with tempfile.TemporaryDirectory(prefix="paperworks-capabilities-bench-") as temporary:
        root = Path(temporary)
        wrapper = root / "soffice-wrapper"
        marker = root / "launches.log"
        write_wrapper(wrapper)
        env = controlled_env(root, wrapper, marker, soffice)

        before_office = capability_report(before, [], env)
        if before_office.get("available") is not True:
            raise RuntimeError(f"baseline did not find the wrapper: {before_office!r}")
        marker.write_text("", encoding="utf-8")
        before_cold = sample_cohort(before, [], env, count)
        before_warm = sample_cohort(before, [], env, count)
        before_launches = marker_count(marker)
        results["before_custom_soffice"] = {
            "cold_processes": summarize(before_cold),
            "warm_processes": summarize(before_warm),
            "measured_soffice_launches": before_launches,
        }

        marker.write_text("", encoding="utf-8")
        after_fast_office = capability_report(after, [], env)
        if after_fast_office.get("available") is not True:
            raise RuntimeError(f"current fast path did not find the wrapper: {after_fast_office!r}")
        if "version" in after_fast_office:
            raise RuntimeError(f"current fast path unexpectedly reports version: {after_fast_office!r}")
        after_cold = sample_cohort(after, [], env, count)
        after_warm = sample_cohort(after, [], env, count)
        after_launches = marker_count(marker)
        if after_launches != 0:
            raise RuntimeError(f"default capabilities launched soffice {after_launches} times")
        results["after_custom_soffice"] = {
            "cold_processes": summarize(after_cold),
            "warm_processes": summarize(after_warm),
            "measured_soffice_launches": after_launches,
        }

        marker.write_text("", encoding="utf-8")
        direct = subprocess.run(
            [str(soffice), "--version"], check=True, capture_output=True, text=True
        ).stdout.strip()
        exact_office = capability_report(after, ["--version"], env)
        exact_value = exact_office.get("version")
        if exact_value != direct:
            raise RuntimeError(
                f"exact version mismatch: capabilities={exact_value!r}, soffice={direct!r}"
            )
        priming_launches = marker_count(marker)
        if priming_launches != 1:
            raise RuntimeError(f"expected one exact-version cache priming launch, got {priming_launches}")
        warm_exact = sample_cohort(after, ["--version"], env, count)
        cached_launches = marker_count(marker) - priming_launches
        if cached_launches != 0:
            raise RuntimeError(f"cached exact-version calls launched soffice {cached_launches} times")
        results["after_cached_exact_version"] = {
            "warm_processes": summarize(warm_exact),
            "cache_priming_launches": priming_launches,
            "measured_cached_launches": cached_launches,
            "version_matches_soffice": True,
        }

    return results


def measure_discovery(after: Path, count: int) -> dict[str, Any]:
    env = os.environ.copy()
    env.pop("DOCUMENT_SOFFICE", None)
    office = capability_report(after, [], env)
    if office.get("available") is not True or not office.get("path"):
        raise RuntimeError(f"current local discovery did not find LibreOffice: {office!r}")
    cold = sample_cohort(after, [], env, count)
    warm = sample_cohort(after, [], env, count)
    return {
        "selected_path": office["path"],
        "cold_processes": summarize(cold),
        "warm_processes": summarize(warm),
        "launches": "default report omits version; no version subprocess is invoked",
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--before", required=True, type=Path, help="pre-change paperworks binary")
    parser.add_argument("--after", required=True, type=Path, help="current paperworks binary")
    parser.add_argument("--soffice", required=True, type=Path, help="installed soffice executable")
    parser.add_argument("--runs", type=int, default=30, help="samples per cold/warm cohort (default: 30)")
    args = parser.parse_args()

    if args.runs < 1:
        parser.error("--runs must be at least 1")
    for executable in (args.before, args.after, args.soffice):
        if not executable.is_file() or not os.access(executable, os.X_OK):
            parser.error(f"not an executable file: {executable}")

    direct_version = subprocess.run(
        [str(args.soffice), "--version"], check=True, capture_output=True, text=True
    ).stdout.strip()
    result = {
        "method": {
            "sample_definition": "fresh paperworks process per sample; first cohort is cold, second follows the first",
            "kernel_page_cache": "not flushed",
            "p95": "nearest-rank",
            "runs_per_cohort": args.runs,
            "soffice_version": direct_version,
        },
        "custom_soffice_comparison": measure_comparison(
            args.before.resolve(), args.after.resolve(), args.soffice.resolve(), args.runs
        ),
        "current_cached_discovery": measure_discovery(args.after.resolve(), args.runs),
    }
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
