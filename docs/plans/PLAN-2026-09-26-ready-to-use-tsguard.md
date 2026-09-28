---
summary: Make npm/pnpm installation sufficient to run the complete existing tsguard toolchain.
read_when:
  - Implementing a self-contained npm distribution or evaluating a TypeScript port.
status: done
---

# Ready-to-use tsguard

## Objective

After installing @oxguard/tsguard, users can invoke every existing analysis
command without separately installing tsguard's implementation tools or running
setup to obtain them. Analysis findings, missing project tests, configuration
errors and vulnerabilities must still produce honest failures. This does not
mean every arbitrary project's check passes.

Work is isolated on feat/tsguard-ready-to-use, branched from the completed npm
distribution work. The earlier distribution branch remains available.

## Initial assessment (done)

- The prior npm package only distributed the engine.
- Go targets use package-manager exec to resolve project-installed tools.
- Coverage discovers a runner from the project's dependencies/configuration.
- Opengrep and rules are resolved from the project's oxguard cache; missing
  Opengrep silently skips SAST today.
- Lint and secrets need usable configuration as well as installed executables.
- Existing setup adds implementation tools directly to the consuming manifest,
  downloads Opengrep, and offers hooks. It is not suitable as an automatic
  npm postinstall under the original package-ownership constraints.

The smaller approach is to retain Go and distribute package-owned tools and
defaults, with explicit resolution in Go. Rewriting the engine in TypeScript
would additionally require porting subprocess handling, timeout/signal behavior,
locking, configuration, runner discovery, setup, hooks, and existing tests.
Changing language alone does not solve tool availability or default configs.

## Architecture decision

The user selected retaining Go with the complete packaged toolchain. npm owns
the pinned analyzers; Go owns command dispatch and gate behavior. This expands
the earlier distribution-only scope. Preserve standalone CLI behavior when no
npm runtime is supplied.

The latest objective requests all capabilities from installation. Include SAST
availability in the implementation design; the earlier preference to leave
Opengrep optional must be reconciled rather than silently skipping its gate.

## Stages

1. Decide architecture and how the complete toolchain, including SAST, is shipped.
2. Pin required tools, bundle safe fallback configs and implement package-relative
   resolution. Preserve explicit project configs and package-manager detection.
3. Make doctor accurately report package-owned tool availability and keep
   setup focused on explicitly requested project integration.
4. Test real analysis commands after installing tarballs with npm and pnpm,
   including projects without tsguard analyzer devDependencies. The unpublished
   npm fixture supplies both local tarballs; pnpm installs only the main tarball
   and resolves the native package through a temporary local override.
5. Verify platform compatibility, release artifacts, installation integrity,
   exit codes and unmodified consuming manifests; document and commit stages.

Package install must not run setup or add analyzers as consuming-project direct
dependencies. Provide project configuration fallbacks without overwriting the
project's own files. Build-time downloads are pinned and checksum-verified;
installations do not download Opengrep through a lifecycle script.

## Completion evidence

- The main package owns exact analyzer pins and fallback configs. Five native
  packages include the Go CLI and pinned, checksum-verified Opengrep engines;
  Linux packages include glibc and musl builds.
- `go test ./...` passes for the standalone and packaged runtime paths. The
  npm packed-install test passes with npm, pnpm 10.34.5 and pnpm 12.6.0. It
  exercises the full check pipeline, negative type/SAST results, argument and
  exit-code forwarding, and unchanged consumer manifests.
- All five Go targets cross-compiled and packed locally. The Windows process
  tree change cross-compiled; runtime execution on Linux and Windows remains
  covered by the GitHub Actions matrix after push. Workflow validation and
  JavaScript syntax checks pass.
- The existing GitHub Release/install-script path remains unchanged. npm
  publication is not performed locally and still requires registry setup.

Remote Opengrep rule sets and CVE feeds remain network-dependent. Direct tool
versions are exact; consuming lockfiles record transitive resolutions.
