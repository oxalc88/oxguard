# OxGuard capability evals

The evals contain 30 Tsguard cases and 42 PyGuard cases (16 behavior checks and 26 contract/parity requirements). They complement unit, installed-distribution and release smoke tests.

## Known answers

`evals/cases.json` records stable case IDs, fixture names and expected semantic results; `evals/fixtures/` holds reviewed source and configuration. Expectations are never generated from the candidate's output. Finding identity is checked structurally and across repeated runs; messages and logs are not golden snapshots. Passing these small fixtures does not establish production precision/recall.

## Run through an installed package

Use Node 24 and Go from `tsguard/go.mod`:

```sh
node --test evals/evals.test.cjs
OXGUARD_EVAL_REPORT_DIR=eval-reports node --test npm/distribution.test.cjs
```

The harness builds, packs and installs the real npm distribution and runs each case in a fresh temporary project. Set `TSGUARD_PACKAGE_MANAGER=pnpm` to exercise pnpm. For a package already installed elsewhere:

```sh
node evals/run.cjs --launcher /absolute/path/node_modules/@oxguard/tsguard/bin/tsguard.cjs \
  --revision COMMIT_SHA --report eval-reports/candidate.json
```

Do not substitute an uninstalled CLI or plain `npx`.

## Compare revisions

On a PR, the Linux npm job evaluates the base and candidate native binaries with the same corpus, evaluator, launcher and toolchain:

```sh
OXGUARD_EVAL_REPORT_DIR=eval-reports \
OXGUARD_EVAL_BASELINE_REF=FULL_40_CHARACTER_COMMIT_SHA \
node --test npm/distribution.test.cjs
node evals/compare.cjs eval-reports/npm-linux-x64/baseline.json \
  eval-reports/npm-linux-x64/candidate.json eval-reports/comparison.json
```

Reports with different corpus/evaluator hashes, environments or case sets cannot be compared.

## Results and merge rule

Each case is `pass`, `fail`, `unsupported` (a historical CLI lacks the interface) or `skipped` (only the conditional symlink case). The candidate must pass every required case, even when the baseline also failed; losing a previously passing case blocks. A new capability needs positive, negative and boundary cases. Changing an expected answer requires a documented behavior decision and review; never rewrite an oracle to get green CI. Store reports as CI artifacts, not committed answers.

## PyGuard

Requires Python >=3.12 (3.14.1 is not supported).

```sh
uv venv --python 3.12 /tmp/pyguard-evals
uv pip install --python /tmp/pyguard-evals/bin/python --require-hashes -r evals/python/requirements.txt
(cd pyguard && go build -o /tmp/pyguard-eval .)
node evals/python/run.cjs --binary /tmp/pyguard-eval --source . \
  --python /tmp/pyguard-evals/bin/python --revision COMMIT_SHA \
  --report-dir eval-reports/pyguard
```

Use uv 0.12.19 (on Windows the interpreter is `Scripts/python.exe`). `--baseline-binary`, `--baseline-source` and `--baseline-revision` evaluate a rebuilt base commit. `behavior.json` requires every behavior case to pass; `parity.json` requires all 26 parity requirements. `evals/python/cases.json` holds the PyGuard corpus; a case pins the analyzer's own exit status with `oracle_exit_code` when findings do not imply it (mypy exits 2 on a syntax abort). `evals/capabilities.json` maps the 18 shared capabilities across both languages. See [PyGuard agent evaluation](pyguard-agent-evaluation.md).

## Level 1 completion gate

The report job runs `evals/level1-ready.cjs` and publishes `level1-ready.json`. Readiness requires every case in both corpora and all 18 shared capabilities. `level1_agent_contract_ready` also requires the agent traces to be ready and current: on PRs, failing or stale traces are a warning; releases set `OXGUARD_REQUIRE_CURRENT_TRACES=1` and block.

### Agent traces (schema 2)

`agent-traces/{typescript,python}.json` record `skill_inventory` and `sessions.{quality,repair,review,focused}`, each with `agent`, `model`, `effort`, `read_source`, `skill_files` (path → normalized SHA-256) and the raw stream hash. `skill_files` are the files the agent actually read plus a declared minimum per session. Changing the inventory makes every session stale; otherwise a session is stale only when one of its own files changed. `node evals/agent-actions.cjs` warns per stale session and fails on wrong behavior, or on stale with `--require-current`. `agent-actions.test.cjs` (blocking) tests the scorer and the skill word limits: total ≤ 700, entry ≤ 150.

### Refresh skill evidence

Re-record only stale sessions, with fresh independent agents; never replace only hashes. Keep the capture directory at `$OXGUARD_CAPTURE_DIR` (default `/workspace/oxguard-capture`) with `ts-toolchain/`, `npm-cache/`, `py-env/`, `uv-cache/`, `bin-real/pyguard` and the packed tsguard `*.tgz`. Then:

```sh
evals/agent-capture/record.sh <effort> [previous-replay-dir] lang/session ...
node evals/agent-capture/assemble.cjs captures <language> <capture-root>/replay [session=dir ...] [--note file]
```

`record.sh` rebuilds fixtures, keeps the sessions not re-recorded and runs the selected agents in parallel; `assemble.cjs` builds the trace from saved streams and hashes each session's installed skill copy. Give agents the task scope without scorer expectations, and keep fixtures outside the repository. `evals/record-agent-command.cjs` is the wrapper that observes the agents' real commands (argv, exit code, output and source fingerprints); configure it with absolute tool and project paths:

```json
{
  "tools": { "pyguard": "/absolute/path/pyguard", "uv": "/absolute/path/uv" },
  "projects": [{ "root": "/absolute/path/fixture", "events": "/absolute/path/events.jsonl", "env": { "UV_OFFLINE": "1" } }]
}
```
