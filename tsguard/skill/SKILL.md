---
name: tsguard
description: Use tsguard to check TypeScript quality, fix findings, review security or maintainability, and interpret existing results.
---

# Use tsguard

Read project `AGENTS.md`. Select a guide:

| Goal | Guide |
| --- | --- |
| Run checks or explain results | [Check](workflows/check.md) |
| Fix findings or tools | [Fix](workflows/fix.md) |
| Review vulnerabilities or secrets | [Security](workflows/security.md) |
| Review complexity or changes | [Maintainability](workflows/review.md) |

Read only relevant guides. Preserve scope and authorization. Default to `check` for general quality requests; failure investigations start with `diagnose --output json`.

Detailed formats: [result contract](https://github.com/oxalc88/oxguard/blob/main/docs/guides/tsguard-result-contract.md).
