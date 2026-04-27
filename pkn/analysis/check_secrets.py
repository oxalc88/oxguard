"""Detect credential leaks using detect-secrets.

Compares a fresh scan against .secrets.baseline.
Exits 1 if new secrets are found that aren't in the baseline.

Usage:
    uv run python tools/analysis/check_secrets.py

To create or update the baseline:
    uv run detect-secrets scan > .secrets.baseline
"""

import json
import subprocess
import sys
from pathlib import Path

BASELINE = Path(__file__).parent.parent.parent / ".secrets.baseline"
SCAN_PATHS = ["functions", "cdk", "scripts"]


def _scan() -> dict:  # type: ignore[type-arg]
    result = subprocess.run(
        ["detect-secrets", "scan", *SCAN_PATHS],
        capture_output=True,
        text=True,
        check=True,
    )
    return json.loads(result.stdout)  # type: ignore[no-any-return]


def main() -> int:
    if not BASELINE.exists():
        print("ERROR: .secrets.baseline not found.")
        print("Run: pkn secrets --init")
        return 1

    baseline: dict = json.loads(BASELINE.read_text())  # type: ignore[type-arg]
    current = _scan()

    baseline_results: dict = baseline.get("results", {})  # type: ignore[type-arg]
    current_results: dict = current.get("results", {})  # type: ignore[type-arg]

    new_secrets: dict = {}  # type: ignore[type-arg]
    for filepath, secrets in current_results.items():
        known_hashes = {s["hashed_secret"] for s in baseline_results.get(filepath, [])}
        new = [s for s in secrets if s["hashed_secret"] not in known_hashes]
        if new:
            new_secrets[filepath] = new

    if new_secrets:
        count = sum(len(v) for v in new_secrets.values())
        print(f"ERROR: {count} new potential secret(s) detected!")
        for filepath, secrets in new_secrets.items():
            for secret in secrets:
                line = secret.get("line_number", "?")
                kind = secret.get("type", "unknown")
                print(f"  {filepath}:{line} [{kind}]")
        print("\nIf false positive: uv run detect-secrets scan > .secrets.baseline")
        return 1

    print("detect-secrets: OK")
    return 0


if __name__ == "__main__":
    sys.exit(main())
