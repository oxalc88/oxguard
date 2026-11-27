# Opengrep 2.1.0 migration evidence

Measured 2026-11-27 on Linux x64, WSL2 kernel 6.6.87.2, Node 24.18.0, npm 11.16.0 and Go toolchain 1.26.11. Implementation provenance: rounds 0–2 were Codex (OpenAI through the Codex harness) gpt-6.1-sol at effort medium and high (a round-2 xhigh attempt was cancelled without a push) and attempted to reimplement 1.23.0 ignore semantics in Go; each review found new parity gaps. The redesign (OxGuard-owned scope arguments with native Opengrep 2.1.0 `.semgrepignore` support) and its rounds 3 to 5 (approved minified exception, monorepo shim, unchanged exclusion semantics, nested test restoration) were implemented by Claude Sonnet 5.5 through Claude Code, chosen by the owner after repeated review failures on the same issue. The orchestrator supplies an independent reviewer session; this report is implementation evidence, not approval.

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

`python3 scripts/compare-opengrep.py /absolute/v1.23.0/opengrep /absolute/v2.1.0/opengrep` verifies the Linux x64 hashes before executing either engine. It creates each existing security-contract fixture with the unchanged `evals/cases.json` overrides and local `rule.yaml`, copies the rule to `node_modules/.cache/oxguard/rules`, and uses `scan --config node_modules/.cache/oxguard/rules --error --quiet --json` with the same root targets and default directory exclusions; the candidate uses the production Go scope builder (`tsguard/security_scope.go`) through a compiled test helper. The harness asserts equality of `paths.scanned`, findings, errors and native exits for every default-scope case. Network version checks are disabled in both environments. One warmup is discarded, then five wall-clock runs are measured per engine/case in the same project. The 2.1.0 time includes OxGuard's own scope construction (a separate `git rev-parse`, directory listing and ignore-file check, measured as a whole compiled test-helper process per run, 12.2 ms in the last large-tree run, an upper bound because it includes helper start-up). Runs include process startup and ran with no other load on the host; they characterize this host, not production throughput. The direct engine comparison isolates SAST; the installed eval separately checks dependency failure and full security orchestration.

| Case | 1.23.0 median ms | 2.1.0 median ms (incl. OxGuard scope) | Native exits, both | Findings, both |
|---|---:|---:|---|---:|
| security.clean | 2,728.3 | 183.7 | 0 | 0 |
| security.multiple | 2,571.6 | 184.1 | 1 | 2 |
| security.dependency_failure_still_sast | 2,666.9 | 183.7 | 1 | 1 |
| Generated tree, 50,000 `.ts` files in 500 directories plus an excluded `tests/` and `node_modules/` (3 timed runs) | 22,164.3 | 12,143.1 | 1 | 10 |

The five measured milliseconds are: clean baseline `[2728.3, 2775.6, 2653.2, 2767.2, 2672.8]`, candidate `[183.7, 177.7, 184.5, 188.1, 182.3]`; multiple baseline `[2569.5, 2643.3, 2569.0, 2587.7, 2571.6]`, candidate `[212.5, 184.1, 184.6, 181.2, 182.2]`; dependency-failure baseline `[2706.1, 2562.7, 2666.9, 2710.9, 2646.6]`, candidate `[179.0, 181.5, 189.5, 218.8, 183.7]`; large tree baseline `[23283.7, 21999.0, 22164.3]`, candidate `[12143.1, 11907.8, 12390.2]`. All 113 comparison cases pass (security fixtures and default scope identical, every matrix row classified as documented). On the large tree both engines select the same 50,000 files and report the same 10 findings and exit code. No batching is involved: argv holds only the root entries and exclusion patterns, never a file list.

| Case | Rule ID in both engines | File | Line | Column |
|---|---|---|---:|---:|
| security.multiple | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 1 | 23 |
| security.multiple | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 2 | 27 |
| security.dependency_failure_still_sast | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 1 | 23 |

Both engines return empty native errors arrays for these fixtures. An additional deliberately invalid rule exits 7 in both engines; 2.1.0 emits one `InvalidRuleSchemaError` with a message/path/span instead of 1.23.0’s schema error plus aggregate `SemgrepError`. An adapter regression test verifies the 2.1.0 error remains `invalid_configuration`, with raw diagnostic evidence retained; upstream diagnostic wording/count is not a stable contract. Existing normalization consumes unchanged `check_id`, `path`, `start.line/col`, `extra.message/severity` and `errors`; upstream `engine_kind` changes from `OSS` to `Opengrep`, which is outside the OxGuard contract. The unchanged installed known answers verify findings, source positions, diagnostic artifacts, stable IDs, assessment completeness and native/normalized exits. All 31 installed capability cases pass, including the three required security cases. No expectation was weakened and no rule source, blocking policy or interfile flag changed. The original unchanged-scope claim was incorrect: unadapted 2.1.0 changes ignore targeting even with identical root arguments; the scope arguments described below retain the default scope. The [2.1.0 CLI source](https://github.com/opengrep/opengrep/blob/v2.1.0/src/osemgrep/core/CLI_common.ml#L174) makes cross-file analysis opt-in via `--enable-interfile`; OxGuard does not pass it.

## Target scope and `.semgrepignore`

Owner-approved decisions: `.semgrepignore` stays supported as a project configuration and follows Opengrep 2.1.0's native semantics; the default scope (no `.semgrepignore`) stays what 1.23.0 scanned; tsguard exclusion semantics are unchanged from the shipped version; in a Git subproject, a scan-root or Git-root `.semgrepignore` replaces the test defaults for the root entries, while restoring nested test paths is deferred to a follow-up; minified files are never scanned. OxGuard does not parse ignore files, does not write anything into the scanned project and does not disable project ignore files.

### What changed upstream

The [2.1.0 implementation](https://github.com/opengrep/opengrep/blob/v2.1.0/src/targeting/Semgrepignore.ml) replaced the legacy [1.23.0 parser](https://github.com/opengrep/opengrep/blob/v1.23.0/cli/src/semgrep/ignores.py). Reproduced against both pinned engines: it reads `.semgrepignore` files at every depth, implements gitignore syntax (negation and character ranges work, `*` does not cross `/`, no inline comments), reads `:include` with a deprecation notice, and always skips `*.min.js` found while scanning a directory. It ships the same built-in default list as 1.23.0 and applies it only when the project root has no `.semgrepignore` (an empty file also replaces it), but anchors the list at each explicit target. OxGuard scans the root entries as targets (it must never enumerate `node_modules`; Windows pnpm junctions can stall that), so the built-in list never skips `./tests` or `./vendor`: unadapted 2.1.0 newly scans them. Without `--project-root`, outside Git each root target is its own project, so a root `.semgrepignore` is not found at all, and command-line `--exclude` patterns are evaluated relative to each target. Inside Git, the project root is the Git root, so the default-replacing file is the one there, and the built-in list still hides test directories nested below each root entry when only the subproject directory has a `.semgrepignore`.

### How OxGuard builds the scope

`securityScanArgs` in `tsguard/security_scope.go` produces these arguments, followed by the root entries (never `node_modules` or `.git`) as targets:

| Argument | Why |
|---|---|
| `--project-root .` outside Git | One project for every root target, so the root `.semgrepignore` works; Git projects keep Opengrep's native Git root and `.gitignore` handling |
| `--exclude` for tsguard exclusions | Built-in directories, `oxguard.toml` `exclude` and `--exclude`, mapped as described below |
| `--exclude` for the baseline layer | Always applied, outranks every ignore file |
| `--exclude` for the test defaults | Only when no `.semgrepignore` exists at the scan root or at the Git root |
| Root-level `*.min.js` files are not targets | Owner-approved exception: minified files are never scanned. Opengrep exempts explicit root files from every exclusion, so this one filter lives in `securityRootTargets` |

If every root entry is such a minified file, Opengrep is not started (without targets it would scan the working directory) and the security gate records a clean, complete result with no findings and exit 0.

Not used: `--force-exclude` (measured: with it, root *files* named like directory entries (`build`, `dist`, `skills`, `.claude`, `.agents`, `.kiro`, `.next`, `coverage`, `CVS`, `.svn`, `.hg`, `_darcs`) and `*_test.go` stop being scanned, a difference from 1.23.0 beyond minified files), `--x-ignore-semgrepignore-files` (internal, "may change or disappear without notice", and it would remove project ignore files) and `--semgrepignore-filename` with a generated file (writes into the project).

### tsguard exclusions: unchanged from the shipped version

Main passed every entry E as `--exclude E` to 1.23.0, which matches `**/E` and `**/E/**` (wcmatch). Measured against that engine for each form: leading and trailing slashes never anchored anything, so `/src` excluded `src` and `lib/src`, `/a.ts` excluded `src/a.ts` and `lib/a.ts`, and `a/b`, `/a/b`, `a/b/` and `/a/b/` were the same; `./x`, `!x` and `#x` were literals that matched nothing; root-level files (explicit targets) were never excludable. OxGuard maps every entry to `**/` plus the entry stripped of leading and trailing slashes, and `scripts/compare-opengrep.py` asserts identical file sets for `x`, `/x`, `x/`, `/x/`, `a/b`, `/a/b`, `/a/b/`, `/src`, `src/a.ts`, `/a.ts`, `/lib/src`, `*.snap.ts`, `/*.ts`, `!a.ts`, `./x`, `**/x`, `x/*`, below a Git root, and for root files. Main accepted `!` entries (as literals), so they are accepted. Repeated separators collapse the same way (`a//b`, `/a//b/`, `a///b` and `a//b//` all meant `a/b`; measured, rows `exclude.map.double-slash` and following). One owner-approved hardening: an `oxguard.toml` or `--exclude` entry that is empty after stripping slashes (`""`, `/`, `//`) is rejected with an actionable error. Under 1.23.0 it excluded every file below the root entries (explicit root-level files stayed scanned), silently disabling the gate. The CLI already rejected an empty element (`--exclude foo,` fails in both the shipped version and this one with `empty directory in --exclude`); only configuration-file entries, which were not validated, are newly rejected (rows `approved.empty-exclusion.*`).

### Default-list layers

The 1.23.0 default list ([template](https://github.com/opengrep/opengrep/blob/v1.23.0/cli/src/semgrep/templates/.semgrepignore)) is split in one table in the code, with a test that the layers reproduce the template entry for entry:

| Layer | Entries | Project `.semgrepignore` |
|---|---|---|
| Baseline, always applied | `.git`, `.svn`, `.hg`, `_darcs`, `CVS`, `build/`, `vendor/`, `dist/`, `*.min.js`, `.env/`, `.tox/`, `node_modules/`, `.npm/`, `.yarn/`, `.venv/`, `_opam/`, `_build/`, `_cargo/` | Cannot remove them (negations do not re-include them); it can only add exclusions |
| Test defaults, replaceable | `test/`, `tests/`, `testsuite/`, `*_test.go` | A `.semgrepignore` at the scan root (1.23.0's rule) or at the Git root (2.1.0's rule), even empty, replaces them |

The reason for the baseline layer: if a short project file such as `docs/` replaced everything, dependencies, vendored code and minified bundles would silently start being scanned. The intentional differences from 1.23.0 are therefore that projects with a `.semgrepignore` no longer have vendored or build-output files scanned (1.23.0 scanned them once any project file existed), and that minified files are never scanned, root level included (1.23.0 scanned an explicit root `x.min.js`; a project whose only finding is in such a file now passes). Both reduce findings only in non-project code.

### Git subprojects: the shim and the deferred nested-test limit

The test defaults are not supplied when a `.semgrepignore` exists at the scan root or at the Git root, so a subproject's root entries (`tests/`, `test/`) are scanned as under 1.23.0 (rows `monorepo.scan-root-file`, `monorepo.git-root-file`, `monorepo.both-files`, `monorepo.git-root-equals-scan-root`). Opengrep 2.1.0 decides whether its own built-in list applies by looking for `.semgrepignore` at the project root, which inside Git is the Git root, and OxGuard cannot change that. Measured alternatives that do not work: `--project-root <scan root>` inside Git treats the directory as a project without version control, so Git-root and nested `.gitignore` files stop applying and ignored files are scanned; negating the built-ins with `--exclude '!tests/'` does not re-include them; `--semgrepignore-filename` cannot name a path.

**Known limit, deferred by owner decision (rows labelled `approved-deferred`).** When the only `.semgrepignore` is at the scan root of a Git subproject, the built-ins still hide, below each root entry: directories named `test`, `tests` or `testsuite` (`src/tests`), directories named like `*_test.go` (`foo_test.go/`) and files named `*_test.go`. 1.23.0 scanned them, so this is a `less scanned` difference that could hide findings in test code. Everything else in these rows (root entries, hostile names outside test paths, case variants such as `Tests` and `TEST`, which the case-sensitive built-ins never hid) is identical. The workaround is an empty `.semgrepignore` at the Git root: Opengrep then replaces its built-ins natively, and the `workaround.*` rows show that nested test directories, `*_test.go` directories and files, names with control characters, quotes, spaces, globs and leading dashes, symlinked test directories, and test directories ignored by `.gitignore`, by the project `.semgrepignore` or by the baseline at different depths are then identical to 1.23.0 (baseline-excluded ones `approved-reduction`; a path with a carriage return `approved-expansion`). The restoration itself (a listing run plus explicit targets) was removed from this PR after review rounds 5 and 6 found input-handling defects in it; the implementation as of that point is preserved on the branch `feat/opengrep-nested-test-restoration` for a separate follow-up PR. Nothing in this PR depends on it.

Which file wins when both exist: patterns of both are applied (Opengrep reads every `.semgrepignore` from the Git root down) and either one replaces the test defaults; 1.23.0 applied only the scan directory's file.

### Compatibility matrix

`python3 scripts/compare-opengrep.py <1.23.0> <2.1.0> --update-doc docs/opengrep-2.1-migration.md` regenerates the table below from the measurement. It runs every row with both pinned engines (sha256 verified first) and the production argument builder, and fails if a row stops behaving as classified. 1.23.0 uses the pre-migration invocation (`--exclude <entry>` and root targets). Counts are files Opengrep reports as scanned; every file carries one `eval`, so selected files and findings coincide, and each row compares rule ID, file, line, column, message, severity, diagnostics (type, level, message, path), the full finding list (so duplicates show) and the exit code. "More scanned" can newly break builds; "less scanned" could hide findings; The owner-approved exceptions each have their own label and a check that fails when the approved behavior is absent or anything else differs: `approved-reduction` equals the 1.23.0 result minus the listed minified, vendored or build-output files; `approved-tolerance` (an unreadable excluded directory: 1.23.0 exits 2, 2.1.0 skips it and reports no error); `approved-rejection` (an empty or slash-only configuration exclusion is rejected where 1.23.0 excluded everything below the root entries); `approved-expansion` (a path containing a carriage return, which 1.23.0 skipped in a Git work tree, where it read the file list line by line, and 2.1.0 scans; outside Git both engines scan it, row `carriage-return-name-outside-git`: `src/cr<CR>/a.ts` with an `eval` goes from exit 0 to exit 1); `approved-deferred` (the nested test paths of the known limit above, asserted as the 1.23.0 result minus exactly those paths). `both-fail` marks a behavior that must stay a failure in both engines (an unreadable root entry that is not excluded). A self-test proves the checks fail on drift in any compared field. In the no-`.semgrepignore` case the only rows that differ from 1.23.0 are those approved exceptions; nothing else differs there.

Unreadable directories: an unreadable root entry that is not excluded fails in both engines (1.23.0 refuses to start, exit 2; 2.1.0 reports a scan error, which OxGuard turns into an `analyzer_failure` finding); an unreadable directory below a root entry has never failed either engine and still does not (owner decision: "included must fail" applies to top-level root entries only; it is skipped silently in both, row `unreadable.included-nested-dir`); an excluded one is skipped by both when nested and only by 2.1.0 when it is a root entry.

The default-list check compares the two layers with the list embedded in the pinned 2.1.0 binary; the 1.23.0 binary does not embed a readable copy, so its list is verified by the Go test against the template at the cited commit.

<!-- matrix:start (generated by scripts/compare-opengrep.py --update-doc; do not edit by hand) -->
113 cases measured with both engines: 5 approved-deferred, 3 approved-expansion, 7 approved-reduction, 3 approved-rejection, 1 approved-tolerance, 1 both-fail, 2 different, 81 identical, 7 less scanned, 3 more scanned.

| Case | Scenario | 1.23.0 files | 2.1.0 files | Exit 1.23.0 / 2.1.0 | Result |
|---|---|---:|---:|---|---|
| `fixture.security.clean` |  | 1 | 1 | 0 / 0 | identical |
| `fixture.security.multiple` |  | 1 | 1 | 1 / 1 | identical |
| `fixture.security.dependency_failure_still_sast` |  | 1 | 1 | 1 / 1 | identical |
| `default.scope` |  | 12 | 12 | 1 / 1 | identical |
| `default.git` |  | 12 | 12 | 1 / 1 | identical |
| `default.git-gitignored-untracked` |  | 10 | 10 | 1 / 1 | identical |
| `default.git-gitignored-tracked` |  | 12 | 12 | 1 / 1 | identical |
| `default.custom-exclude-any-depth` |  | 11 | 11 | 1 / 1 | identical |
| `default.symlinks` |  | 12 | 12 | 1 / 1 | identical |
| `approved.root-minified` | owner-approved: 1.23.0 scanned an explicit root x.min.js; minified files are never scanned (only 1.23.0: `x.min.js`) | 13 | 12 | 1 / 1 | approved-reduction |
| `approved.root-minified-only-finding` | 1.23.0 exits 1 on the minified file alone; the candidate exits 0 (only 1.23.0: `x.min.js`) | 2 | 1 | 1 / 0 | approved-reduction |
| `approved.root-minified-with-ignore` | (only 1.23.0: `x.min.js`) | 15 | 14 | 1 / 1 | approved-reduction |
| `default.root-level-entries` | root files named like directory entries stay scanned exactly as in 1.23.0; only minified files differ (only 1.23.0: `a.min.js`, `x.min.js`) | 31 | 29 | 1 / 1 | approved-reduction |
| `default.root-level-entries-user-exclude` | explicit root files ignore exclusions in both engines (root files cannot be excluded) | 3 | 3 | 1 / 1 | identical |
| `approved.root-minified-alone` | only a root minified file: no Opengrep run, a clean complete result (1.23.0 exits 1) (only 1.23.0: `x.min.js`) | 1 | 0 | 1 / 0 | approved-reduction |
| `exclude.map.x` | `x` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.slash-x` | `/x` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.x-slash` | `x/` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.slash-x-slash` | `/x/` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.a-b` | `a/b` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.slash-a-b` | `/a/b` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.slash-a-b-slash` | `/a/b/` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.slash-src` | `/src` excludes the same files in both engines | 12 | 12 | 1 / 1 | identical |
| `exclude.map.src-a-ts` | `src/a.ts` excludes the same files in both engines | 18 | 18 | 1 / 1 | identical |
| `exclude.map.slash-a-ts` | `/a.ts` excludes the same files in both engines | 16 | 16 | 1 / 1 | identical |
| `exclude.map.slash-lib-src` | `/lib/src` excludes the same files in both engines | 18 | 18 | 1 / 1 | identical |
| `exclude.map.glob` | `*.snap.ts` excludes the same files in both engines | 18 | 18 | 1 / 1 | identical |
| `exclude.map.slash-glob` | `/*.ts` excludes the same files in both engines | 2 | 2 | 1 / 1 | identical |
| `exclude.map.bang` | `!a.ts` excludes the same files in both engines | 19 | 19 | 1 / 1 | identical |
| `exclude.map.dot-slash` | `./x` excludes the same files in both engines | 19 | 19 | 1 / 1 | identical |
| `exclude.map.double-star` | `**/x` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.children` | `x/*` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.double-slash` | `a//b` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.slash-double-slash` | `/a//b/` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.triple-slash` | `a///b` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.dot-a` | `./a` excludes the same files in both engines | 19 | 19 | 1 / 1 | identical |
| `exclude.map.a-dot-b` | `a/./b` excludes the same files in both engines | 19 | 19 | 1 / 1 | identical |
| `exclude.map.trailing-double` | `a//b//` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.leading-double` | `//a//b` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `approved.empty-exclusion.empty` | owner-approved hardening: `` in oxguard.toml excluded every file below the root entries under 1.23.0 (explicit root files stayed scanned); it is rejected with an actionable error (only 1.23.0: `a.ts`, `x.ts`) | 2 | 0 | 1 / rejected | approved-rejection |
| `approved.empty-exclusion.slash` | owner-approved hardening: `/` in oxguard.toml excluded every file below the root entries under 1.23.0 (explicit root files stayed scanned); it is rejected with an actionable error (only 1.23.0: `a.ts`, `x.ts`) | 2 | 0 | 1 / rejected | approved-rejection |
| `approved.empty-exclusion.double-slash` | owner-approved hardening: `//` in oxguard.toml excluded every file below the root entries under 1.23.0 (explicit root files stayed scanned); it is rejected with an actionable error (only 1.23.0: `a.ts`, `x.ts`) | 2 | 0 | 1 / rejected | approved-rejection |
| `exclude.map.git-subproject-slash-src` | the same mapping below a Git root: no Git-relative translation | 9 | 9 | 1 / 1 | identical |
| `exclude.map.root-file-not-excludable` | root-level explicit files were never excludable in 1.23.0 and still are not | 16 | 16 | 1 / 1 | identical |
| `layer.docs-only-ignore` | tests are scanned as in 1.23.0; minified/vendored/build artifacts stay excluded (1.23.0 scanned them once a project file existed) (only 1.23.0: `.env/e.ts`, `.tox/t.ts`, `.venv/v.ts`, `_build/b.ts`, `src/vendor/v.ts`, `src/x.min.js`, `vendor/v.ts`) | 21 | 14 | 1 / 1 | less scanned |
| `layer.empty-ignore` | an empty project file still releases the test defaults but not the artifacts (only 1.23.0: `.env/e.ts`, `.tox/t.ts`, `.venv/v.ts`, `_build/b.ts`, `src/vendor/v.ts`, `src/x.min.js`, `vendor/v.ts`) | 22 | 15 | 1 / 1 | less scanned |
| `layer.user-exclude-with-ignore` | tsguard --exclude/config entries apply on top of any project file | 13 | 13 | 1 / 1 | identical |
| `layer.ignore-cannot-reinclude` | negations cannot re-include the baseline layer (only 1.23.0: `.env/e.ts`, `.tox/t.ts`, `.venv/v.ts`, `_build/b.ts`, `src/vendor/v.ts`, `src/x.min.js`, `vendor/v.ts`) | 22 | 15 | 1 / 1 | less scanned |
| `approved.unreadable-excluded-dir` | owner-approved: 1.23.0 refuses to start on an unreadable root entry even when excluded (exit 2); 2.1.0 skips the excluded directory and scans the rest (only 2.1.0: `a.ts`, `cache/private/p.ts`, `docs/d.ts`, `lib/l.ts`, `lib/src/y.ts`, `scripts/s.ts`, `src/a.test.ts`, `src/a.ts`, `src/b.ts`, `src/deep/c.ts`, `src/fixtures/f.ts`, `src/gen/g.ts`) | 0 | 12 | 2 / 1 | approved-tolerance |
| `unreadable.included-root-entry` | an unreadable root entry that is NOT excluded keeps failing: 1.23.0 exits 2, 2.1.0 reports a scan error (OxGuard: analyzer_failure) (only 2.1.0: `a.ts`, `cache/private/p.ts`, `docs/d.ts`, `lib/l.ts`, `lib/src/y.ts`, `scripts/s.ts`, `src/a.test.ts`, `src/a.ts`, `src/b.ts`, `src/deep/c.ts`, `src/fixtures/f.ts`, `src/gen/g.ts`) | 0 | 12 | 2 / 1 | both-fail |
| `unreadable.included-nested-dir` | unchanged: an unreadable directory below a root entry has never failed either engine (it is skipped silently in both); only an unreadable root entry fails | 12 | 12 | 1 / 1 | identical |
| `unreadable.excluded-nested-dir` | an unreadable directory below a root entry that IS excluded does not fail either engine | 12 | 12 | 1 / 1 | identical |
| `ignore.root-replaces-defaults` | the test defaults are replaced by the project file in both engines | 14 | 14 | 1 / 1 | identical |
| `ignore.root-empty` | an empty root file also replaces the defaults | 15 | 15 | 1 / 1 | identical |
| `ignore.root-comments-only` |  | 15 | 15 | 1 / 1 | identical |
| `ignore.root-git` |  | 14 | 14 | 1 / 1 | identical |
| `monorepo.no-ignore-files` | defaults apply; Git-root and scan-directory .gitignore honored | 9 | 9 | 1 / 1 | identical |
| `monorepo.scan-root-file` | the shim keeps root entries `tests/` and `test/` scanned as in 1.23.0; the nested `src/tests` stays skipped by Opengrep's built-ins (deferred to a follow-up; workaround: an empty .semgrepignore at the Git root) (only 1.23.0: `src/tests/t.ts`) | 11 | 10 | 1 / 1 | approved-deferred |
| `monorepo.scan-root-file-and-empty-git-root-file` | an empty .semgrepignore at the Git root: native replacement, no restoration needed | 11 | 11 | 1 / 1 | identical |
| `monorepo.git-root-file` | the Git-root file is honored (1.23.0 ignored it) and replaces the test defaults (only 1.23.0: `scripts/s.ts`; only 2.1.0: `src/tests/t.ts`, `test/a.ts`, `tests/a.ts`) | 12 | 14 | 1 / 1 | different |
| `monorepo.both-files` | patterns of both files apply (1.23.0 read only the scan-root file); both replace the defaults (only 1.23.0: `scripts/s.ts`) | 14 | 13 | 1 / 1 | less scanned |
| `monorepo.git-root-equals-scan-root` | unchanged when the scan root is the Git root | 12 | 12 | 1 / 1 | identical |
| `deferred.nested-test-paths` | nested test, tests, testsuite directories and nested *_test.go files stay skipped; root-level entries and other files match 1.23.0 (only 1.23.0: `lib/pkg/b_test.go`, `lib/pkg/test/u.ts`, `lib/pkg/testsuite/s.ts`, `src/a_test.go`, `src/test/u.ts`, `src/tests/c_test.go`, `src/tests/deep/tests/n.ts`, `src/tests/t.ts`, `src/testsuite/s.ts`) | 14 | 5 | 1 / 1 | approved-deferred |
| `deferred.directory-named-test-go` | a directory named like the *_test.go built-in stays skipped natively until the follow-up (only 1.23.0: `src/foo_test.go/a.ts`) | 2 | 1 | 1 / 0 | approved-deferred |
| `deferred.case-variants` | Opengrep's built-ins are case-sensitive: other spellings are scanned natively and need no restoration (only 1.23.0: `lib/pkg/b_test.go`, `lib/pkg/test/u.ts`, `lib/pkg/testsuite/s.ts`, `src/a_test.go`, `src/test/u.ts`, `src/tests/c_test.go`, `src/tests/deep/tests/n.ts`, `src/tests/t.ts`, `src/testsuite/s.ts`) | 19 | 10 | 1 / 1 | approved-deferred |
| `deferred.hostile-names-outside-test-paths` | hostile names outside test paths are scanned natively, identical to 1.23.0 (only 1.23.0: `lib/pkg/b_test.go`, `lib/pkg/test/u.ts`, `lib/pkg/testsuite/s.ts`, `src/a_test.go`, `src/test/u.ts`, `src/tests/c_test.go`, `src/tests/deep/tests/n.ts`, `src/tests/t.ts`, `src/testsuite/s.ts`) | 18 | 9 | 1 / 1 | approved-deferred |
| `workaround.basic` | nested test, tests, testsuite directories and nested *_test.go files are scanned as in 1.23.0 | 14 | 14 | 1 / 1 | identical |
| `workaround.project-ignored-stays-excluded` | a nested test directory and a *_test.go file the project .semgrepignore excludes stay excluded | 14 | 14 | 1 / 1 | identical |
| `workaround.gitignored-stays-excluded` | nested test paths ignored by the Git-root or the scan-directory .gitignore stay excluded | 15 | 15 | 1 / 1 | identical |
| `workaround.deep` | deeply nested directories, test directories inside test directories, nested *_test.go inside them | 17 | 17 | 1 / 1 | identical |
| `workaround.baseline-inside-stays-excluded` | the baseline layer applies inside nested tests (1.23.0 scanned vendor, build and minified files once a project file existed) (only 1.23.0: `src/tests/vendor/v.ts`, `src/tests/x.min.js`) | 16 | 14 | 1 / 1 | approved-reduction |
| `workaround.no-duplicate-reports` | overlapping candidates are reported exactly once: Opengrep skips test directories nested in a target | 16 | 16 | 1 / 1 | identical |
| `workaround.forged-path-newline` | a file named `src/decoy<LF>src/hidden_test.go` is one path: the ignored clean file it imitates stays excluded and the vulnerable file is scanned | 15 | 15 | 1 / 1 | identical |
| `workaround.parent-reference-in-name` | a name containing `<LF>../` is a single file name inside the scan root; nothing outside is added | 15 | 15 | 1 / 1 | identical |
| `workaround.control-characters` | TAB, ESC, VT, LF, U+0085, U+2028 and U+2029 in directory names are one path each and are scanned | 21 | 21 | 1 / 1 | identical |
| `workaround.carriage-return-name` | owner-approved expansion: 1.23.0 drops any path containing a carriage return from the scan (a quirk); 2.1.0 scans it (only 2.1.0: `src/cr\r/tests/m.ts`) | 14 | 15 | 1 / 1 | approved-expansion |
| `approved.carriage-return-name` | owner-approved expansion, measured in a Git work tree: a vulnerable file whose directory name holds a carriage return was invisible to 1.23.0 (exit 0) and is reported by 2.1.0 (exit 1) (only 2.1.0: `src/cr\r/a.ts`) | 1 | 2 | 0 / 1 | approved-expansion |
| `approved.carriage-return-name-in-file` | owner-approved expansion: the same for a carriage return inside a file name (only 2.1.0: `src/a\rb.ts`) | 1 | 2 | 0 / 1 | approved-expansion |
| `carriage-return-name-outside-git` | outside a Git work tree both engines scan paths with a carriage return; only 1.23.0's Git file listing dropped them | 3 | 3 | 1 / 1 | identical |
| `workaround.quotes-spaces-globs-dashes` | quotes, spaces, glob and shell metacharacters and leading dashes are passed as ./-prefixed literal targets | 23 | 23 | 1 / 1 | identical |
| `workaround.directory-named-test-go` | a directory named like the *_test.go built-in is hidden natively and is restored (empty subproject .semgrepignore) | 2 | 2 | 1 / 1 | identical |
| `workaround.directory-named-test-go-deep` | directories matching *_test.go, nested inside each other and holding test files and test directories, are scanned once | 17 | 17 | 1 / 1 | identical |
| `workaround.case-variants` | Opengrep's built-ins are case-sensitive: other spellings are never hidden and never restored | 19 | 19 | 1 / 1 | identical |
| `workaround.symlinked-test-directory` | a symlinked directory is never followed out of the scan root, and a symlink named tests is not a target | 14 | 14 | 1 / 1 | identical |
| `workaround.ignore-depths` | test directories ignored by .gitignore or the project .semgrepignore at different depths, inside or around a test directory, stay excluded | 17 | 17 | 1 / 1 | identical |
| `workaround.baseline-at-depths` | baseline entries (vendor, node_modules, dist, minified files) exclude test directories at any depth around or inside them (only 1.23.0: `src/c/tests/vendor/tests/w.ts`, `src/d/x.min.js`, `src/vendor/tests/v.ts`) | 17 | 14 | 1 / 1 | approved-reduction |
| `workaround.long-path` | a path of about 350 characters is restored | 15 | 15 | 1 / 1 | identical |
| `nested-tests.outside-git` | outside Git --project-root already replaces the defaults: no restoration needed | 14 | 14 | 1 / 1 | identical |
| `include.gitignore` |  | 13 | 13 | 1 / 1 | identical |
| `include.gitignore-git` |  | 13 | 13 | 1 / 1 | identical |
| `include.custom-file` |  | 14 | 14 | 1 / 1 | identical |
| `include.directory` |  | 14 | 14 | 1 / 1 | identical |
| `include.unknown-directive` | (only 2.1.0: `a.ts`, `cache/private/p.ts`, `docs/d.ts`, `lib/l.ts`, `lib/src/y.ts`, `src/a.test.ts`, `src/a.ts`, `src/b.ts`, `src/deep/c.ts`, `src/fixtures/f.ts`, `src/gen/g.ts`, `src/tests/t.ts`, `test/a.ts`, `tests/a.ts`) | 0 | 14 | 2 / 1 | different |
| `include.missing-file` |  | 15 | 15 | 1 / 1 | identical |
| `ignore.scripts-docs` |  | 13 | 13 | 1 / 1 | identical |
| `ignore.scripts-docs-no-slash` |  | 13 | 13 | 1 / 1 | identical |
| `ignore.test-suffix` |  | 14 | 14 | 1 / 1 | identical |
| `ignore.fixtures-globstar` |  | 14 | 14 | 1 / 1 | identical |
| `ignore.anchored-build` |  | 15 | 15 | 1 / 1 | identical |
| `ignore.anchored-vs-unanchored` |  | 14 | 14 | 1 / 1 | identical |
| `ignore.inner-slash-anchors` |  | 14 | 14 | 1 / 1 | identical |
| `ignore.comments-and-blank-lines` |  | 13 | 13 | 1 / 1 | identical |
| `ignore.inline-comment` | gitignore has no inline comments: 1.23.0 stripped ` # ...`, 2.1.0 reads a literal pattern (only 2.1.0: `docs/d.ts`) | 13 | 14 | 1 / 1 | more scanned |
| `ignore.star-crosses-slash` | 1.23.0 fnmatch `*` crossed `/` (hid src/deep/c.ts); gitignore `*` does not (only 2.1.0: `src/deep/c.ts`, `src/fixtures/f.ts`, `src/gen/g.ts`, `src/tests/t.ts`) | 8 | 12 | 1 / 1 | more scanned |
| `ignore.nested-src` | a nested file is now honored; 1.23.0 read only the cwd file (only 1.23.0: `src/a.ts`) | 12 | 11 | 1 / 1 | less scanned |
| `ignore.nested-src-with-root` | (only 1.23.0: `src/a.ts`) | 14 | 13 | 1 / 1 | less scanned |
| `ignore.negation` | negation was silently dropped in 1.23.0 and is honored now (only 2.1.0: `src/b.ts`, `src/deep/c.ts`, `src/fixtures/f.ts`, `src/gen/g.ts`, `src/tests/t.ts`) | 8 | 13 | 1 / 1 | more scanned |
| `ignore.range` | character ranges were silently dropped in 1.23.0 (only 1.23.0: `src/a.ts`, `src/b.ts`) | 15 | 13 | 1 / 1 | less scanned |
| `ignore.double-star-middle` |  | 14 | 14 | 1 / 1 | identical |
| `ignore.trailing-spaces` |  | 14 | 14 | 1 / 1 | identical |
| `ignore.crlf` |  | 13 | 13 | 1 / 1 | identical |
| `ignore.escaped-trailing-space` |  | 15 | 15 | 1 / 1 | identical |
| `ignore.unicode-name` |  | 15 | 15 | 1 / 1 | identical |
<!-- matrix:end -->

### What users may notice

Projects without a `.semgrepignore` see no change except three owner-approved differences: a scan that continues when an excluded root entry is unreadable (1.23.0 exited 2; an unreadable root entry that is not excluded still fails), the rejection of empty or slash-only configuration exclusions, and the reduction that minified files, root-level ones included, are never scanned. Projects with a `.semgrepignore`: nested files are honored (can hide findings); negation and character ranges work (both directions); `*` stays within a directory level; inline comments are not recognised; a Git-root `.semgrepignore` is honored too and, like a scan-directory file, replaces the test defaults; unknown directives are ignored; vendored and build-output files are no longer scanned once a project file exists. To keep an exclusion that no longer comes from a `.semgrepignore`, add it to `oxguard.toml` `exclude` or `--exclude`.

## Verification and evidence reuse

The local gate includes `go vet ./...` and `go test ./...` in contract, tsguard and pyguard; eval and harness regression tests; identity-hook and release-document tests; installed npm distribution/capability/maintainability tests; and agent-action scoring with zero failures and zero stale sessions. ZIP rejection tests cover digest mismatch, malicious paths, symlinks, duplicates, missing files, oversized declarations and extracted executable mismatch. A real standalone setup smoke downloads the upstream host asset, validates cache reuse, executes `--version` and then runs an engine-backed scope check (a Git subproject with no `.semgrepignore`, with a scan-root file and with a Git-root file: `docs/` hidden by the file, `tests/` scanned only when a file replaces the defaults, `vendor/` always excluded); the `npm bundled` jobs run it on Linux, macOS and Windows. The Windows job skips the full `go test ./...` that Linux and macOS run, so it has its own focused step, "Security scope unit tests (Windows)", running the scope tests (layers, exclusion mapping, scope arguments, root targets, empty selection). Everything else the matrix proves (the 113 rows against both pinned engines) runs locally on Linux x64 only: macOS and Windows execution of the matrix, and Windows file-name behavior, are not covered by CI. Existing `scripts/release-smoke.py archive` and `npm` checks passed on local unpublished archives/tarballs. `install.sh` installed the local checksum-verified archive through a temporary curl transport stub and successfully ran the installed CLI; PowerShell execution is left to Windows/platform verification. Skill files and agent trace hashes were not changed.

Local final results: contract 36 passing tests/subtests, tsguard 168, pyguard 252; the normal Go run skips the opt-in network smoke and the comparison-only helper, which `scripts/compare-opengrep.py` runs. The Node harness (`node --test evals/*.test.cjs evals/python/*.test.cjs evals/p0/*.test.cjs scripts/*.test.cjs tsguard/analysis/change-metrics.test.cjs`) has 92 passes, zero failures and one optional real-Python skip (the CI file list: 86 passes, one skip); required npm distribution has 3/3 passes, installed capability 31/31 (including `security.clean`, `security.multiple` and `security.dependency_failure_still_sast`) and installed maintainability 40/40. Identity hooks pass 6/6 and release documentation 4/4; agent scoring reports 0 fail and 0 stale. Go unit tests of the scope builder cover the default layers (reproducing the 1.23.0 template), the exclusion mapping for every slash form and the rejection of empty entries, project-file replacement of only the test layer, nested files, the monorepo cases (no file, scan-root file, Git-root file, both, scan root equal to the Git root), the explicit empty selection and its clean gate record, the root-level minified-file filter (other root files, including ones named `build`/`skills`/`CVS`, stay targets) and that selection writes nothing even into a read-only scan root; the engine behavior (default scope, the `.semgrepignore` matrix, exclusion forms, nested tests, unreadable directories, symlink escape, root-level files) is asserted by `scripts/compare-opengrep.py` against both pinned engines, whose checker has a self-test (`--self-test`, also run at every start) that fails on drift in any compared field. No skill file mentions `.semgrepignore`, so no skill or agent trace changed.

PR #58 reuse is deliberately not claimed. Running its `classify` function on the staged raw diff returns `{ docs_only: false, component: "none" }`, requiring both components. Its classifier permits reuse only for an unambiguous single component. `npm/**` alone is Tsguard-owned under the actual classifier (even native-tools.json); this PR also changes `.github/workflows/npm.yml`, shared `scripts/**`, fixture documentation and root/docs files, so classification requires both components. Both Go component suites are verified locally and CI must produce fresh platform/component evidence. Reuse would require a successful exact-base main push run within 24 hours plus validated artifacts and manifests; the migration does not weaken that requirement.
