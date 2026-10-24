# Fix findings

Read [Check](check.md) first. Retrieve omitted findings before edits.

Failing workflow: `diagnose` → plan from all findings and `analyses` → repair → rerun the failed gate, then full `check`. Restore errored analyses within authorization.

For quality findings, fix the reported rule and location. Preserve project architecture, validation and recovery policy. Do not lower thresholds or suppress findings to obtain a pass.

For execution errors, inspect diagnostics and restore the analyzer or configuration within authorization. Use the project's package manager; keep source unchanged during tool repair. Bundled Python helpers execute privately; editing project reference copies cannot repair them. Update the compatible binary when authorized.

Rerun the failed gate, then the full requested command with its original scope. Verify complete results before claiming success. `fix`, `setup` and installation require authorization for those operations.
