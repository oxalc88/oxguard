# Tsguard agent and JSON result contract

Install the project-local distribution, then select an output mode:

```sh
npm install -D @oxguard/tsguard
npx --no-install tsguard check --output agent
npx --no-install tsguard check --output json
npx --no-install tsguard types --root /path/to/project --output json
```

Commands come first, followed by flags. Analysis commands support `human`
(default), `agent`, and `json`. Setup, hooks and doctor retain their human
interfaces; selecting a structured mode for those commands is an input error.
Help and version remain their existing text interfaces.

## One normalized contract

Go adapters create a `RunResult` before the agent or JSON reporter runs. The
Node launcher still only resolves native packages and forwards the process.
Neither reporter parses the rendered human CLI. Human commands retain the
existing tool output and `[OK]` / `[FAIL]` rendering.

Launcher failures (unsupported platform, missing native package, version mismatch,
or native startup failure) occur before Go starts. They retain the existing
stderr/exit interface; only a started Go CLI can produce `RunResult`.

Schema version `1` has these fields, including empty arrays on success:

| Field | Meaning |
|---|---|
| `schema_version` | String `"1"`; breaking contract changes require a new version |
| `status` | `pass`, `fail`, `error`, `advisory`, or `skipped` |
| `command` | Requested analysis command |
| `exit_code` | Compatible process exit code |
| `findings` | First-class normalized quality or execution records |
| `measurements` | Native FTA scores available from this run |
| `artifacts` | Optional full log-file reference |
| `diagnostics` | Gate, channel, ID and project-relative path for each raw analyzer stream |

Each finding has `id`, `level`, `gate`, `rule`, `severity`, `status`, `category`,
`evidence` and a `diagnostics` array of diagnostic IDs. `location` is optional
and contains `file` plus one-based `line` / `column` when known. FTA findings
include numeric `observed` and `threshold`. `remediation` is optional; this
contract does not invent a source fix when the analyzer provides none.

Rules preserve TypeScript codes, Biome categories/rule names and Opengrep
`check_id` values. Tsguard-owned rules include
`tsguard.fta.score_exceeded`, `tsguard.<gate>.failed`, and
`tsguard.execution.<category>`. IDs are the rule plus a SHA-256 of gate, rule,
project-relative location, line and column. They do not hash English messages,
measurements, absolute project roots, timestamps or PIDs. Location changes can
change an ID. No cross-revision identity or baseline comparison is promised.

Findings are sorted by execution-error/blocking/advisory priority, gate,
location and ID; measurements by file and metric. Tool execution order remains
lint → FTA → types → coverage → security. JSON has no raw `output` field, no
elapsed-time fields and no display truncation. It describes only executed gates,
not unexecuted fail-fast steps, and preserves the analyzer's available findings.

## Semantic outcome and process code

| Finding category | Agent action |
|---|---|
| `quality` | Inspect the named rule, location and evidence |
| `tool_missing` | Restore the executable/module/test runner |
| `timeout` | Inspect runtime and per-tool timeout |
| `invalid_configuration` | Correct invocation or configuration |
| `dependency_failure` | Check registry/network availability; do not assume a source defect |
| `lock_contention` | Wait for the other instance or inspect the lock |
| `startup_failure`, `interrupted`, `diagnostics_failure`, `lock_failure` | Resolve the execution environment |
| `adapter_failure` | Inspect the tool's structured-output compatibility |
| `analyzer_failure` | Inspect explicit analyzer error records |
| `unclassified_failure` | Inspect this gate's diagnostics; cause is not yet known |

Finding `status` is `blocking`, `advisory`, or `execution_error`. A run with
execution errors has `status: error`; blocking findings produce `fail`;
non-blocking findings produce `advisory`. An analyzer failure without a supported
adapter uses a stable gate-level `unclassified_failure`, never a guessed source
rule. Known npm/Node transport and module-resolution error records are detected
before fallback. Arbitrary English sentences are not exhaustively parsed.

Codes stay 0 for passing/advisory completion, 1 for blocking analyzer failures,
3 for invalid invocation/root/configuration, and 4 for lock contention. Native
partial scan diagnostics and advisory commands can expose `error` with code 0;
check semantic status as well as the process code. Failed decoding of required
structured output returns code 1 rather than claiming a pass.

Human heavy commands retain pipe refusal (code 5, `pipe_refused` internally).
Structured output is designed to be captured and accepts piped stdout without
`--allow-pipe`. This is an explicit mode-specific policy, not a gate change.

Unknown flags, missing values, invalid numbers and invalid output modes now fail
instead of silently using defaults. `--timeout` and `--max-fta-score` are positive
integers; `--tail` is a non-negative integer. `--root` must directly contain
`package.json`; omitting it retains upward cwd discovery. Structured modes fail
on malformed or unreadable `oxguard.toml`; human mode retains its warning and
fallback behavior. CLI > file > defaults remains unchanged.

## Bounded output and drill-down

Agent output shows semantic status, command, primary gate, total finding count,
and at most ten findings. Each finding has one rule/location/category/status
line and one bounded evidence line. Dynamic text is reduced to one line, control
characters are removed, and fields are capped at 240 UTF-8 bytes. The result is
at most 26 lines and 6 KiB. Extra findings are counted with `omitted`; JSON
contains all normalized records available from the executed analyzers.

```text
FAIL
command: types
gate: types
findings: 2
TS2322 src/order.ts:41 [quality/blocking]
Type 'string' is not assignable to type 'number'.
TS2345 src/service.ts:18 [quality/blocking]
Argument has the wrong type.
diagnostics: node_modules/.cache/oxguard/diagnostics/diagnostic-001-types-stdout.log (paths in --output json)
```

Structured modes automatically save full stdout and stderr separately under
`node_modules/.cache/oxguard/diagnostics/`. The IDs and paths are deterministic
for the invocation order. Each stream is written from byte zero, and adapters
read full stdout independently of the runner's 2 MiB raw-memory window.
Diagnostics are mutable cache files: later invocations can overwrite matching
paths. Copy them if evidence must survive another run. They are not snapshots.

`--tail` still limits human display; it never limits normalized findings.
`--log-file` still appends the combined full analyzer stream and is listed as an
artifact. Relative log paths retain their invocation-cwd meaning even with
`--root`; the artifact reference is made relative to the analysis root. It is optional for agent invocation. Raw analyzer content is available only through these diagnostic files.

## Adapter coverage and limits

| Analyzer | Deterministic extraction |
|---|---|
| TypeScript / tsc | `--pretty false`, TS code, project-relative file, line, column, primary message; global compiler errors are configuration failures |
| FTA | Native JSON on success; native first score-cap stderr record on failure; score/threshold finding and measurement |
| Packaged Biome/Ultracite | Biome JSON diagnostics with all diagnostics enabled, rule/category, severity, file and position |
| Opengrep | Native JSON results/errors; preserve rule IDs and positions; `--error` continues to make every match blocking |
| Coverage, secrets, dependency audit, Knip/jscpd, other lint backends | Stable gate-level fallback plus diagnostic references; no invented detailed normalization |

FTA 3.0.1 exits before printing JSON when the cap is exceeded. The adapter reads
its exact fixed first-failure record instead of rerunning the analyzer or changing
the cap. Therefore a failed FTA invocation exposes only the first failing file,
matching existing fail-fast behavior. Successful JSON supplies measured files;
FTA's existing small-file exclusions still apply.

Biome's JSON format is upstream-labelled experimental; its pinned package version
is unchanged. Unexpected or malformed supported JSON becomes `adapter_failure`.
Standalone Ultracite and packaged ESLint/Oxlint paths retain their existing backend
and use the stable gate-level fallback. Native tsc primary messages are extracted;
additional indented explanation remains in diagnostics.

Level 1 analyzers remain existing. This PR implements the normalized result
contract. Level 2 graphs/criticality and Level 3 baseline/change comparison remain
missing. The skill is intentionally unchanged; simplifying it is the next small
integration step after the CLI contract is proven.
