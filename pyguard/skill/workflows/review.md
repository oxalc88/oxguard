# Maintainability

1. Pick the command for the question:

- `criticality`: advisory static caller ranking, not runtime impact.
- `audit`: advisory criticality, dead code and dependencies.
- Structural and baseline-change analyses do not exist for Python; say so.

2. Run it with `--output json`.
3. Inspect the reported locations. Separate analyzer facts from judgment. Smaller files or lower scores alone do not prove simplification. Criticality defaults to `functions/cdk` unless `--dirs` is set. Vulture and deptry stay advisory.

Done: report findings, advisories and judgments separately.
