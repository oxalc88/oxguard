# PyGuard agent contract and parity

The baseline was evaluated on 2026-10-04 at main `14a2a8a1c2f5b0fe9263092e3ebfdd147286baa1`. PR #5 first added regression evidence, then implements Python parity using the same Go result/reporting module as Tsguard. This supplements the historical architecture evaluation without changing its original conclusions.

| Capability | Baseline Python evidence | PR #5 implementation |
|---|---|---|
| Level 1 gates | Existing Ruff, mypy, Radon, annotations, coverage/security | Existing tools, thresholds and gate order retained |
| Normalized RunResult | Private name/ok/output process record only | Shared schema-1 envelope with first-class findings, measurements, artifacts and diagnostics |
| Complete JSON | Flag absent/ignored; human output | `--output json`, from deterministic normalized data |
| Bounded agent output | No finding count or complete-result drill-down | Ten findings, omission count, same 26-line/6-KiB limit as Tsguard |
| IDs and execution categories | Missing uv and source defects both exit 1 | Native rules and stable owned IDs; semantic execution errors |
| Input validation/root | Unknown flags/missing values silently ignored; upward discovery only | Invalid inputs rejected; explicit `--root` |
| Diagnostics/skill | `--tail`/`--log-file`; skill interprets full logs | Complete referenced raw streams; skill uses bounded result and complete JSON retrieval |
| Criticality | Advisory script writes an error artifact while CLI prints OK | Distinct function callers, explicit scope, truthful execution/artifact errors |
| Remaining Level 2 / Level 3 | Missing | Still missing |

## Baseline observations

Real Ruff/mypy/Radon and owned annotation code passed independent clean/faulty controls. CLI `--timout 30`, a trailing `--timeout` and `--output xml` were ignored on clean code. `--output json`/`agent` did not create a contract. Missing uv and mypy quality findings both returned 1, and the runner did not retain semantic categories. These observations required implementation, not a documentation-only assertion of parity.

The caller fixture has two distinct function callers of `target`, with two calls inside one caller. The old CLI printed `[OK] criticality`, exited 0 and wrote `pyan3 failed: [Errno 2] No such file or directory: ''` in CRITICALITY.md. The script caught that exception and returned 0. It also hard-coded `functions`/`cdk` and used DOT definition/use edges to compute in-degree, allowing namespace containment to affect counts.

## Implementation evidence

With the same pinned toolchain, current candidate and rebuilt baseline run the same corpus: all 15 behavior cases are retained, and all 20 current parity cases pass. The original nine requirements pass; five additional JSON checks cover Ruff, Radon failure/boundary, annotations and criticality measurements. Exact rule/location/count expectations are independent fixtures, not snapshots of huge logs. JSON is validated and compared across repeat runs. These small fixtures do not establish production precision/recall.

The known caller answer remains two. Its invocation now explicitly passes `--dirs functions`, matching the fixture's location; this scope correction does not weaken the expected count. The new criticality JSON case verifies `criticality.in_degree` with `core.target = 2` and advisory findings. The script uses absolute paths and pyan3 function-use edges, excluding namespace containment and repeated-call duplication. Failures produce execution records while the advisory CLI keeps exit 0.

Shared/Python Go tests additionally cover oversized diagnostic streams, tail-independent normalization, complete JSON shape, stable IDs, bounded output, missing executables/helpers, uv dependency failure, timeout, invalid configuration/output, malformed adapters, locks, threshold boundaries and advisory execution errors. CI requires every Python parity case to pass, even if the baseline lacks the feature. Existing npm/pnpm installation tests protect the TypeScript path after shared-module extraction.

See the [Python result contract](pyguard-result-contract.md) for supported adapters, fallback limits, deployment of updated owned helpers and the [capability evals](oxguard-evals.md) for reproducible commands. A broader shared policy engine, unified CLI, uvx packaging, remaining Level 2 analysis and Level 3 remain separate work.

Native coverage, Bandit, Vulture and deptry controls now exercise six additional adapter cases. Both skills also have captured and scored real agent actions for omissions, fail-fast, execution repair and advisory interpretation. The completion report combines current CLI corpus results with those skill captures; CI replays captured agent scoring and does not silently call a live LLM.
