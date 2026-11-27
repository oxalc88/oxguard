# Opengrep 2.1.0 migration evidence

Measured 2026-11-27 on Linux x64, WSL2 kernel 6.6.87.2, Node 24.18.0, npm 11.16.0 and Go toolchain 1.26.11. Implementation provenance: agent Codex, provider OpenAI through the Codex harness, model gpt-6.1-sol, effort medium. The orchestrator supplies an independent reviewer session; this report is implementation evidence, not approval.

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

A controlled Linux x64 native npm package comparison holds the Go binary, metadata and documentation fixed: the old glibc+musl payload packs to 91,408,207 bytes, while the new single static payload packs to 38,843,142 bytes. Expanded engine bytes grow from 85,568,637 to 161,695,104. A gzip archive of just the x64 glibc executable shrinks from 41,481,697 to 32,191,282 bytes. These are local pack measurements, not published package sizes. Allow additional package/cache/extraction disk space despite the smaller compressed package.

## Equivalent-rule detection and timing

`python3 scripts/compare-opengrep.py /absolute/v1.23.0/opengrep /absolute/v2.1.0/opengrep` verifies the Linux x64 hashes before executing either engine. It creates each existing security-contract fixture with the unchanged `evals/cases.json` overrides and local `rule.yaml`, copies the rule to `node_modules/.cache/oxguard/rules`, and uses `scan --config node_modules/.cache/oxguard/rules --error --quiet --json` with the same root targets. Network version checks are disabled in both environments. One warmup is discarded, then five wall-clock scans are measured per engine/case in the same project. The runs include process startup; concurrent verification was active, so timings characterize these tiny fixtures on this host, not production throughput. The direct engine comparison isolates SAST; the installed eval separately checks dependency failure and full security orchestration.

| Case | 1.23.0 median ms | 2.1.0 median ms | Native exits, both | Findings, both |
|---|---:|---:|---|---:|
| security.clean | 3,109.395 | 225.922 | 0 | 0 |
| security.multiple | 4,190.175 | 199.512 | 1 | 2 |
| security.dependency_failure_still_sast | 3,034.027 | 205.130 | 1 | 1 |

The five measured milliseconds are: clean baseline `[2934.521, 3086.075, 3439.027, 3109.395, 3161.263]`, candidate `[233.210, 217.853, 225.922, 213.580, 240.292]`; multiple baseline `[4753.911, 4190.175, 3477.148, 4397.241, 3299.954]`, candidate `[212.630, 194.645, 199.512, 190.732, 204.854]`; dependency-failure baseline `[3443.137, 3034.027, 2987.241, 3001.890, 3321.956]`, candidate `[261.419, 219.221, 201.771, 201.632, 205.130]`.

| Case | Rule ID in both engines | File | Line | Column |
|---|---|---|---:|---:|
| security.multiple | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 1 | 23 |
| security.multiple | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 2 | 27 |
| security.dependency_failure_still_sast | node_modules..cache.oxguard.rules.oxguard-eval-no-eval | src/index.ts | 1 | 23 |

Both engines return empty native errors arrays for these fixtures. An additional deliberately invalid rule exits 7 in both engines; 2.1.0 emits one `InvalidRuleSchemaError` with a message/path/span instead of 1.23.0’s schema error plus aggregate `SemgrepError`. An adapter regression test verifies the 2.1.0 error remains `invalid_configuration`, with raw diagnostic evidence retained; upstream diagnostic wording/count is not a stable contract. Existing normalization consumes unchanged `check_id`, `path`, `start.line/col`, `extra.message/severity` and `errors`; upstream `engine_kind` changes from `OSS` to `Opengrep`, which is outside the OxGuard contract. The unchanged installed known answers verify findings, source positions, diagnostic artifacts, stable IDs, assessment completeness and native/normalized exits. All 31 installed capability cases pass, including the three required security cases. No expectation was weakened and no rule source, blocking policy, root scope or interfile flag changed. The [2.1.0 CLI source](https://github.com/opengrep/opengrep/blob/v2.1.0/src/osemgrep/core/CLI_common.ml#L174) makes cross-file analysis opt-in via `--enable-interfile`; OxGuard does not pass it.

The release notes mention `.gitignore` targeting changes. Direct equivalent-fixture probes with both engines confirmed the same scanned-file sets for an ignored untracked file in a Git repository (excluded), the same file outside Git (included), and an ignored tracked file (included). This narrow check does not certify every custom `.semgrepignore` glob/negation behavior: 2.1.0 explicitly changes that parser toward gitignore semantics. Existing projects with custom ignore files should review those upstream compatibility notes. Remote p/javascript/p/typescript feeds remain unpinned and can change independently; this comparison deliberately uses fixed local rules.

## Verification and evidence reuse

The local gate includes `go vet ./...` and `go test ./...` in contract, tsguard and pyguard; eval and harness regression tests; identity-hook and release-document tests; installed npm distribution/capability/maintainability tests; and agent-action scoring with zero failures and zero stale sessions. ZIP rejection tests cover digest mismatch, malicious paths, symlinks, duplicates, missing files, oversized declarations and extracted executable mismatch. A real standalone setup smoke downloads the upstream host asset, validates cache reuse and executes `--version`; CI runs it on Linux, macOS and Windows. Existing `scripts/release-smoke.py archive` and `npm` checks passed on local unpublished archives/tarballs. `install.sh` installed the local checksum-verified archive through a temporary curl transport stub and successfully ran the installed CLI; PowerShell execution is left to Windows/platform verification. Skill files and agent trace hashes were not changed.

Local final results: contract 36 passing tests/subtests, tsguard 154, pyguard 252; the normal Go run skips the one opt-in network smoke, which passes separately. The Node harness has 92 passes, zero failures and one optional real-Python skip; required npm distribution has 3/3 passes, installed capability 31/31 and installed maintainability 40/40. Identity hooks pass 6/6 and release documentation 3/3; agent scoring reports 0 fail and 0 stale.

PR #58 reuse is deliberately not claimed. Running its `classify` function on the staged raw diff returns `{ docs_only: false, component: "none" }`, requiring both components. Its classifier permits reuse only for an unambiguous single component. `npm/**` alone is Tsguard-owned under the actual classifier (even native-tools.json); this PR also changes `.github/workflows/npm.yml`, shared `scripts/**`, fixture documentation and root/docs files, so classification requires both components. Both Go component suites are verified locally and CI must produce fresh platform/component evidence. Reuse would require a successful exact-base main push run within 24 hours plus validated artifacts and manifests; the migration does not weaken that requirement.
