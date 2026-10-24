# PyGuard agent contract and parity

PyGuard shares Tsguard's Go result module: a schema-1 envelope with findings, measurements, artifacts and diagnostics; `--output json`; bounded agent output (ten findings, omission count, 26 lines / 6 KiB); stable IDs and execution categories; strict input validation and `--root`; and truthful criticality results.

CI requires all 15 behavior cases and all 22 parity cases to pass (see [capability evals](oxguard-evals.md)). Adapter limits are in the [Python result contract](pyguard-result-contract.md). Maintainability parity with Tsguard is tracked in [#23](https://github.com/oxalc88/oxguard/issues/23).
