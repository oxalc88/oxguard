---
summary: Distribute the existing tsguard Go CLI through platform-specific npm packages.
read_when:
  - Maintaining npm packaging or the release pipeline.
status: done
---

# npm distribution

1. Inspect CLI, installers, version injection, repository identity and documentation. Done.
2. Add a small Node launcher and a single versioned package generator. Done.
3. Verify packed installation, CLI output, errors, exit codes and signals; add CI. Done.
4. Extend releases to publish native packages before the launcher; document local testing and registry setup. Done.

Keep Go behavior and GitHub Release/install-script distribution unchanged. No new analyzers.
The workflow template and git guidance files under ~/projects/WORKFLOW are absent;
use repository conventions. docs-list and a repository docs discovery command are
absent; discovered docs with rg --files and inspected their read_when metadata.

## Verification

- Go tests pass for tsguard and internal/hooks after the module-owner correction.
- Packed distribution integration test passes on macOS arm64: offline tarball
  installation, version/help, actual Go exit code 3, native fixture exit codes
  0/1/3/4/5/37, argument and stream forwarding, Unix signals, npm platform
  rejection, unsupported platform, missing package and mismatched version.
- All five native targets cross-compiled with CGO disabled; all six packages
  packed successfully with version 0.0.0-local in ignored dist/npm/.
- actionlint, JavaScript syntax checks, Go formatting and git diff --check pass.
- Linux/macOS/Windows integration execution is configured in CI; only macOS
  arm64 was executed locally. No registry publication was performed.

Release setup requires npm scope ownership and the NPM_TOKEN Actions secret.
See docs/guides/npm-distribution.md for token setup and partial-publish recovery.

## pnpm verification follow-up

1. Test packed installation under pnpm's isolated dependency layout, including
   transitive optional native dependency resolution and exit-code forwarding. Done.
2. Add pnpm execution to the distribution CI and document verified versions. Done.
3. Run npm regression and workflow checks, then commit the follow-up. Done.

Packed installation tests pass locally on macOS arm64 with npm 11.16.0 and
pnpm 10.34.5/12.6.0. Both pnpm versions resolve the native package transitively
with scripts disabled, run version/help, forward native exit codes 3 and 37
through pnpm exec, and pass the launcher stream/signal/error checks. Native
manifests contain no analyzer dependencies or install scripts, and no Opengrep
cache is created. Workflow lint and JavaScript syntax/whitespace checks pass.
CI covers all three managers on Linux, macOS and Windows. No publishing or
global package-manager configuration changes were made.
