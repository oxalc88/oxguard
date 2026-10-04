# PyGuard agent contract and parity

Evaluated on 2026-10-04 against main `14a2a8a1c2f5b0fe9263092e3ebfdd147286baa1`.
PR #5 adds tests and reporting; it does not implement Python contract features.
This supplements the historical Tsguard evaluation without changing its conclusions.

PyGuard can be invoked by an agent, but is not yet agent friendly in the same
sense as current Tsguard. Its skill remains the layer that interprets raw logs.

| Capability | Tsguard | PyGuard evidence |
|---|---|---|
| Level 1 gates | Existing | Existing Ruff, mypy, Radon, annotation, coverage/security gates |
| Normalized RunResult | Schema 1 | `runner.go` Result only has private name/ok/output fields |
| Complete JSON | `--output json` | Flag absent; a real probe still prints human output |
| Bounded agent output | Ten findings, explicit omission count | Flag absent; no finding count or complete-result drill-down |
| Stable finding IDs / semantic categories | Implemented | No common finding fields; mypy defects and missing uv both exit 1 |
| Invalid-input rejection | Implemented | `parseFlags` ignores unknown flags and missing values; numeric parsing errors ignored |
| Explicit root | `--root` | Only walks upward for pyproject.toml; `--root` ignored |
| Diagnostics | IDs and raw files | `--tail` / `--log-file` supported; skill reads and interprets full logs |
| Criticality | Advisory caller measurements and artifact | Advisory script exists, but known-answer graph probe fails with pinned pyan3 |
| Level 2 beyond criticality | Still missing | Still missing |
| Level 3 project comparison | Still missing | Still missing |

## Direct observations

The Python corpus contains 15 passing legacy behavior checks and nine unmet parity
requirements. These counts apply only to the fixed fixtures and pinned toolchain.

- Real Ruff/mypy/Radon and owned annotation code pass clean controls and fail faulty
  ones. Exact rule/location or metric expectations come from independent fixtures;
  analyzer JSON is not presented as PyGuard JSON.
- `mypy --timout 30`, a trailing `--timeout`, and `--output xml` all run successfully
  on clean code instead of rejecting invalid input. The desired rejection tests fail.
- `--output json` and `--output agent` are silently ignored. Their requirements are
  unsupported, even when the command itself exits 0.
- Missing uv and a real type defect both return 1. `Runner.Run` does not retain tool
  exit code, timeout category or structured findings. Start failure even returns
  before the usual `[FAIL]` rendering. An agent cannot use exit 1 to infer a source fix.
- The criticality fixture has two distinct function callers of `target`; calling
  twice within one caller does not create another distinct caller. The CLI prints
  `[OK] criticality`, exits 0, and writes `pyan3 failed: [Errno 2] No such file or
  directory: ''` in CRITICALITY.md. The owned script catches the exception and returns
  0. Advisory success is therefore not proof that analysis completed.

`analysis/analyze_criticality.py` also hard-codes `functions` and `cdk`; the CLI's
`--dirs` input does not reach it. It uses default DOT definition and use edges to
compute in-degree, so namespace/definition edges can contribute to the count.
The positive fixture requires function callers; rewriting it to accept an error
artifact or namespace edges would conceal the capability gap.

## Next implementation

Give PyGuard the same public result meaning as Tsguard: schema version, status,
command, exit code, findings, measurements, artifacts and diagnostics. Use real
Ruff/mypy structured adapters and stable owned rules for remaining conditions.
Keep Python tools/thresholds and the current human workflow. Add bounded agent and
complete JSON reporters, strict flags/root, and explicit execution categories.

Then update the Python skill to consume that implemented contract, retrieve
omitted findings, and distinguish quality from execution failures. Repair and
verify criticality execution/counts using the existing positive fixture. Missing
capabilities become measured gains; they must not be accepted merely because their
flags appear in help. Graph rules beyond that milestone and Level 3 remain separate.
