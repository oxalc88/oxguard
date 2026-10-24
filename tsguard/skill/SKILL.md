---
name: tsguard
description: Use tsguard to check TypeScript quality, investigate failures, fix findings, review security or maintainability, and set up the toolchain.
---

# Use tsguard

Read project `AGENTS.md`, then only the workflow for the task:

| Task | Workflow |
| --- | --- |
| Run checks or read results | [Check](workflows/check.md) |
| Investigate a failure before repair | [Diagnose](workflows/diagnose.md) |
| Fix findings or broken tools | [Fix](workflows/fix.md) |
| Review vulnerabilities or secrets | [Security](workflows/security.md) |
| Review complexity or changes | [Maintainability](workflows/review.md) |
| Install or verify the toolchain | [Setup](workflows/setup.md) |

Default to Check. Stay within the requested scope and authorization.

Result format: [contract](https://github.com/oxalc88/oxguard/blob/main/docs/guides/tsguard-result-contract.md).
