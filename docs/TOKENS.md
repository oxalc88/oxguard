# Token log

One row per task or PR and per model. Update this file in each PR.

Format mirrors `docs/TOKENS.md` in `oxalc88/futbol-excitante`.

- **Total**: all tokens of the sessions (input + output). Most input is cached context.
- **Not cached**: input tokens minus cached input tokens.
- **Output**: output + reasoning tokens.
- **Effort**: `default` means no effort was set; Codex used its default.

For Codex sessions, read the last `token_count` event in `~/.codex/sessions/`. For pi, read the `usage` fields of the session files (`--session-dir`).

| Date (Lima) | Task / PR | Tool | Role | Model | Effort | Sessions | Total | Not cached | Output | Source |
|---|---|---|---|---|---|---:|---:|---:|---:|---|
| 2026-10-25 | #32 CI docs-only fast path | pi | write | nan/mimo-v2.6-flash | default | 2 | 1,842,422 | 96,748 | 46,858 | pi session dir (ci) |
| 2026-10-25 | #32 CI docs-only fast path | Codex CLI | review | gpt-6.1-sol | medium | 2 | 444,879 | 66,971 | 4,276 | ~/.codex/sessions (ci) |
| 2026-10-25 | #33 docs-only CI timing test (throwaway) | none | - | - | - | 0 | 0 | 0 | 0 | no model used |
| 2026-10-25 | P0a pins and Python >=3.12 | pi | write | nan/mimo-v2.6-flash | default | 4 | 12,726,677 | 400,121 | 150,172 | pi session dir (p0a) |
| 2026-10-25 | P0a pins and Python >=3.12 | Codex CLI | review | gpt-6.1-sol | medium | 3 | 766,225 | 99,914 | 5,959 | ~/.codex/sessions (p0a) |
| 2026-10-25 | P0b mypy 2.4.0 | pi | write | nan/mimo-v2.6-flash | default | 3 | 17,234,437 | 425,409 | 202,564 | pi session dir (p0b) |
| 2026-10-25 | P0b mypy 2.4.0 | pi | write (escalation) | nan/glm5.3-flash | default | 1 | 626,495 | 47,237 | 9,722 | pi session dir (p0b) |
| 2026-10-25 | P0b mypy 2.4.0 | Codex CLI | review | gpt-6.1-sol | medium | 4 | 1,313,442 | 191,075 | 7,871 | ~/.codex/sessions (p0b) |
| 2026-10-25 | P0c pytest 9.1.1 | pi | write | nan/mimo-v2.6-flash | default | 1 | 7,207,291 | 142,197 | 60,934 | pi session dir (p0c) |
| 2026-10-25 | P0c pytest 9.1.1 | Codex CLI | review | gpt-6.1-sol | medium | 1 | 977,644 | 90,433 | 6,187 | ~/.codex/sessions (p0c) |
| 2026-10-26 | P0d pyan3 2.9.0 | pi | write | nan/mimo-v2.6-flash | default | 2 | 7,572,132 | 213,411 | 81,153 | pi session dir (p0d) |
| 2026-10-26 | P0d pyan3 2.9.0 | pi | write (escalation) | nan/glm5.3-flash | default | 1 | 99,643 | 9,395 | 2,696 | pi session dir (p0d) |
| 2026-10-26 | P0d pyan3 2.9.0 | Codex CLI | review | gpt-6.1-sol | medium | 2 | 855,827 | 91,737 | 8,250 | ~/.codex/sessions (p0d) |
| 2026-10-28 | Deps: certifi 2026.10.23 and filelock 4.2.4 (security) | pi | write | nan/deepseek-v4-flash | default | 1 | 1,417,428 | 64,470 | 15,486 | pi session dir (depA) |
| 2026-10-28 | Deps: certifi 2026.10.23 and filelock 4.2.4 (security) | pi | review | nan/deepseek-v4-flash | default | 1 | 684,522 | 47,523 | 15,431 | pi session dir (depA review) |
| 2026-10-28 | Deps: golang.org/x/sys v0.49.0 and x/term v0.47.0 | pi | write | nan/deepseek-v4-flash | default | 1 | 1,639,154 | 69,846 | 11,036 | pi session dir (depB) |
| 2026-10-28 | Deps: golang.org/x/sys v0.49.0 and x/term v0.47.0 | pi | review | nan/deepseek-v4-flash | default | 1 | 696,481 | 35,702 | 15,659 | pi session dir (depB review) |
| 2026-10-29 | P1 Python maintainability harness (#23) | pi | write | nan/mimo-v2.6-flash | default | 2 | 5,309,991 | 156,333 | 83,322 | pi session dir (p1) |
| 2026-10-29 | P1 Python maintainability harness (#23) | pi | write (escalation) | nan/glm5.3-flash | default | 2 | 180,686 | 34,466 | 5,164 | pi session dir (p1) |
| 2026-10-29 | P1 Python maintainability harness (#23) | Codex CLI | review | gpt-6.1-sol | medium | 4 | 812,102 | 145,792 | 3,782 | ~/.codex/sessions (p1) |

#32: 2 implementation rounds, 2 review rounds (round 1 requested 3 fixes, round 2 approved). Per session: mimo 681,389 (44,670 not cached, 29,103 output) and 1,161,033 (52,078 / 17,755); Sol 155,916 (31,342 / 1,950) and 288,963 (35,629 / 2,326). Codex's own "tokens used" lines for the two reviews were 33,292 and 37,955. During review 2, Sol started a `gpt-6-luna` session (142,660 total) that was not requested; it is not counted as a review.

P0a: 4 mimo sessions (the first was stopped when P0a was paused), 3 Sol review rounds (round 1: suffixed Python banners rejected, docs missing; round 2: deep annotations passed or crashed; round 3 approved). Sol ran with multi-agent disabled and reported using no other model.

P0b: per session, pi mimo 1: 4,898,230 total (103,958 not cached, 37,536 output); mimo 2: 9,961,442 (179,723 / 67,671); mimo 3: 2,374,765 (141,728 / 97,357), which hit the 40-minute timeout without a change; glm5.3-flash (escalation at session 4): 626,495 (47,237 / 9,722). Sol review 1: 391,527 (61,809 / 2,806); review 2: 564,029 (70,130 / 3,019); an interrupted launch that never reviewed: 15,058 (2,699 / 71); review 3: 342,828 (56,437 / 1,975). Codex's "tokens used" lines for the reviews were 64,615, 73,149 and 58,412. Sol ran with multi-agent disabled and reported using no other model. 3 review rounds: round 1 found exit-2 JSON syntax diagnostics were being discarded and the changelog wrongly called `hint` new; round 2 found syntax columns weren't normalized; round 3 approved.

P0c: 1 mimo session, no escalation; 1 Sol review round, which approved with no findings (Codex "tokens used" line: 96,620). Sol ran with multi-agent disabled and reported using no other model.

P0d: per session, pi mimo 1: 6,545,273 total (161,478 not cached, 62,259 output), which hit the 40-minute timeout with the work partly done; mimo 2: 1,026,859 (51,933 / 18,894), which finished it and passed verification; glm5.3-flash (escalation at session 3, after mimo's 2-session cap): 99,643 (9,395 / 2,696), which fixed the round-1 changelog findings. Sol review 1: 476,003 (45,691 / 4,584); review 2: 379,824 (46,046 / 3,666). Codex's "tokens used" lines were 50,275 and 49,712. 2 review rounds: round 1 asked for 2 P3 changelog fixes (the CLI exit code, and the false claim that the call graph was unchanged); round 2 approved. Sol ran with multi-agent disabled and reported using no other model.

P1 (#23 Python maintainability harness): per session, pi mimo 1: 5,165,624 total (138,160 not cached, 78,472 output), which implemented the harness, parity changes, CI wiring, tests and docs and passed the gate; mimo 2: 144,367 (18,173 / 4,850), which fixed the round-1 findings, but those edits (and its session files) were lost when the box state rolled back; glm5.3-flash (escalation after mimo's 2-session cap): 150,533 (16,852 / 4,081), which reapplied the round-1 fixes; glm 2: 30,153 (17,614 / 1,083), which fixed a windows-latest CI failure in the relative `--binary` test (temp dir and checkout on different drives). Sol review 1: 288,252 (54,685 / 1,503); review 2 of the lost edits: 224,448 (43,398 / 954), approved; review 2 repeated on the reapplied fixes: 220,127 (36,909 / 818), approved. Review 3 of the Windows test fix: 79,275 (10,800 / 507), approved. Codex's "tokens used" lines were 56,188, 44,352, 37,727 and 11,307. Round 1 asked for 2 P2 fixes (implemented readiness must explicitly require non-empty positive, negative and boundary arrays; resolve a relative `--binary` against the invocation cwd). Sol ran with multi-agent disabled and reported using no other model.

#31 (model roles doc): no token data was recorded for its sessions, so it has no row. After merging main, its docs-only CI run (40200327340) took 41 s.

Values at 2026-10-25 08:40 Lima; #32 values are final.

Deps certifi/filelock: 1 deepseek implementation session and 1 separate deepseek review session (approved in round 1 with only P3 notes: no issue reference in the changelog, filelock's requires_python moved to >=3.11, old pins still appear in recorded traces, which are not stale). No other model was used.

Deps x/sys/x/term: 1 deepseek implementation session and 1 separate deepseek review session (approved in round 1 with only P3 notes: changelog wording on the go directive, no issue reference, and the pre-existing oxDevelop module path in pyguard/go.mod). No other model was used.
