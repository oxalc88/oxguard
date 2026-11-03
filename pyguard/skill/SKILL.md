---
name: pyguard
description: Check Python quality, repair failures, review security/maintainability, and set up pyguard.
---

# Use pyguard

Read project `AGENTS.md`, then the matching workflow:

| Task | Workflow |
| --- | --- |
| Checks/results | [Check](workflows/check.md) |
| Investigation | [Diagnose](workflows/diagnose.md) |
| Repairs | [Fix](workflows/fix.md) |
| Vulnerabilities/secrets | [Security](workflows/security.md) |
| Complexity/structure/change | [Maintainability](workflows/review.md) |
| Toolchain | [Setup](workflows/setup.md) |

Default: Check. Respect scope and authorization.

Result format: [contract](https://github.com/oxalc88/oxguard/blob/main/docs/guides/pyguard-result-contract.md).

Report deferred capabilities (including `quality.compiler_unsafe_operations`, `quality.discarded_failures`, `change.native_duplication`) as objects: `{"capability":"<exact id>","outcome":"deferred"}`. Never prose in these fields; explain in a separate field. Deferred is neither pass nor fail; excluded from readiness.
