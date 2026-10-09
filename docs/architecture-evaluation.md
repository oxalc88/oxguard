# Tsguard architecture evaluation

**Evaluation date:** 2026-10-03  
**Canonical code evaluated:** `main` at `c26afb524d68dc49fe2ecdc235e742bbe7ec8da6`  
**Scope:** architecture and agent-facing behavior only. No Tsguard features, gates, thresholds, npm behavior, or skills are changed by this evaluation.

## Implementation status after PR #3

The evaluation below remains the evidence-based assessment of `main` at the recorded SHA. Its conclusions and scorecard are preserved. This implementation adds the first recommended contract step and the requested PyGuard criticality parity milestone. It does not add blocking structural rules or change analysis.

| Area | Implementation status |
|---|---|
| Level 1 analyzers | Existing; analyzers, thresholds, gate order and native fail-fast behavior preserved |
| Normalized result contract | Implemented in Go by this PR: schema version 1, explicit gate execution/normalization status, assessment completeness, findings, measurements, diagnostic references and artifacts |
| Agent / JSON outputs | Implemented: `--output agent` is bounded; `--output json` contains the complete normalized run |
| Input contract | Unknown/missing/invalid flags rejected; explicit `--root`; structured modes reject unreadable/malformed `oxguard.toml` |
| Progressive disclosure | CLI reduction implemented for tsc, FTA, packaged Biome, Opengrep, coverage, secrets, dependencies, Knip and jscpd; unsupported backends remain explicitly partial |
| Human / npm interfaces | Human rendering retained; launcher and packaging unchanged; structured modes accept piped stdout |
| Skill debt | Agent skill updated to use installed npm/pnpm CLI, normalized outputs, omitted-finding JSON retrieval, execution categories and advisory criticality |
| Level 2 | PyGuard criticality parity implemented: compiler-resolved function/method caller graph, in-degree, top 30, advisory `CRITICALITY.md` in audit; remaining structural analysis still missing |
| Level 3 | Still missing |

See [agent and JSON contract](guides/tsguard-result-contract.md) for schema, semantic categories, diagnostic lifecycle and adapter limits. Historical claims of missing contracts below describe the evaluated revision, not this follow-up.

## Python contract parity implemented by PR #5

PyGuard now shares the Go schema-1 result/reporting module with Tsguard and has native Ruff/mypy/Radon/Bandit/pip-audit/coverage/deptry adapters, Vulture native API records, structured owned helpers, bounded agent output, complete JSON, strict flags/root and explicit execution categories. The Python skill consumes that implemented contract and retrieves omitted findings through JSON. Level 1 analyzers remain existing. The advisory criticality milestone now counts distinct function callers, honors CLI scope and exposes failures truthfully; remaining Level 2 and Level 3 are still missing.

The Python corpus retains 15 baseline behavior passes and passes 20 parity cases, including six native coverage/security/dead-code/dependency controls. Both versions run against the same corpus and pinned toolchain. See [Python contract](guides/pyguard-result-contract.md), [PyGuard evidence](guides/pyguard-agent-evaluation.md) and [capability evals](guides/oxguard-evals.md). This updates implementation status only; the original evaluation and recommendations below remain unchanged.

## Executive assessment

**v0.8 candidate implementation update (2026-10-09):** Tsguard now adds an explicit zero-config quality baseline, compiler-backed typed lint, advisory delegation/handler findings, a static module graph with SCC depth and coupling measurements, and inert Git source comparisons for supported complexity-displacement scenarios. The existing normalized contract and Level 1 fail-fast gates remain. New PyGuard maintainability parity and broader change metrics remain gaps; full release readiness is not claimed. See [scope and boundaries](maintainability.md) and [validation](maintainability-validation.md). The executive assessment below describes the original evaluated revision.

Tsguard is a useful **Level 1 code-quality gate** with a strong npm delivery path, but it is not yet an agent-native analysis engine. The Go CLI orchestrates deterministic analyzers well, but its public result contract is still human CLI text. There is no common finding model, no machine-readable output, no Level 2 graph model, and no Level 3 baseline/change model.

The npm launcher is now the preferred installation path for TypeScript projects because it gives an agent one project-local dependency and a tested native launcher without requiring `tsguard setup`, a global binary, or analyzer-by-analyzer installation.

The main agent problem is after invocation:

```text
agent
  |
  v
npx --no-install tsguard <gate>
  |
  v
Go runner
  |
  +--> analyzer A prose
  +--> analyzer B prose
  +--> analyzer C prose
  |
  v
[OK]/[FAIL] wrapper text + exit code
  |
  v
skill reads raw logs and interprets them with the LLM
```

That makes execution deterministic, but interpretation is not deterministic or context-efficient.

**Current maturity**

| Area | Assessment |
|---|---|
| Level 1 — Code | **IMPLEMENTED / mature enough to gate** |
| Level 2 — Structure | **MISSING**, except PyGuard has an advisory criticality implementation that defines the intended parity milestone |
| Level 3 — Change | **MISSING / not ready** |
| Agent friendliness | **1.6 / 3** |

---

## 1. Current architecture

### 1.1 Actual npm execution path

The current npm path is:

```text
npm install -D @oxguard/tsguard
            |
            v
node_modules/.bin/tsguard
            |
            v
npm/tsguard/bin/tsguard.cjs
            |
            | selects @oxguard/tsguard-<os>-<arch>
            | checks native package version
            | injects TSGUARD_RUNTIME / TSGUARD_NODE / TSGUARD_OPENGREP
            v
native Go tsguard binary
            |
            v
main.go -> parseFlags -> findProjectRoot -> buildConfig -> dispatch
            |
            v
targets.go
            |
            v
Runner.Run
            |
            +--> package-owned Node tools through bin/tool.cjs
            +--> native FTA executable
            +--> native Opengrep
            +--> project test runner where applicable
            |
            v
stdout / stderr / exit status / generated artifacts
```

The Node launcher does not reimplement gate logic. Gate orchestration remains in Go.

### 1.2 Go orchestration

`tsguard/main.go` provides the command surface and dispatches to functions in `targets.go`. `Runner.Run` executes each underlying tool from the discovered project root.

Important current properties:

- `check` is sequential and fail-fast: lint -> FTA -> types -> coverage -> security.
- `audit` is advisory and always returns exit 0 even when Knip or jscpd reports findings.
- subprocess output is captured by the Go runner;
- analyzer stderr is merged into analyzer stdout before Tsguard interprets or logs it;
- the in-memory captured stream is capped at 2 MiB per tool;
- the optional log file receives the complete analyzer stream and is not subject to that 2 MiB in-memory cap;
- the Go wrapper itself prints `[OK]` / `[FAIL]` separately from the analyzer stream.

There is no analysis-domain layer between "tool output" and "reporter output". The effective internal result is currently:

```text
Result
├── name
├── ok
└── output: string
```

This is a process result, not a quality finding model.

---

## 2. Current agent interaction

### 2.1 Preferred installation path

For an agent operating in a TypeScript repository, npm should now be the preferred path:

```sh
npm install -D @oxguard/tsguard
npx --no-install tsguard check
```

Use `npx --no-install` after installation when deterministic agent execution matters. The repository's npm integration test uses that form. Plain `npx tsguard` is convenient for humans but can involve npm's package-resolution/fetch behavior when the local executable is absent.

For pnpm:

```sh
pnpm add -D @oxguard/tsguard
pnpm exec tsguard check
```

### 2.2 Input contract

| Input | Current support | Assessment |
|---|---|---|
| command / gate | positional command | **Strong** |
| project root | inferred by walking upward from cwd until `package.json` | **Implicit** |
| changed files | no general changed-file input; `--if-typescript` only decides run/skip from hook stdin | **Partial** |
| directories | `--dirs d1,d2` | **Implemented** |
| exclusions | `--exclude d1,d2` plus defaults | **Implemented** |
| configuration | implicit `oxguard.toml` at detected root | **Implemented but implicit** |
| baseline | none | **Missing** |
| output mode | no agent/JSON/human mode | **Missing** |
| verbosity | `--tail`, `--log-file` | **Partial** |
| timeout | `--timeout` per tool | **Implemented** |

Input precedence is deterministic for supported values:

```text
CLI
 |
 v
oxguard.toml
 |
 v
built-in defaults
```

However, the public parser is permissive rather than strict. Unknown flags and missing values are not rejected with a stable validation error. That is risky for agents because a mistyped argument can silently fall back to another behavior.

### 2.3 Implicit execution state

Normal execution still depends on state that is not fully visible in the command:

- cwd determines the project root;
- package-manager detection determines command execution;
- existing project configs can change analyzer behavior;
- declared project TypeScript and Vitest versions can override package-owned ones;
- `oxguard.toml` is loaded automatically;
- malformed `oxguard.toml` prints a warning and falls back instead of failing;
- full security checks can require registry/network access;
- default Opengrep rule identifiers can require remote rule retrieval;
- `npm audit` / `audit-ci` depend on package-registry availability;
- the heavy-gate pipe policy depends on how the calling shell/tool connects stdout.

The npm path removes a large amount of previous setup state, but the analysis contract is still partly environment-sensitive.

### 2.4 Project mutation

The npm installation itself has no lifecycle setup hook and does not mutate project configuration beyond the normal dependency/lockfile install.

Some gate execution is not strictly read-only:

- `check` removes stale `coverage/` before running;
- coverage tools generate coverage artifacts;
- FTA can write `node_modules/.cache/oxguard/fta.json`;
- fallback Biome/TypeScript/Secretlint config is written under `node_modules/.cache/oxguard/defaults`;
- explicit `setup`, `hooks`, and `fix` are mutating commands.

This is acceptable for a quality tool, but agents need a stable distinction between read-only analysis outputs, cache artifacts, and requested source/config mutation.

---

## 3. Execution evidence and measured inputs/outputs

### 3.1 Evidence source

The current npm integration test, `npm/distribution.test.cjs`, builds the real Go binary, prepares the actual npm launcher/native packages, installs them into a temporary consumer, and invokes the installed command.

The latest successful `main` workflow evaluated was GitHub Actions run **36429671182**, at the same main SHA used by this document. It passed the npm/pnpm matrix on Linux, macOS, and Windows.

The test executes or verifies:

```text
npx --no-install tsguard --version
npx --no-install tsguard --help
npx --no-install tsguard doctor
npx --no-install tsguard fix
npx --no-install tsguard lint
npx --no-install tsguard types
npx --no-install tsguard fta
npx --no-install tsguard coverage
npx --no-install tsguard secrets
npx --no-install tsguard audit
npx --no-install tsguard check
npx --no-install tsguard security
```

It also verifies type failure, SAST failure, arbitrary argument forwarding, stdin, stdout, stderr, exit codes, native-version mismatch, missing native package, unsupported platform, startup failure, and signal forwarding.

The integration harness captures most command stdout/stderr for assertions instead of echoing it into Actions logs. Therefore this evaluation does not invent byte counts for captured analyzer output that the CI log does not expose.

### 3.2 Representative command contract

| Command | What an agent receives today |
|---|---|
| `--version` | one version line; exit 0 |
| `--help` | human usage text; **38 lines / 2,233 UTF-8 bytes** from the current usage constant |
| `doctor` | human `[OK]` / `[FAIL]` lines per dependency; exit 0 or 1 |
| `check` | small wrapper text on success; on failure, analyzer-specific prose unless `--log-file` hides it; fail-fast |
| `fta` | wrapper status plus FTA's human output on failure; no normalized file/score finding |
| `audit` | Knip and jscpd human output; always exit 0, so findings cannot be inferred from process status |
| `security` | Secretlint, PM audit/audit-ci, Opengrep output; blocking result is exit 1, but individual finding semantics remain tool-specific |

The current skill file itself is **184 lines / 8,020 bytes** before it reads any analyzer output.

### 3.3 Stdout and stderr behavior

At the npm launcher boundary:

```text
parent stdin  -> native stdin
native stdout -> parent stdout
native stderr -> parent stderr
native exit   -> launcher exit
```

Inside the Go runner:

```text
analyzer stdout --+
                  +--> one captured stream
analyzer stderr --+
                        |
                        +--> optional full log file
                        |
                        +--> human failure rendering
```

Therefore analyzer stdout/stderr identity is lost after entering `Runner.Run`.

### 3.4 Exit codes

Observed/public semantics are:

| Code | Meaning |
|---:|---|
| 0 | command passed, or advisory command completed |
| 1 | a blocking analyzer/gate failed, doctor failed, or many execution/tool errors |
| 2 | setup-specific Node environment failure in the standalone setup path |
| 3 | unknown command / project-root discovery error |
| 4 | another Tsguard instance holds the lock |
| 5 | heavy gate refused because stdout is a pipe |
| signal-derived | launcher preserves Unix signal termination; Windows maps signals conventionally |

The npm integration test explicitly exercises 0, 1, 3, 4, 5, and an arbitrary 37 through its native fixture.

The semantics are stable enough for "pass/fail/retry" decisions, but not enough to identify a quality finding category. Exit 1 intentionally collapses many analyzer and environment failures. `audit` intentionally reports findings while returning 0.

### 3.5 Output boundedness

There are three distinct behaviors:

**Default failure**

```text
[FAIL] <tool>
<raw analyzer output>
```

The in-memory raw output is capped at 2 MiB per tool. That is a memory safety bound, not an agent-context bound. A 2 MiB failure is still much too large for routine LLM consumption.

**`--tail N`**

The runner keeps only the final N lines for display. This is useful but can omit the primary finding if the analyzer prints summaries at the end and details earlier.

**`--log-file PATH`**

The complete analyzer stream is appended to the file. When a tool fails, stdout becomes approximately:

```text
[FAIL] <tool> (see <path>)
```

Important architecture detail: the log file contains subprocess output, but the wrapper's `[OK]` / `[FAIL]` labels are not written into that same log. Multiple tool streams are appended without a Tsguard-owned structured boundary record.

This means the current skill's "read the log and prioritize [FAIL] sections" model is not backed by a stable log format.

The on-disk log is also not capped by the runner's 2 MiB in-memory buffer. Reading it in chunks is only an LLM workaround.

---

## 4. Progressive disclosure

Desired:

```text
             analysis engine
                    |
          detailed deterministic data
                    |
         +----------+----------+
         |          |          |
         v          v          v
      AGENT        JSON       HUMAN
      small       complete    detailed
```

Current:

```text
               analyzers
                   |
                   v
             raw text streams
                   |
            +------+------+
            |             |
            v             v
       CLI wrapper      log file
       [OK]/[FAIL]      raw/full
            |             |
            +------+------+
                   |
                   v
                  LLM
         manually reconstructs
              the findings
```

### Assessment

Tsguard has **output suppression**, but not true progressive disclosure.

`--tail` and `--log-file` can reduce what appears in the terminal, but there is no deterministic middle layer that answers:

```text
status
gate
finding
location
reason
evidence
next action
```

without reading analyzer prose.

### Skill debt

`tsguard/skill/SKILL.md` currently compensates for CLI deficiencies. It instructs the agent to:

- verify a global `tsguard` binary;
- install from GitHub Releases if missing;
- force `--allow-pipe`;
- create a full log file;
- read that log;
- read large logs in chunks;
- manually rank different analyzer findings;
- manually translate analyzer-specific prose into one finding format.

That is **architecture debt**, not a desirable permanent agent layer.

The LLM should choose remediation and explain tradeoffs. It should not be the parser or reducer for deterministic analyzer results.

---

## 5. Quality phase evaluation

## Level 1 — Code quality

Question: **Is this individual unit of code difficult or risky to maintain?**

| Capability | Status | Policy | Current implementation |
|---|---|---|---|
| FTA / maintainability | **IMPLEMENTED** | blocking | `fta-cli`, per-file score cap; default 60 |
| cyclomatic / cognitive complexity | **IMPLEMENTED** | blocking | FTA includes cyclomatic input; Biome/Ultracite enforces cognitive-complexity rules |
| Halstead | **IMPLEMENTED** | blocking as part of FTA | FTA score includes Halstead volume; not exposed as a normalized independent finding |
| types | **IMPLEMENTED** | blocking | `tsc --noEmit` |
| lint / format | **IMPLEMENTED** | blocking | Ultracite/Biome |
| coverage | **IMPLEMENTED** | blocking | 80% line/function/branch/statement floor |
| security — secrets | **IMPLEMENTED** | blocking | Secretlint |
| security — dependency CVEs | **IMPLEMENTED** | blocking via audit-ci | package-manager audit is informational; audit-ci gates |
| security — SAST | **IMPLEMENTED** | blocking | Opengrep |
| dead code | **IMPLEMENTED** | advisory | Knip via `audit` |
| duplicates | **IMPLEMENTED** | advisory | jscpd via `audit` |

Level 1 is functionally strong. Its primary weakness is not missing analyzers; it is the lack of normalized findings and stable machine output.

## Level 2 — Structural quality

Question: **Is the structure of the system unnecessarily difficult to understand or change?**

| Target capability | Tsguard status |
|---|---|
| function/method call graph | **MISSING** |
| module dependency graph | **MISSING** |
| criticality / in-degree | **MISSING** |
| fan-in / fan-out | **MISSING** |
| cycles | **MISSING** |
| dependency depth | **MISSING** |
| pass-through wrappers | **MISSING** |
| single-use abstractions | **MISSING** |
| module proliferation | **MISSING** |
| architecture/layer rules | **MISSING** |

Knip and jscpd are useful Level 1/advisory analyzers. They are not substitutes for a structural graph model.

### PyGuard criticality parity

PyGuard currently provides the reference milestone:

```text
Python files
    |
    v
pyan call graph
    |
    v
NetworkX directed graph
    |
    v
in-degree
    |
    v
descending ranking
    |
    v
top 30
    |
    v
CRITICALITY.md
```

It is integrated into `pyguard audit` and is advisory.

Tsguard currently has **none** of the parity chain:

| Parity property | PyGuard | Tsguard |
|---|---|---|
| unit = function/method | yes | no |
| graph = caller -> callee | yes | no |
| metric = in-degree | yes | no |
| descending ranking | yes | no |
| top N = 30 | yes | no |
| `CRITICALITY.md` | yes | no |
| advisory severity | yes | no |
| integrated in `audit` | yes | no |

This is a feature gap. It should not be solved by an LLM inspecting imports or reading source heuristically. The intended path remains deterministic AST -> graph -> metric -> finding/artifact.

## Level 3 — Change quality

Question: **Did this change genuinely simplify the system, or only move complexity elsewhere?**

Current readiness: **not ready**.

Tsguard has no:

- baseline identifier;
- baseline snapshot;
- common metric representation;
- normalized file/symbol identity across analyzers;
- structural graph measurement;
- current-vs-baseline comparator;
- change finding type;
- stable machine output to preserve comparison evidence.

Level 3 must not parse Level 1/2 CLI prose.

The minimum reusable data flow should be:

```text
                  baseline
                     |
             +-------+-------+
             |               |
             v               v
       code measures   structure measures
             |               |
             +-------+-------+
                     |
                  snapshot
                     |
              deterministic
                 compare
                     |
                     v
             change findings
```

For complexity displacement, Level 3 needs measurements such as:

- FTA score by file;
- cognitive/cyclomatic observations where available;
- file/module counts;
- graph node/edge counts;
- fan-in/fan-out;
- maximum dependency/call depth;
- cycles;
- wrapper/abstraction counts.

---

## 6. Internal finding model

### Current state

Tsguard does not have a common quality finding representation.

The existing `Result` only knows:

```text
tool name
success/failure
captured text
```

That creates direct coupling:

```text
analyzer text
    |
    +--> human CLI
    |
    +--> skill instructions
    |
    +--> agent parsing logic
```

Each new analyzer therefore risks adding a new output dialect that the skill or agent must understand.

### Recommended minimal domain model

A small Go model is enough; no framework is required.

Conceptually:

```text
Finding
├── id
├── level          code | structure | change
├── gate
├── rule
├── severity
├── status
├── location
│   ├── file
│   ├── line
│   └── symbol
├── observed
├── threshold
├── evidence
└── remediation
```

Level 3 also needs measurements, not only findings:

```text
Measurement
├── metric
├── level
├── subject
├── value
├── unit
└── evidence
```

And one run envelope:

```text
RunResult
├── status
├── command
├── project
├── findings[]
├── measurements[]
├── artifacts[]
└── diagnostics[]
```

The exact field names are not prescribed. The architectural requirement is that analyzers produce deterministic domain data before reporters or LLM integrations consume it.

This would reduce coupling between:

- analyzer adapters;
- CLI reporting;
- JSON reporting;
- agent reporting;
- human reports;
- future Level 3 comparison.

Raw analyzer text can remain available as diagnostic evidence or an artifact, but it should not be the integration contract.

---

## 7. npm architecture evaluation

### 7.1 Launcher contract

| Concern | Evidence | Assessment |
|---|---|---|
| argument forwarding | `process.argv.slice(2)` passed to native child; arbitrary spaced argument tested | **Strong** |
| cwd preservation | child spawned without cwd override; Go then finds project root from cwd | **Strong** |
| environment forwarding | `...process.env` plus Tsguard runtime variables | **Strong** |
| stdout/stderr | `stdio: inherit` | **Strong** |
| stdin | inherited and explicitly tested | **Strong** |
| exit-code forwarding | native status forwarded; arbitrary 37 tested | **Strong** |
| signal forwarding | SIGINT/SIGTERM and Unix SIGHUP tested | **Strong** |
| unsupported platform | fails with explicit OS/arch message | **Strong** |
| missing native package | explicit reinstall instruction | **Strong** |
| native version mismatch | launcher compares native package version with main package version | **Strong** |
| package scripts | no install scripts required | **Strong** |
| platform coverage | npm/pnpm matrix on Linux/macOS/Windows | **Strong** |

The npm adapter should not be redesigned for agent use. Its current responsibility boundary is good.

### 7.2 Preferred agent invocation

Recommended direction:

```text
agent
  |
  v
project-local @oxguard/tsguard
  |
  v
npx --no-install tsguard <explicit command/flags>
  |
  v
stable Tsguard contract
```

rather than:

```text
agent
  |
  v
command -v / curl / global install
  |
  v
setup discovery
  |
  v
project dependency mutation
  |
  v
tsguard
```

### 7.3 Stale npm-era integration documentation

The repository is currently inconsistent:

- `README.md` correctly presents npm as a complete installation path with no separate setup needed to run commands.
- `docs/guides/npm-distribution.md` correctly documents the npm-owned toolchain.
- `tsguard/skill/SKILL.md` still begins with `command -v tsguard` and a GitHub Release/global-install flow.
- `docs/AI_INSTALL.md` still gives agents the GitHub Release installer and `tsguard setup --yes` as the primary TypeScript install procedure.
- generated hooks commonly invoke bare `tsguard`, which assumes PATH resolution rather than explicitly documenting the project-local npm path.

These are documentation/integration debts. This task intentionally does not change them.

---

## 8. Agent-friendliness scorecard

Scale:

- **0** = absent
- **1** = poor / manual
- **2** = usable with friction
- **3** = strong deterministic contract

| Area | Score | Reason when below 3 |
|---|---:|---|
| Installation | **3** | npm package owns analyzers/native binaries and needs no setup to run |
| Invocation | **2** | project-local invocation is simple, but heavy gates can reject piped stdout unless `--allow-pipe`; root remains cwd-derived |
| Input clarity | **2** | dirs/excludes/timeout are explicit, but root/config/tool resolution are implicit; no general changed-file scope; parser is permissive |
| Exit-code semantics | **2** | stable coarse codes exist, but exit 1 collapses findings and execution failures; audit findings still return 0 |
| Output boundedness | **2** | `--tail` and a 2 MiB memory cap exist, but default failure output can still be huge and full log files are not bounded for agent context |
| Machine-readable output | **0** | no Tsguard-owned JSON/SARIF/stable data contract |
| Finding consistency | **1** | wrapper status is consistent; actual findings remain analyzer-specific prose |
| Progressive disclosure | **1** | CLI can hide raw output but cannot emit a bounded deterministic finding summary |
| Artifact discoverability | **1** | artifacts are analyzer/cache-specific; no run manifest or artifact index |
| Context-window efficiency | **1** | the skill explicitly reads full logs/chunks and performs LLM-side reduction |
| CI usability | **2** | npm path is strongly CI-tested, but pipe handling and security network dependencies add friction |
| Agent-tool portability | **2** | CLI is cross-platform and hooks exist, but skill/install assumptions and shell/PATH differences leak into integrations |

**Average: 19 / 36 = 1.58, rounded to 1.6 / 3.**

---

## 9. Architecture debt vs feature gaps

## ARCHITECTURE DEBT

1. **No machine-readable Tsguard result contract.** Consumers must parse human/analyzer output.
2. **No normalized finding/measurement model.** Analyzer-specific text leaks directly to reporters and agent integration.
3. **The skill performs deterministic reduction with an LLM.** It reads raw logs, ranks findings, and translates tool dialects.
4. **The log-file contract is not self-describing.** Raw subprocess streams are concatenated; Tsguard's own gate labels are not part of the log.
5. **Default failure output is too large for agents.** A 2 MiB process buffer is not a useful context-window limit.
6. **`--tail` is line truncation, not semantic reduction.** Important evidence can be removed while noise remains.
7. **Analyzer stderr is merged into stdout.** Downstream consumers lose channel semantics.
8. **Input validation is permissive.** Unknown flags and missing flag values do not produce a strict deterministic contract.
9. **Project root is only implicit cwd discovery.** There is no explicit root input for remote/agent orchestration.
10. **Changed-file support is only a hook run/skip heuristic.** It is not a stable scope contract.
11. **Pipe refusal is an execution-environment concern exposed to agents.** Skills need `--allow-pipe` knowledge.
12. **npm-era documentation is inconsistent.** The current skill and AI install guide still assume the older global/GitHub Release flow.
13. **Security reproducibility depends on external registries.** Default remote Opengrep rules and vulnerability data are not a fully pinned offline input.

## FEATURE GAP

1. TypeScript function/method call graph.
2. TypeScript in-degree criticality ranking.
3. Tsguard `CRITICALITY.md` parity with PyGuard.
4. Module dependency graph.
5. Fan-in/fan-out metrics.
6. Cycle detection.
7. Dependency/call depth.
8. Pass-through wrapper detection.
9. Single-use abstraction detection.
10. Module proliferation rules.
11. Architecture/layer dependency rules.
12. Baseline snapshot model.
13. Baseline-vs-current comparison.
14. Complexity-displacement detection and other Level 3 change findings.

The two categories should remain separate. Adding graph features without fixing the result contract would increase the amount of raw text an agent must parse.

---

## 10. Recommended target architecture

### 10.1 Reporting architecture

```text
                         TSGUARD
                            |
              +-------------+-------------+
              |                           |
              v                           v
            INPUT                       ENGINE
      command/root/scope/config     deterministic analyzers
              |                           |
              |                           v
              |                 measurements + findings
              |                           |
              +---------------------------+
                            |
                   normalized RunResult
                            |
              +-------------+-------------+
              |             |             |
              v             v             v
            AGENT          JSON          HUMAN
           reporter       reporter       reporter
              |             |             |
        small bounded     complete      detailed CLI
           semantic       contract        output
            output
```

The agent reporter should normally answer only:

```text
FAIL structure.complexity_displacement
Affected:
- src/orders/service.ts
- src/orders/manager.ts
Evidence:
FTA: 82 -> 44
dependency depth: 2 -> 5
new wrappers: 2
```

The agent should request or read full diagnostics only for a specific finding that needs drill-down.

### 10.2 Quality-layer architecture

```text
Level 1
CODE
  |
  | normalized measurements/findings
  v
Level 2
STRUCTURE
  |
  | normalized measurements/findings
  v
Level 3
CHANGE
  |
  v
baseline/current comparison findings
```

Level 3 consumes the same deterministic measurements produced by Levels 1 and 2. It does not rerun interpretation against rendered text.

### 10.3 Fit with the existing Go architecture

This target fits the existing codebase without a framework rewrite.

Keep:

- current Go CLI entry point;
- command dispatch;
- Runner/process control;
- npm launcher/native-package design;
- existing analyzers and thresholds;
- fail-fast behavior where desired.

Add later, in small slices:

```text
analyzer invocation
      |
      v
deterministic adapter
      |
      v
Finding / Measurement / Artifact
      |
      v
RunResult
      |
      +--> current human renderer
      +--> bounded agent renderer
      +--> JSON renderer
```

The important boundary is **before** the LLM. If an analyzer only emits text, a deterministic adapter may parse that tool's stable output or, preferably, invoke the analyzer's native structured mode when one exists. That parsing belongs in Tsguard, not in `SKILL.md`.

---

## 11. Prioritized next steps

### 1. Stabilize the agent-facing input/output contract

Define a versionable contract for command, explicit root/scope, config, timeout, output mode, and exit semantics. Reject invalid/unknown inputs instead of silently ignoring them.

A minimal internal run envelope is needed here even if the full structural model comes later.

### 2. Remove unnecessary agent log consumption

Make Tsguard perform deterministic reduction before the agent sees output. The normal agent path should receive a bounded finding summary. Full raw logs remain opt-in diagnostic evidence.

After that exists, simplify the skill so it invokes and acts on Tsguard results rather than parsing them.

### 3. Bring Tsguard criticality to PyGuard semantic parity

Implement only the already-defined milestone:

```text
TypeScript function/method
        |
caller -> callee graph
        |
      in-degree
        |
descending top 30
        |
 CRITICALITY.md
        |
 advisory in audit
```

Do not add unrelated Level 2 rules in the same slice.

### 4. Define the reusable structural representation

Stabilize graph node identity, graph edges, measurements, findings, severity, evidence, and artifacts so future Level 2 checks share one deterministic model.

### 5. Add deterministic Level 2 graph checks

Add cycles, fan-in/fan-out, depth, dependency rules, wrappers, single-use abstractions, and proliferation as separate deterministic rules over AST/graph data.

### 6. Stabilize Level 2

Validate semantics, false-positive policy, output contracts, performance, and agent-context bounds before introducing historical comparison.

### 7. Add Level 3 baseline/change comparison

Persist or load normalized measurements for a chosen baseline, compare them with current measurements, and emit change findings such as `POSSIBLE_COMPLEXITY_DISPLACEMENT`.

Do not implement Level 3 by parsing CLI logs or rendered Markdown.
