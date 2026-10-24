# Maintainability

1. Pick the command for the question:

- `criticality`: advisory static caller ranking, not runtime impact.
- `maintainability`, `smells`, `structure`: delegation, failure-policy and graph observations.
- `change --baseline <ref>`: compare with a supplied or configured baseline. Never invent one.

2. Run it with `--output json`.
3. Inspect the reported locations. Separate analyzer facts from judgment. Smaller files or lower scores alone do not prove simplification. Static matches do not prove behavioral equivalence.

Done: report findings, advisories and judgments separately.
