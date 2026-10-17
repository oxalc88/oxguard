# Review security

Read [Check](check.md). Use `pyguard security --output agent`, preserving the requested root and scope.

Inspect every sub-gate: Bandit, pip-audit and detect-secrets complete independently even when another fails. The outer check remains fail-fast. Missing engines or skipped audits are execution failures.

Preserve native rule IDs, locations, package identity and fixed versions. Never expose secret values or baseline hashes.

Development-only classification does not waive vulnerabilities. Without a valid explicit review, retain native blocking policy. Never create acceptance entries merely to pass.

For an authorized `.oxguard-dependency-policy.json` review, require package/advisory identity, `scope: development_tool` or `ci`, `product_exposure: not_exposed`, `tooling_risk: accepted`, an evidence-backed reason, and SHA-256 bindings for the manifest and every existing supported lockfile. Stale or malformed reviews cannot waive findings. Accepted reviews stay visible. Hashes do not prove runtime reachability.
