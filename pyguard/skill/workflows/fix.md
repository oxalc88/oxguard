# Fix

1. Get every finding first: if omitted, rerun the same command and scope with `--output json`; use [Diagnose](diagnose.md) only for other gates.
2. Quality findings: fix the reported rule at the reported location. Never lower thresholds or suppress findings.
3. Execution errors: read the referenced diagnostics; restore the tool or config within authorization without editing source. Bundled helpers run privately, so editing project copies repairs nothing; update the binary when authorized.
4. Rerun the failed gate, then the full `check` with the original scope.

Done: `check` exits 0 with a complete assessment, or remaining blockers are reported.
