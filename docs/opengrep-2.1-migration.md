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

`python3 scripts/compare-opengrep.py /absolute/v1.23.0/opengrep /absolute/v2.1.0/opengrep` verifies the Linux x64 hashes before executing either engine. It creates each existing security-contract fixture with the unchanged `evals/cases.json` overrides and local `rule.yaml`, copies the rule to `node_modules/.cache/oxguard/rules`, and uses `scan --config node_modules/.cache/oxguard/rules --error --quiet --json` with the same root targets and default directory exclusions; the candidate uses the production Go scope builder (`tsguard/security_scope.go`) through a compiled test helper. The harness asserts equality of `paths.scanned`, findings, errors and native exits for every default-scope case. Network version checks are disabled in both environments. One warmup is discarded, then five wall-clock runs are measured per engine/case in the same project. The 2.1.0 time includes OxGuard's own scope construction (a separate `git rev-parse`, directory listing and ignore-file check, measured as a whole compiled test-helper process per run, 7.8 ms in the last large-tree run, an upper bound because it includes helper start-up). Runs include process startup and ran with no other load on the host; they characterize this host, not production throughput. The direct engine comparison isolates SAST; the installed eval separately checks dependency failure and full security orchestration.

| Case | 1.23.0 median ms | 2.1.0 median ms (incl. OxGuard scope) | Native exits, both | Findings, both |
|---|---:|---:|---|---:|
| security.clean | 2,702.0 | 183.4 | 0 | 0 |
| security.multiple | 2,721.2 | 193.1 | 1 | 2 |
| security.dependency_failure_still_sast | 2,752.7 | 193.5 | 1 | 1 |
| Generated tree, 50,000 `.ts` files in 500 directories plus an excluded `tests/` and `node_modules/` (3 timed runs) | 24,169.6 | 13,285.1 | 1 | 10 |

The five measured milliseconds are: clean baseline `[2672.8, 2702.0, 2643.3, 2741.6, 2783.1]`, candidate `[185.8, 184.1, 182.1, 183.4, 182.6]`; multiple baseline `[2644.7, 2753.0, 2701.4, 2721.2, 2762.5]`, candidate `[190.3, 194.4, 193.1, 190.6, 239.2]`; dependency-failure baseline `[2752.7, 2691.4, 2833.9, 2699.1, 2911.9]`, candidate `[193.7, 190.1, 208.2, 193.5, 189.3]`; large tree baseline `[23661.6, 24169.6, 25044.8]`, candidate `[12815.3, 13285.1, 15709.7]`. All 81 comparison cases pass (security fixtures and default scope identical, every matrix row classified as documented). On the large tree both engines select the same 50,000 files and report the same 10 findings and exit code. No batching is involved: argv holds only the root entries and exclusion patterns, never a file list.

| Case | Rule ID in both engines | File | Line | Column |
|---|---|---|---:|---:|
| security.multiple | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 1 | 23 |
| security.multiple | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 2 | 27 |
| security.dependency_failure_still_sast | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 1 | 23 |

Both engines return empty native errors arrays for these fixtures. An additional deliberately invalid rule exits 7 in both engines; 2.1.0 emits one `InvalidRuleSchemaError` with a message/path/span instead of 1.23.0’s schema error plus aggregate `SemgrepError`. An adapter regression test verifies the 2.1.0 error remains `invalid_configuration`, with raw diagnostic evidence retained; upstream diagnostic wording/count is not a stable contract. Existing normalization consumes unchanged `check_id`, `path`, `start.line/col`, `extra.message/severity` and `errors`; upstream `engine_kind` changes from `OSS` to `Opengrep`, which is outside the OxGuard contract. The unchanged installed known answers verify findings, source positions, diagnostic artifacts, stable IDs, assessment completeness and native/normalized exits. All 31 installed capability cases pass, including the three required security cases. No expectation was weakened and no rule source, blocking policy or interfile flag changed. The original unchanged-scope claim was incorrect: unadapted 2.1.0 changes ignore targeting even with identical root arguments; the scope arguments described below retain the default scope. The [2.1.0 CLI source](https://github.com/opengrep/opengrep/blob/v2.1.0/src/osemgrep/core/CLI_common.ml#L174) makes cross-file analysis opt-in via `--enable-interfile`; OxGuard does not pass it.

## Target scope and `.semgrepignore`

Owner-approved decisions: `.semgrepignore` stays supported as a project configuration and follows Opengrep 2.1.0's native semantics; the default scope (no `.semgrepignore`) stays what 1.23.0 scanned; tsguard exclusion semantics are unchanged from the shipped version; a Git subproject keeps 1.23.0's nested test paths; minified files are never scanned. OxGuard does not parse ignore files, does not write anything into the scanned project and does not disable project ignore files.

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
| Extra explicit targets for nested test paths | Only for a Git subproject whose scan directory has a `.semgrepignore` and whose Git root has none (below) |
| Root-level `*.min.js` files are not targets | Owner-approved exception: minified files are never scanned. Opengrep exempts explicit root files from every exclusion, so this one filter lives in `securityRootTargets` |

If every root entry is such a minified file, Opengrep is not started (without targets it would scan the working directory) and the security gate records a clean, complete result with no findings and exit 0.

Not used: `--force-exclude` (measured: with it, root *files* named like directory entries (`build`, `dist`, `skills`, `.claude`, `.agents`, `.kiro`, `.next`, `coverage`, `CVS`, `.svn`, `.hg`, `_darcs`) and `*_test.go` stop being scanned, a difference from 1.23.0 beyond minified files), `--x-ignore-semgrepignore-files` (internal, "may change or disappear without notice", and it would remove project ignore files) and `--semgrepignore-filename` with a generated file (writes into the project).

### tsguard exclusions: unchanged from the shipped version

Main passed every entry E as `--exclude E` to 1.23.0, which matches `**/E` and `**/E/**` (wcmatch). Measured against that engine for each form: leading and trailing slashes never anchored anything, so `/src` excluded `src` and `lib/src`, `/a.ts` excluded `src/a.ts` and `lib/a.ts`, and `a/b`, `/a/b`, `a/b/` and `/a/b/` were the same; `./x`, `!x` and `#x` were literals that matched nothing; root-level files (explicit targets) were never excludable. OxGuard maps every entry to `**/` plus the entry stripped of leading and trailing slashes, and `scripts/compare-opengrep.py` asserts identical file sets for `x`, `/x`, `x/`, `/x/`, `a/b`, `/a/b`, `/a/b/`, `/src`, `src/a.ts`, `/a.ts`, `/lib/src`, `*.snap.ts`, `/*.ts`, `!a.ts`, `./x`, `**/x`, `x/*`, below a Git root, and for root files. Main accepted `!` entries (as literals), so they are accepted. One deliberate difference: an entry that is empty after stripping slashes (`""`, `/`; a trailing comma in `--exclude` is enough) excluded every file under 1.23.0 and silently disabled the gate; it is rejected as invalid configuration.

### Default-list layers

The 1.23.0 default list ([template](https://github.com/opengrep/opengrep/blob/v1.23.0/cli/src/semgrep/templates/.semgrepignore)) is split in one table in the code, with a test that the layers reproduce the template entry for entry:

| Layer | Entries | Project `.semgrepignore` |
|---|---|---|
| Baseline, always applied | `.git`, `.svn`, `.hg`, `_darcs`, `CVS`, `build/`, `vendor/`, `dist/`, `*.min.js`, `.env/`, `.tox/`, `node_modules/`, `.npm/`, `.yarn/`, `.venv/`, `_opam/`, `_build/`, `_cargo/` | Cannot remove them (negations do not re-include them); it can only add exclusions |
| Test defaults, replaceable | `test/`, `tests/`, `testsuite/`, `*_test.go` | A `.semgrepignore` at the scan root (1.23.0's rule) or at the Git root (2.1.0's rule), even empty, replaces them |

The reason for the baseline layer: if a short project file such as `docs/` replaced everything, dependencies, vendored code and minified bundles would silently start being scanned. The intentional differences from 1.23.0 are therefore that projects with a `.semgrepignore` no longer have vendored or build-output files scanned (1.23.0 scanned them once any project file existed), and that minified files are never scanned, root level included (1.23.0 scanned an explicit root `x.min.js`; a project whose only finding is in such a file now passes). Both reduce findings only in non-project code.

### Git subprojects: nested test paths

When only the subproject's scan directory has a `.semgrepignore` (Git root has none), Opengrep 2.1.0 applies its own test paths below each root entry, hiding `src/tests`, which 1.23.0 scanned. Measured alternatives: `--project-root <scan root>` inside Git treats the directory as a project without version control, so Git-root and nested `.gitignore` files stop applying and ignored files are scanned; negating the built-ins with `--exclude '!tests/'` does not re-include them; `--semgrepignore-filename` cannot name a path. OxGuard therefore restores the paths with explicit targets, and lets Opengrep and Git make every ignore decision:

1. It asks Opengrep which files it would select, with the scan root as the project so the scan-directory file replaces the built-ins (`scan --x-ls --project-root . <same --exclude arguments> <root entries>`). This applies every `.semgrepignore` and every exclusion, never lists `node_modules` (excluded and never a target) and does not follow symlinked directories. `--x-ls` is an internal flag ("may change or disappear without notice"); the engine is pinned and the harness fails if it changes.
2. Each directory named `test`, `tests` or `testsuite` below a root entry (at any depth) becomes an explicit directory target. Opengrep does not apply its built-ins to the target itself, still applies `.gitignore`, project ignore files and every exclusion inside it, and skips test directories nested in it natively, so targets never overlap and nothing is reported twice (the harness asserts identical finding lists, including a test directory inside a test directory).
3. Each nested `*_test.go` file becomes an explicit file target. Explicit files bypass every filter, so `git check-ignore` first drops the ones `.gitignore` excludes (it never reports tracked files, like Opengrep's own Git handling); the listing already applied `.semgrepignore` and the exclusions.

Bound: the extra targets are limited to 16,000 bytes of argv on every platform (Windows limits a command line to 32,767 characters, shared with the engine path, patterns and root targets). A larger set fails closed with `invalid_configuration` and the workaround below instead of a partial scan. Failures of the listing or `git check-ignore` fail closed as `analyzer_failure`. Workaround for very large subprojects: an empty `.semgrepignore` at the Git root replaces Opengrep's built-ins natively (row `monorepo.scan-root-file-and-empty-git-root-file`).

Which file wins when both exist: patterns of both are applied (Opengrep reads every `.semgrepignore` from the Git root down) and either one replaces the test defaults; 1.23.0 applied only the scan directory's file.

### Compatibility matrix

`python3 scripts/compare-opengrep.py <1.23.0> <2.1.0> --update-doc docs/opengrep-2.1-migration.md` regenerates the table below from the measurement. It runs every row with both pinned engines (sha256 verified first) and the production argument builder, and fails if a row stops behaving as classified. 1.23.0 uses the pre-migration invocation (`--exclude <entry>` and root targets). Counts are files Opengrep reports as scanned; every file carries one `eval`, so selected files and findings coincide, and each row compares rule ID, file, line, column, message, severity, diagnostics (type, level, message, path), the full finding list (so duplicates show) and the exit code. "More scanned" can newly break builds; "less scanned" could hide findings; `approved-reduction` is the owner-approved exception that equals the 1.23.0 result minus the listed minified, vendored or build-output files and differs in nothing else (a self-test proves the check fails on drift in any compared field). In the no-`.semgrepignore` case no row differs from 1.23.0 except the approved minified reduction.

<!-- matrix:start (generated by scripts/compare-opengrep.py --update-doc; do not edit by hand) -->
81 cases measured with both engines: 6 approved-reduction, 3 different, 62 identical, 7 less scanned, 3 more scanned.

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
| `exclude.map.x` | `x` excludes the same files in both engines | 12 | 12 | 1 / 1 | identical |
| `exclude.map.slash-x` | `/x` excludes the same files in both engines | 12 | 12 | 1 / 1 | identical |
| `exclude.map.x-slash` | `x/` excludes the same files in both engines | 12 | 12 | 1 / 1 | identical |
| `exclude.map.slash-x-slash` | `/x/` excludes the same files in both engines | 12 | 12 | 1 / 1 | identical |
| `exclude.map.a-b` | `a/b` excludes the same files in both engines | 13 | 13 | 1 / 1 | identical |
| `exclude.map.slash-a-b` | `/a/b` excludes the same files in both engines | 13 | 13 | 1 / 1 | identical |
| `exclude.map.slash-a-b-slash` | `/a/b/` excludes the same files in both engines | 13 | 13 | 1 / 1 | identical |
| `exclude.map.slash-src` | `/src` excludes the same files in both engines | 10 | 10 | 1 / 1 | identical |
| `exclude.map.src-a-ts` | `src/a.ts` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.slash-a-ts` | `/a.ts` excludes the same files in both engines | 14 | 14 | 1 / 1 | identical |
| `exclude.map.slash-lib-src` | `/lib/src` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.glob` | `*.snap.ts` excludes the same files in both engines | 15 | 15 | 1 / 1 | identical |
| `exclude.map.slash-glob` | `/*.ts` excludes the same files in both engines | 2 | 2 | 1 / 1 | identical |
| `exclude.map.bang` | `!a.ts` excludes the same files in both engines | 16 | 16 | 1 / 1 | identical |
| `exclude.map.dot-slash` | `./x` excludes the same files in both engines | 16 | 16 | 1 / 1 | identical |
| `exclude.map.double-star` | `**/x` excludes the same files in both engines | 12 | 12 | 1 / 1 | identical |
| `exclude.map.children` | `x/*` excludes the same files in both engines | 12 | 12 | 1 / 1 | identical |
| `exclude.map.git-subproject-slash-src` | the same mapping below a Git root: no Git-relative translation | 7 | 7 | 1 / 1 | identical |
| `exclude.map.root-file-not-excludable` | root-level explicit files were never excludable in 1.23.0 and still are not | 14 | 14 | 1 / 1 | identical |
| `layer.docs-only-ignore` | tests are scanned as in 1.23.0; minified/vendored/build artifacts stay excluded (1.23.0 scanned them once a project file existed) (only 1.23.0: `.env/e.ts`, `.tox/t.ts`, `.venv/v.ts`, `_build/b.ts`, `src/vendor/v.ts`, `src/x.min.js`, `vendor/v.ts`) | 21 | 14 | 1 / 1 | less scanned |
| `layer.empty-ignore` | an empty project file still releases the test defaults but not the artifacts (only 1.23.0: `.env/e.ts`, `.tox/t.ts`, `.venv/v.ts`, `_build/b.ts`, `src/vendor/v.ts`, `src/x.min.js`, `vendor/v.ts`) | 22 | 15 | 1 / 1 | less scanned |
| `layer.user-exclude-with-ignore` | tsguard --exclude/config entries apply on top of any project file | 13 | 13 | 1 / 1 | identical |
| `layer.ignore-cannot-reinclude` | negations cannot re-include the baseline layer (only 1.23.0: `.env/e.ts`, `.tox/t.ts`, `.venv/v.ts`, `_build/b.ts`, `src/vendor/v.ts`, `src/x.min.js`, `vendor/v.ts`) | 22 | 15 | 1 / 1 | less scanned |
| `exclude.unreadable-excluded-dir` | 1.23.0 refuses to start on an unreadable root entry; 2.1.0 skips the excluded directory (only 2.1.0: `a.ts`, `cache/private/p.ts`, `docs/d.ts`, `lib/l.ts`, `lib/src/y.ts`, `scripts/s.ts`, `src/a.test.ts`, `src/a.ts`, `src/b.ts`, `src/deep/c.ts`, `src/fixtures/f.ts`, `src/gen/g.ts`) | 0 | 12 | 2 / 1 | different |
| `ignore.root-replaces-defaults` | the test defaults are replaced by the project file in both engines | 14 | 14 | 1 / 1 | identical |
| `ignore.root-empty` | an empty root file also replaces the defaults | 15 | 15 | 1 / 1 | identical |
| `ignore.root-comments-only` |  | 15 | 15 | 1 / 1 | identical |
| `ignore.root-git` |  | 14 | 14 | 1 / 1 | identical |
| `monorepo.no-ignore-files` | defaults apply; Git-root and scan-directory .gitignore honored | 9 | 9 | 1 / 1 | identical |
| `monorepo.scan-root-file` | OxGuard restores the test paths Opengrep's built-ins would hide: root entries and nested directories (src/tests) | 11 | 11 | 1 / 1 | identical |
| `monorepo.scan-root-file-and-empty-git-root-file` | an empty .semgrepignore at the Git root: native replacement, no restoration needed | 11 | 11 | 1 / 1 | identical |
| `monorepo.git-root-file` | the Git-root file is honored (1.23.0 ignored it) and replaces the test defaults (only 1.23.0: `scripts/s.ts`; only 2.1.0: `src/tests/t.ts`, `test/a.ts`, `tests/a.ts`) | 12 | 14 | 1 / 1 | different |
| `monorepo.both-files` | patterns of both files apply (1.23.0 read only the scan-root file); both replace the defaults (only 1.23.0: `scripts/s.ts`) | 14 | 13 | 1 / 1 | less scanned |
| `monorepo.git-root-equals-scan-root` | unchanged when the scan root is the Git root | 12 | 12 | 1 / 1 | identical |
| `nested-tests.basic` | nested test, tests, testsuite directories and nested *_test.go files are scanned as in 1.23.0 | 14 | 14 | 1 / 1 | identical |
| `nested-tests.project-ignored-stays-excluded` | a nested test directory and a *_test.go file the project .semgrepignore excludes stay excluded | 14 | 14 | 1 / 1 | identical |
| `nested-tests.gitignored-stays-excluded` | nested test paths ignored by the Git-root or the scan-directory .gitignore stay excluded | 15 | 15 | 1 / 1 | identical |
| `nested-tests.deep` | deeply nested directories, test directories inside test directories, nested *_test.go inside them | 17 | 17 | 1 / 1 | identical |
| `nested-tests.baseline-inside-stays-excluded` | the baseline layer applies inside nested tests (1.23.0 scanned vendor, build and minified files once a project file existed) (only 1.23.0: `src/tests/vendor/v.ts`, `src/tests/x.min.js`) | 16 | 14 | 1 / 1 | approved-reduction |
| `nested-tests.no-duplicate-reports` | overlapping candidates are reported exactly once: Opengrep skips test directories nested in a target | 16 | 16 | 1 / 1 | identical |
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

Projects without a `.semgrepignore` see no change except a tolerant scan when an excluded directory is unreadable, the rejection of empty exclusion entries, and the approved reduction that minified files, root-level ones included, are never scanned. Projects with a `.semgrepignore`: nested files are honored (can hide findings); negation and character ranges work (both directions); `*` stays within a directory level; inline comments are not recognised; a Git-root `.semgrepignore` is honored too and, like a scan-directory file, replaces the test defaults; unknown directives are ignored; vendored and build-output files are no longer scanned once a project file exists. To keep an exclusion that no longer comes from a `.semgrepignore`, add it to `oxguard.toml` `exclude` or `--exclude`.

## Verification and evidence reuse

The local gate includes `go vet ./...` and `go test ./...` in contract, tsguard and pyguard; eval and harness regression tests; identity-hook and release-document tests; installed npm distribution/capability/maintainability tests; and agent-action scoring with zero failures and zero stale sessions. ZIP rejection tests cover digest mismatch, malicious paths, symlinks, duplicates, missing files, oversized declarations and extracted executable mismatch. A real standalone setup smoke downloads the upstream host asset, validates cache reuse and executes `--version`; CI runs it on Linux, macOS and Windows. Existing `scripts/release-smoke.py archive` and `npm` checks passed on local unpublished archives/tarballs. `install.sh` installed the local checksum-verified archive through a temporary curl transport stub and successfully ran the installed CLI; PowerShell execution is left to Windows/platform verification. Skill files and agent trace hashes were not changed.

Local final results: contract 36 passing tests/subtests, tsguard 171, pyguard 252; the normal Go run skips the opt-in network smoke and the comparison-only helper, which `scripts/compare-opengrep.py` runs. The Node harness (`node --test evals/*.test.cjs evals/python/*.test.cjs evals/p0/*.test.cjs scripts/*.test.cjs tsguard/analysis/change-metrics.test.cjs`) has 92 passes, zero failures and one optional real-Python skip (the CI file list: 86 passes, one skip); required npm distribution has 3/3 passes, installed capability 31/31 (including `security.clean`, `security.multiple` and `security.dependency_failure_still_sast`) and installed maintainability 40/40. Identity hooks pass 6/6 and release documentation 3/3; agent scoring reports 0 fail and 0 stale. Go unit tests of the scope builder cover the default layers (reproducing the 1.23.0 template), the exclusion mapping for every slash form and the rejection of empty entries, project-file replacement of only the test layer, nested files, the monorepo cases (no file, scan-root file, Git-root file, both, scan root equal to the Git root), the derivation of nested test targets from an engine listing (nested and deeply nested directories, `*_test.go`, root-level entries left alone, no duplicate targets, `git check-ignore` filtering, fail-closed listing errors and the argv bound), the explicit empty selection and its clean gate record, the root-level minified-file filter (other root files, including ones named `build`/`skills`/`CVS`, stay targets) and that selection writes nothing even into a read-only scan root; the engine behavior (default scope, the `.semgrepignore` matrix, exclusion forms, nested tests, unreadable directories, symlink escape, root-level files) is asserted by `scripts/compare-opengrep.py` against both pinned engines, whose checker has a self-test (`--self-test`, also run at every start) that fails on drift in any compared field. No skill file mentions `.semgrepignore`, so no skill or agent trace changed.

PR #58 reuse is deliberately not claimed. Running its `classify` function on the staged raw diff returns `{ docs_only: false, component: "none" }`, requiring both components. Its classifier permits reuse only for an unambiguous single component. `npm/**` alone is Tsguard-owned under the actual classifier (even native-tools.json); this PR also changes `.github/workflows/npm.yml`, shared `scripts/**`, fixture documentation and root/docs files, so classification requires both components. Both Go component suites are verified locally and CI must produce fresh platform/component evidence. Reuse would require a successful exact-base main push run within 24 hours plus validated artifacts and manifests; the migration does not weaken that requirement.
