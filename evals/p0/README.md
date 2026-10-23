# P0 workflow and simplicity baseline

This evaluation extension does not change either CLI, thresholds, bundled skills,
or existing expected answers. `cases.json` defines six questions per language.
The baseline source is commit `632eaf25f2c74dea0406e61a8bc8a8303b8055e6`
(`v0.9.0`). Native probes and fresh agent captures are separate evidence.

## Reproduce the native probes

Build both unchanged Go binaries. Install TypeScript tools from the exact pins
in `npm/toolchain.json` into a workspace-owned runtime containing the unchanged
`npm/tsguard/bin` and `config` directories. Save its generated package lock.
Create a dedicated uv environment and install Python dependencies with
`uv pip install --python <venv-python> --require-hashes -r evals/python/requirements.txt`.
Do not install Python dependencies globally.
Set `UV_PROJECT_ENVIRONMENT` to that same environment for fixtures containing a
`[project]` table; `VIRTUAL_ENV` alone does not select their uv project environment.

Run `node evals/p0/run.cjs <options.json>` with these options:

```json
{
  "fixtures": "/new/disposable/fixtures",
  "reports": "/evidence/native-p0",
  "runtime": {"root": "/installed/ts-runtime", "node": "/path/to/node"},
  "python": "/uv-environment/python",
  "binaries": {"typescript": "/baseline/tsguard", "python": "/baseline/pyguard"},
"env": {"UV_NO_SYNC": "1", "UV_OFFLINE": "1", "VIRTUAL_ENV": "/uv-environment", "UV_PROJECT_ENVIRONMENT": "/uv-environment"}
}
```

Also put the uv executable and environment scripts on PATH. Fixture roots must be
new so earlier edits cannot contaminate a later run. Native output and behavior
tests establish fixture preconditions, not agent judgment. Some cases intentionally
use a calibrated or low cap to isolate a boundary; reports disclose the cap and
must not present those probes as default-policy reproductions. Consolidation
uses measured scores, not the uploaded package's historical numbers.

Freeze the generated baseline fixtures and manifest hashes before comparison.
For candidate runs, copy those frozen fixtures and pass
`existingFixtures: {typescript: "/copied/typescript", python: "/copied/python"}`
in the options. Do not recalibrate a fixture against a candidate, change expected
answers to match it, or compare reports from different fixture/toolchain hashes.

## Fresh agent evidence

Use `prepare-captures.cjs` and `sessions.cjs` to copy each validated fixture into
its own project with an exact copy of the current language skill. Give a fresh
agent only the neutral task and observer interface, without scorer expectations
or previous attempts. Every source read, search, edit and command goes through
`observe.cjs`; record the final explanation with its `finish` operation.

The observer records source, tests and configuration, not just `src`. Generated
build/cache files are excluded. Drift between recorded events or changes to
protected reference skills invalidate the capture. Command output is preserved;
the observer never selects commands or supplies answers. `observe.test.cjs`
checks chronology, drift detection and real process execution.

`score.cjs` derives mechanical evidence. Architectural outcomes additionally need
review of actual reads, behavior verification and the diff; do not award a pass
from an agent's claim, file count or lower score. Preserve invalid attempts with
their reason, then rerun in a new fixture with a new history-free agent.

After reviewing traces and diffs, save judgments keyed by full case ID with
`outcome`, `reason` and the evidence used. Run `summarize.cjs <options.json>` with
`reviews`, `provenance`, `output`, and `languages` (each with `native`, `sessions`
and `manifest` paths). It combines native requirements with observed agent
judgments, rejects invalid captures, checks required context before edits, and
saves input, frozen fixture and evaluator hashes. Its exit code is 1 for a
failed requirement/invalid capture, 2 for pending judgment, and 0 only for passes.
An agent's honest incomplete report can pass its reporting review while the
native exhaustive-inventory requirement still fails the combined case.

One run per language/case is an exploratory baseline, not a failure-rate estimate.
Before comparing a candidate, use the same validated fixtures, rubric, model
configuration and toolchain, with repeated paired runs. Existing fail-fast tests
continue protecting `check`; a future `diagnose` command needs separate expected
behavior. Do not rewrite existing oracles merely to make a baseline green.

## Existing coverage

`capture-existing.cjs` captures the unchanged native TypeScript corpus,
maintainability suite, recorded agent replay, evaluator tests and provenance.
The existing Python runner is `evals/python/run.cjs`. The source archive, binary
hashes, pinned Python lock, generated npm lock and raw reports are external
evidence artifacts. This pass does not claim the complete release/platform matrix.
