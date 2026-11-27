# Opengrep 2.1.0 migration evidence

Measured 2026-11-27 on Linux x64, WSL2 kernel 6.6.87.2, Node 24.18.0, npm 11.16.0 and Go toolchain 1.26.11. Implementation provenance: rounds 0–2 were Codex (OpenAI through the Codex harness) gpt-6.1-sol at effort medium and high (a round-2 xhigh attempt was cancelled without a push) and attempted to reimplement 1.23.0 ignore semantics in Go; each review found new parity gaps. This redesign round (OxGuard-owned scope arguments with native Opengrep 2.1.0 `.semgrepignore` support) was implemented by Claude Sonnet 5.5 through Claude Code, chosen by the owner after repeated review failures on the same issue. The orchestrator supplies an independent reviewer session; this report is implementation evidence, not approval.

## Sources and integrity

Inspected the [2.1.0 release notes and asset list](https://github.com/opengrep/opengrep/releases/tag/v2.1.0), [manual installation documentation](https://github.com/opengrep/opengrep/blob/v2.1.0/README.md#manual-install), [targeting implementation](https://github.com/opengrep/opengrep/blob/v2.1.0/src/targeting/Semgrepignore.ml), [CLI implementation](https://github.com/opengrep/opengrep/blob/v2.1.0/src/osemgrep/cli_scan/Scan_CLI.ml) and [Windows build/package workflow](https://github.com/opengrep/opengrep/blob/v2.1.0/.github/workflows/build-test-windows-x86.yml). `gh release view v2.1.0 -R opengrep/opengrep --json assets` publishes SHA-256 asset digests; no separate checksum-file asset is published. All five release payloads were downloaded into separate empty directories outside the repository, hashed locally and matched to those upstream digests before use. Windows extracted-file hashes were computed from that verified ZIP. The manifest records every digest and size; the original LGPL license hash remains valid. Signatures/certificates are published upstream but were not independently authenticated here.

`npm/native-tools.json` is authoritative. Run `node npm/sync-native.cjs` after updating it; Go embeds the generated copy and tests byte equality. Neither installers nor Go carry a separate hand-maintained version/hash table. Standalone installers still install the OxGuard CLI; its `setup` downloads/verifies the engine into a versioned OS user cache. npm fetches engines only at build time and ships license/source notices. No install lifecycle script was added.

## Compatibility and size

| Platform | 1.23.0 upstream bytes | 2.1.0 upstream bytes | Installed engine layout |
|---|---:|---:|---|
| Linux x64 | 41,612,596 glibc + 43,956,041 musl | 161,695,104 | One static `opengrep` |
| Linux arm64 | 43,242,485 glibc + 45,466,932 musl | 165,911,416 | One static `opengrep` |
| macOS x64 | 43,843,472 | 167,123,072 | `opengrep` |
| macOS arm64 | 43,236,128 | 175,536,424 | `opengrep` |
| Windows x64 | 48,520,704 EXE | 44,150,190 ZIP | 204,674,378-byte EXE + 4,428,886 bytes of DLLs |

Linux assets are now `opengrep_linux_x86` and `opengrep_linux_aarch64`. `file` identifies both as statically linked ELF executables; `readelf -l` shows no interpreter segment, `readelf -d` shows no dynamic section, and host `ldd` reports “not a dynamic executable.” This removes a libc loader dependency and supports retaining glibc/musl with the same binary. No musl platform is dropped. Docker and Podman are unavailable, so this is linkage evidence rather than an Alpine runtime execution; arm64 and macOS execution require their platform CI/runners.

The Windows ZIP contains exactly `opengrep.exe`, `libgcc_s_seh-1.dll`, `libgmp-10.dll`, `libpcre2-8-0.dll`, `libstdc++-6.dll`, `libwinpthread-1.dll` and `libzstd-1.dll`. `objdump -p` on the EXE and all DLLs confirms that the non-system import closure requires those six DLLs (including the indirect libgcc/libwinpthread dependencies). They must remain adjacent. Both npm packaging and standalone setup verify archive SHA-256 before parsing/extraction, allow only exact flat regular entries, reject symlinks/traversal/duplicates/unexpected or missing files, enforce a 220 MiB total ceiling plus individual pinned sizes, and verify every extracted hash. Go verifies the executable and all DLLs before each SAST run, including npm mode. A project file or environment variable cannot authorize an unpinned substitute executable.

A controlled Linux x64 native npm package comparison holds the Go binary, metadata and documentation fixed: the refreshed local pack with the compatibility implementation holds the current Go binary equal on both sides: the old glibc+musl payload packs to 91,423,597 bytes, while the new single static payload packs to 38,857,962 bytes. Expanded engine bytes grow from 85,568,637 to 161,695,104. A gzip archive of just the x64 glibc executable shrinks from 41,481,697 to 32,191,282 bytes. These are local pack measurements, not published package sizes. Allow additional package/cache/extraction disk space despite the smaller compressed package.

## Equivalent-rule detection and timing

`python3 scripts/compare-opengrep.py /absolute/v1.23.0/opengrep /absolute/v2.1.0/opengrep` verifies the Linux x64 hashes before executing either engine. It creates each existing security-contract fixture with the unchanged `evals/cases.json` overrides and local `rule.yaml`, copies the rule to `node_modules/.cache/oxguard/rules`, and uses `scan --config node_modules/.cache/oxguard/rules --error --quiet --json` with the same root targets and default directory exclusions; the candidate uses the production Go scope builder (`tsguard/security_scope.go`) through a compiled test helper. The harness asserts equality of `paths.scanned`, findings, errors and native exits for every default-scope case. Network version checks are disabled in both environments. One warmup is discarded, then five wall-clock runs are measured per engine/case in the same project. The 2.1.0 time includes OxGuard's own scope construction (a separate `git rev-parse`, directory listing and ignore-file check, measured as a whole compiled test-helper process per run, 8.3 ms in the last large-tree run, an upper bound because it includes helper start-up). Runs include process startup and ran with no other load on the host; they characterize this host, not production throughput. The direct engine comparison isolates SAST; the installed eval separately checks dependency failure and full security orchestration.

| Case | 1.23.0 median ms | 2.1.0 median ms (incl. OxGuard scope) | Native exits, both | Findings, both |
|---|---:|---:|---|---:|
| security.clean | 2,573.3 | 176.2 | 0 | 0 |
| security.multiple | 2,591.7 | 183.6 | 1 | 2 |
| security.dependency_failure_still_sast | 2,716.2 | 183.6 | 1 | 1 |
| Generated tree, 50,000 `.ts` files in 500 directories plus an excluded `tests/` and `node_modules/` (3 timed runs) | 22,780.7 | 12,497.1 | 1 | 10 |

The five measured milliseconds are: clean baseline `[2646.4, 2555.7, 2739.4, 2573.3, 2562.6]`, candidate `[199.5, 171.7, 170.1, 176.5, 176.2]`; multiple baseline `[2577.0, 2589.8, 2636.1, 2642.4, 2591.7]`, candidate `[181.5, 199.4, 182.6, 183.6, 183.6]`; dependency-failure baseline `[2716.2, 2878.7, 2721.7, 2569.5, 2590.8]`, candidate `[212.8, 198.0, 180.7, 183.6, 181.5]`; large tree baseline `[22657.8, 22916.9, 22780.7]`, candidate `[12035.4, 12497.1, 13899.6]`. All 57 comparison cases pass (security fixtures and default scope identical, every matrix row classified as documented). On the large tree both engines select the same 50,000 files and report the same 10 findings and exit code. No batching is involved: argv holds only the root entries and exclusion patterns, never a file list.

| Case | Rule ID in both engines | File | Line | Column |
|---|---|---|---:|---:|
| security.multiple | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 1 | 23 |
| security.multiple | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 2 | 27 |
| security.dependency_failure_still_sast | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 1 | 23 |

Both engines return empty native errors arrays for these fixtures. An additional deliberately invalid rule exits 7 in both engines; 2.1.0 emits one `InvalidRuleSchemaError` with a message/path/span instead of 1.23.0’s schema error plus aggregate `SemgrepError`. An adapter regression test verifies the 2.1.0 error remains `invalid_configuration`, with raw diagnostic evidence retained; upstream diagnostic wording/count is not a stable contract. Existing normalization consumes unchanged `check_id`, `path`, `start.line/col`, `extra.message/severity` and `errors`; upstream `engine_kind` changes from `OSS` to `Opengrep`, which is outside the OxGuard contract. The unchanged installed known answers verify findings, source positions, diagnostic artifacts, stable IDs, assessment completeness and native/normalized exits. All 31 installed capability cases pass, including the three required security cases. No expectation was weakened and no rule source, blocking policy or interfile flag changed. The original unchanged-scope claim was incorrect: unadapted 2.1.0 changes ignore targeting even with identical root arguments; the scope arguments described below retain the default scope. The [2.1.0 CLI source](https://github.com/opengrep/opengrep/blob/v2.1.0/src/osemgrep/core/CLI_common.ml#L174) makes cross-file analysis opt-in via `--enable-interfile`; OxGuard does not pass it.

## Target scope and `.semgrepignore`

Owner-approved decision: `.semgrepignore` stays supported as a project configuration and follows Opengrep 2.1.0's native semantics; the default scope (no `.semgrepignore`) stays what 1.23.0 scanned. OxGuard does not parse ignore files, does not write anything into the scanned project and does not disable project ignore files.

### What changed upstream

The [2.1.0 implementation](https://github.com/opengrep/opengrep/blob/v2.1.0/src/targeting/Semgrepignore.ml) replaced the legacy [1.23.0 parser](https://github.com/opengrep/opengrep/blob/v1.23.0/cli/src/semgrep/ignores.py). Reproduced against both pinned engines: it reads `.semgrepignore` files at every depth, implements gitignore syntax (negation and character ranges work, `*` does not cross `/`, no inline comments), reads `:include` with a deprecation notice, and always skips `*.min.js` found while scanning a directory. It ships the same built-in default list as 1.23.0 and applies it only when the project root has no `.semgrepignore` (an empty file also replaces it), but anchors the list at each explicit target. OxGuard scans the root entries as targets (it must never enumerate `node_modules`; Windows pnpm junctions can stall that), so the built-in list never skips `./tests` or `./vendor`: unadapted 2.1.0 newly scans them. Without `--project-root`, outside Git each root target is its own project, so a root `.semgrepignore` is not found at all, and command-line `--exclude` patterns are evaluated relative to each target.

### How OxGuard builds the scope

`securityScopeArgs` in `tsguard/security_scope.go` is read-only and produces these arguments, followed by the root entries (never `node_modules` or `.git`) as targets:

| Argument | Why |
|---|---|
| `--project-root .` outside Git | One project for every root target, so the root `.semgrepignore` and anchored patterns work; Git projects keep Opengrep's native Git root and `.gitignore` handling |
| `--exclude` for tsguard exclusions | Built-in directories, `oxguard.toml` `exclude` and `--exclude`; `/path` is anchored at the scan root (translated to the Git-relative path below a Git root), other entries match at any depth, negation is rejected |
| `--exclude` for the baseline layer | Always applied, outranks every ignore file |
| `--exclude` for the test defaults | Only when no `.semgrepignore` exists at the scan root or at the Git root |
| Root-level `*.min.js` files are not targets | Owner-approved exception: minified files are never scanned. Opengrep exempts explicit root files from every exclusion, so this one filter lives in `securityRootTargets` |

Not used: `--force-exclude` (measured: with it, root *files* named like directory entries (`build`, `dist`, `skills`, `.claude`, `.agents`, `.kiro`, `.next`, `coverage`, `CVS`, `.svn`, `.hg`, `_darcs`) and `*_test.go` stop being scanned, a difference from 1.23.0 beyond minified files), `--x-ignore-semgrepignore-files` (internal, "may change or disappear without notice", and it would remove project ignore files), `--semgrepignore-filename` with a generated file (writes into the project), and explicit file lists with batching (2.1.0 has no target-list-file option, and none is needed).

### Default-list layers

The 1.23.0 default list ([template](https://github.com/opengrep/opengrep/blob/v1.23.0/cli/src/semgrep/templates/.semgrepignore)) is split in one table in the code, with a test that the layers reproduce the template entry for entry:

| Layer | Entries | Project `.semgrepignore` |
|---|---|---|
| Baseline, always applied | `.git`, `.svn`, `.hg`, `_darcs`, `CVS`, `build/`, `vendor/`, `dist/`, `*.min.js`, `.env/`, `.tox/`, `node_modules/`, `.npm/`, `.yarn/`, `.venv/`, `_opam/`, `_build/`, `_cargo/` | Cannot remove them (negations do not re-include them); it can only add exclusions |
| Test defaults, replaceable | `test/`, `tests/`, `testsuite/`, `*_test.go` | A `.semgrepignore` at the scan root (1.23.0's rule) or at the Git root (2.1.0's rule), even empty, replaces them |

The reason for the baseline layer: if a short project file such as `docs/` replaced everything, dependencies, vendored code and minified bundles would silently start being scanned. The one intentional difference from 1.23.0 is therefore that projects with a `.semgrepignore` no longer have minified, vendored or build-output files scanned (1.23.0 scanned them once any project file existed): fewer findings, only in non-project code.

### Compatibility matrix

`python3 scripts/compare-opengrep.py <1.23.0> <2.1.0> --markdown` runs every row with both pinned engines (sha256 verified first) and the production argument builder, and fails if a row stops behaving as classified here. 1.23.0 uses the pre-migration invocation (`--exclude <dir>` and root targets). Counts are files Opengrep reports as scanned; every file carries one `eval`, so selected files and findings coincide, and the harness also compares rule ID, file, line, column, diagnostics and exit code. "More scanned" can newly break builds; "less scanned" could hide findings.

| Group | Scenario | 1.23.0 / 2.1.0 files | Result |
|---|---|---|---|
| Default scope (no `.semgrepignore`) | Security eval fixtures: `security.clean`, `security.multiple`, `security.dependency_failure_still_sast` | 1/1 each, exits 0/0, 1/1, 1/1 | identical |
| | Default dirs (`tests/`, `test/`, `testsuite/`, `src/tests/`, `vendor/`, `build/`, `.venv/`, `.tox/`, `.env/`, `_build/`, `*.min.js`, dependencies) | 12/12 | identical |
| | Git: default, `.gitignore`d untracked file, `.gitignore`d tracked file | 12/12, 10/10, 12/12 | identical |
| | Custom exclusions at any depth, symlinks (escape through a symlinked directory or file) | 11/11, 12/12 | identical |
| a. Defaults with a project file | Root `.semgrepignore` with `scripts/` (test dirs scanned in both), empty file, comments only, Git project | 14/14, 15/15, 15/15, 14/14 | identical |
| | `docs/` only, plus minified, vendored, `.venv`, `.tox`, `.env`, `_build` artifacts | 21/14 | less scanned: artifacts excluded, tests still scanned |
| | Negations in the project file cannot re-include `vendor/`, `*.min.js` | 22/15 | less scanned (artifacts only) |
| b. `:include` | `:include .gitignore` (plain and Git), `:include custom.ignore`, `:include` of a directory, `:include` of a missing file | 13/13, 13/13, 14/14, 14/14, 15/15 | identical |
| | Unknown `:` directive (`:unknown x`) | exit 2 / 14 files | different: 1.23.0 aborts with an error, 2.1.0 ignores the line |
| c. Typical files | `scripts/` + `docs/`, with and without trailing slash, `*.test.ts`, `**/fixtures/`, `/build`, `/b.ts` + `c.ts`, `src/gen` (inner slash anchors), `src/**/c.ts`, comments and blank lines | 13/13 … 15/15 | identical |
| | `src/*.ts` | 8/12 | more scanned: `*` no longer crosses `/` (hid `src/deep/c.ts` and `src/gen/`, `src/tests/`, `src/fixtures/` files) |
| | `docs/ # trailing comment` | 13/14 | more scanned: there is no inline comment, the pattern is a literal |
| d. Nested files | `src/.semgrepignore` listing `a.ts`, with and without a root file | 12/11, 14/13 | less scanned: `src/a.ts` is now hidden, as the file's author intended |
| e. Syntax | `src/*.ts` + `!src/b.ts` | 8/13 | more scanned: negation honored (dropped in 1.23.0) |
| | `src/[a-b].ts` | 15/13 | less scanned: ranges honored (dropped in 1.23.0) |
| | Trailing spaces, CRLF endings, escaped trailing space, Unicode file names | 14/14, 13/13, 15/15, 15/15 | identical |
| OxGuard exclusions | `/src` anchored, also from a Git subproject | 5/6 | more scanned: only the scan root's `src` is excluded; 1.23.0 also excluded `lib/src` |
| | `/a.ts` anchored root file | 11/11 | different: only the root `a.ts` is excluded (1.23.0 excluded `src/a.ts` and scanned the explicit root file) |
| Root-level files | Root files named like every entry type (`build`, `dist`, `vendor`, `CVS`, `.env`, `skills`, `.claude`, `.agents`, `.kiro`, `.next`, `coverage`, `testsuite`, `.hg`, `.svn`, `_darcs`, `.venv`, `.tox`, `.npm`, `.yarn`, `_opam`, `_build`, `_cargo`, `foo_test.go`, `README.md`, `Makefile`, `a.ts`) with no `.semgrepignore`, scanned by a language-agnostic rule | 31/29 | identical except the two `*.min.js` files: approved-reduction |
| | `exclude = ["generated.ts", "/a.ts"]` naming root files | 3/3 | identical: explicit root files ignore exclusions in both engines |
| | `/a.ts` anchored | 11/12 | more scanned: `src/a.ts` is no longer hidden (1.23.0 treated it as any depth) |
| **Approved reduction** (owner decision, round-3 review) | Root `x.min.js` with an `eval`, with and without a project file; a project whose only finding is in a root `x.min.js` | 13/12, 15/14; 2/1 files, exit 1 / 0 | approved-reduction: minified files are never scanned, root included. The harness gives it its own label and fails if the candidate differs from the baseline by anything else |
| Monorepo (scan root `packages/app` below the Git root) | No ignore files; `.gitignore` at the Git root (`gen/`) and in the scan directory (`lib/`) | 10/10 | identical: both `.gitignore` files honored, defaults apply |
| | `.semgrepignore` (`docs/`) only in the scan directory | 12/11 | less scanned: `tests/` and `test/` scanned as in 1.23.0 (OxGuard shim), but `src/tests` stays skipped by Opengrep's native built-ins |
| | Same, plus an empty `.semgrepignore` at the Git root (the workaround) | 12/12 | identical |
| | `.semgrepignore` only at the Git root (`scripts/`) | 12/14 | different: the Git-root file is honored (1.23.0 ignored it) and replaces the test defaults |
| | Files at both levels (`docs/` and `scripts/`) | 14/13 | less scanned: patterns of both files apply, 1.23.0 read only the scan directory's |
| | Scan root equal to the Git root | 12/12 | identical |
| | Excluded directory with no read permission | exit 2 / 12 files | different: 1.23.0 refuses to start on an unreadable root entry; 2.1.0 skips the excluded directory and reports unexcluded unreadable paths as scan errors |

Monorepo shim and its limit: Opengrep 2.1.0 decides whether its built-in default list applies by looking for `.semgrepignore` at the project root, which inside Git is the Git root. OxGuard cannot change that without losing `.gitignore` handling, so it adds what flags allow: the test defaults are not supplied when the scan root has a file, which restores `./tests` and `./test` as in 1.23.0 (the reviewer's reproduction). Measured alternatives: `--project-root <scan root>` inside Git makes Opengrep treat the directory as a project without version control, so Git-root and nested `.gitignore` files (`gen/`, `lib/` above) stop applying and ignored files are scanned; negating the built-ins with `--exclude '!tests/'` does not re-include them; `--semgrepignore-filename` cannot name a path. Remaining difference: with the file only in the scan directory, built-in test paths still skip nested test directories below root entries (`src/tests`). Workaround: add an empty `.semgrepignore` at the Git root (row above). Closing the gap without that file would need either a generated ignore file (written into the project) or OxGuard-side enumeration of nested test directories (a directory walk plus `git check-ignore`), which were rejected as a new parser or project writes. Which file wins when both exist: patterns of both are applied (Opengrep reads every `.semgrepignore` from the Git root down), and either one replaces the test defaults; 1.23.0 applied only the scan directory's file. Upstream marks the `:include` directive deprecated, so a future engine upgrade must re-run the matrix. No common usage tested required more than this shim.

### What users may notice

Projects without a `.semgrepignore` see no change except exclusion semantics (`/path` anchoring), a tolerant scan when an excluded directory is unreadable, and the approved reduction that minified files, root-level ones included, are never scanned (a project whose only finding was in a root `x.min.js` now passes). Projects with a `.semgrepignore`: nested files are honored (can hide findings); negation and character ranges work (both directions); `*` stays within a directory level; inline comments are not recognised; a Git-root `.semgrepignore` is honored too and, like a scan-directory file, replaces the test defaults (nested test directories below root entries are still skipped by Opengrep unless the Git root has a file); unknown directives are ignored; minified, vendored and build-output files are no longer scanned. To keep an exclusion that no longer comes from a `.semgrepignore`, add it to `oxguard.toml` `exclude` or `--exclude`.

## Verification and evidence reuse

The local gate includes `go vet ./...` and `go test ./...` in contract, tsguard and pyguard; eval and harness regression tests; identity-hook and release-document tests; installed npm distribution/capability/maintainability tests; and agent-action scoring with zero failures and zero stale sessions. ZIP rejection tests cover digest mismatch, malicious paths, symlinks, duplicates, missing files, oversized declarations and extracted executable mismatch. A real standalone setup smoke downloads the upstream host asset, validates cache reuse and executes `--version`; CI runs it on Linux, macOS and Windows. Existing `scripts/release-smoke.py archive` and `npm` checks passed on local unpublished archives/tarballs. `install.sh` installed the local checksum-verified archive through a temporary curl transport stub and successfully ran the installed CLI; PowerShell execution is left to Windows/platform verification. Skill files and agent trace hashes were not changed.

Local final results: contract 36 passing tests/subtests, tsguard 166, pyguard 252; the normal Go run skips the opt-in network smoke and the comparison-only helper, which `scripts/compare-opengrep.py` runs. The Node harness (`node --test evals/*.test.cjs evals/python/*.test.cjs evals/p0/*.test.cjs scripts/*.test.cjs tsguard/analysis/change-metrics.test.cjs`) has 92 passes, zero failures and one optional real-Python skip (the CI file list: 86 passes, one skip); required npm distribution has 3/3 passes, installed capability 31/31 (including `security.clean`, `security.multiple` and `security.dependency_failure_still_sast`) and installed maintainability 40/40. Identity hooks pass 6/6 and release documentation 3/3; agent scoring reports 0 fail and 0 stale. Go unit tests of the scope builder cover the default layers (reproducing the 1.23.0 template), project-file replacement of only the test layer, nested files, the monorepo shim (no file, scan-root file, Git-root file, both files, scan root equal to the Git root), Git subproject anchoring, anchored `/src`, invalid exclusions, the root-level minified-file filter (other root files, including ones named `build`/`skills`/`CVS`, stay targets) and that selection writes nothing even into a read-only scan root; the engine behavior (default scope, `.semgrepignore` matrix, unreadable excluded directory, symlink escape, root-level files) is asserted by `scripts/compare-opengrep.py` against both pinned engines. No skill file mentions `.semgrepignore`, so no skill or agent trace changed.

PR #58 reuse is deliberately not claimed. Running its `classify` function on the staged raw diff returns `{ docs_only: false, component: "none" }`, requiring both components. Its classifier permits reuse only for an unambiguous single component. `npm/**` alone is Tsguard-owned under the actual classifier (even native-tools.json); this PR also changes `.github/workflows/npm.yml`, shared `scripts/**`, fixture documentation and root/docs files, so classification requires both components. Both Go component suites are verified locally and CI must produce fresh platform/component evidence. Reuse would require a successful exact-base main push run within 24 hours plus validated artifacts and manifests; the migration does not weaken that requirement.
