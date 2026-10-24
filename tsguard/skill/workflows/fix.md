# Fix

1. Get all findings first via [Diagnose](diagnose.md).
2. Quality findings: fix the reported rule at the reported location. Never lower thresholds or suppress findings.
3. Execution errors: read the referenced diagnostics; restore the tool or config within authorization without editing source. For missing npm-owned Opengrep, reinstall with optional dependencies. Keep the project compiler; TypeScript 7 is unsupported.
4. Rerun the failed gate, then the full `check` with the original scope.

Done: `check` exits 0 with a complete assessment, or remaining blockers are reported.
