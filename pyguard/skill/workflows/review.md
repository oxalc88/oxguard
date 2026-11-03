# Maintainability

Use requested scope and `--output json`:

- `criticality`: advisory static callers, not runtime impact; default `functions/cdk`, overridden by `--dirs`.
- `audit`: advisory criticality, dead code, dependencies and structure.
- `structure`: advisory runtime module/function graphs, cycles, coupling, sync/async delegation and exception-handler smells. Review forwarding boundaries and recovery policies. Outside `check`.
- `radon`: blocking CC/MI/Halstead; cognitive complexity above 20 (20 clean) is advisory: `pass` becomes `advisory`, exit code and blocking outcome unchanged. `complexity_context` (nesting/locations/call edges in `.pyguard-cache/reports/complexity-context/maintainability-graph.json`) is informational, even when limits block. Inspect `partial`/`skipped`/`graph_error`; unknown isn't zero.
- `change --baseline <ref>`: explicit baseline required; advisory structural/Radon/matched-function cognitive comparison. Complete: exit 0 with advisories. Unavailable baseline, incomplete inputs/metrics or missing/suppressed cognitive scores: incomplete, exit 1. Inspect assessment/gates/diagnostics and `.pyguard-cache/maintainability-change.json`. `check --baseline <ref>` adds advisory change without altering blocking outcome/exit.

Separate findings/completeness: structure can exit 0 while incomplete. Analyses, smaller files and lower scores never prove maintainability; inspect behavior.

`pyguard_maintainability_ready`: complete supported Python scope, not tsguard parity.

Report blockers, advisories, gaps, judgments, change capability ids/outcomes.
