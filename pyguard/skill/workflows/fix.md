# Fix

If findings are omitted, first rerun the same command and scope with `--output json`; use Diagnose only for findings outside that command.

1. Get all findings first via [Diagnose](diagnose.md).
2. Quality findings: fix the reported rule at the reported location. Never lower thresholds or suppress findings.
3. Execution errors: read the referenced diagnostics; restore the tool or config within authorization without editing source. Bundled Python helpers run privately; editing project copies does not repair them. Update the binary when authorized.
4. Rerun the failed gate, then the full `check` with the original scope.

Done: `check` exits 0 with a complete assessment, or remaining blockers are reported.
