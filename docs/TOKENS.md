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

#32: 2 implementation rounds, 2 review rounds (round 1 requested 3 fixes, round 2 approved). Per session: mimo 681,389 (44,670 not cached, 29,103 output) and 1,161,033 (52,078 / 17,755); Sol 155,916 (31,342 / 1,950) and 288,963 (35,629 / 2,326). Codex's own "tokens used" lines for the two reviews were 33,292 and 37,955. During review 2, Sol started a `gpt-6-luna` session (142,660 total) that was not requested; it is not counted as a review.

P0a: 4 mimo sessions (the first was stopped when P0a was paused), 3 Sol review rounds (round 1: suffixed Python banners rejected, docs missing; round 2: deep annotations passed or crashed; round 3 approved). Sol ran with multi-agent disabled and reported using no other model.

P0b: per session, pi mimo 1: 4,898,230 total (103,958 not cached, 37,536 output); mimo 2: 9,961,442 (179,723 / 67,671); mimo 3: 2,374,765 (141,728 / 97,357), which hit the 40-minute timeout without a change; glm5.3-flash (escalation at session 4): 626,495 (47,237 / 9,722). Sol review 1: 391,527 (61,809 / 2,806); review 2: 564,029 (70,130 / 3,019); an interrupted launch that never reviewed: 15,058 (2,699 / 71); review 3: 342,828 (56,437 / 1,975). Codex's "tokens used" lines for the reviews were 64,615, 73,149 and 58,412. Sol ran with multi-agent disabled and reported using no other model. 3 review rounds: round 1 found exit-2 JSON syntax diagnostics were being discarded and the changelog wrongly called `hint` new; round 2 found syntax columns weren't normalized; round 3 approved.

P0c: 1 mimo session, no escalation; 1 Sol review round, which approved with no findings (Codex "tokens used" line: 96,620). Sol ran with multi-agent disabled and reported using no other model.

P0d: per session, pi mimo 1: 6,545,273 total (161,478 not cached, 62,259 output), which hit the 40-minute timeout with the work partly done; mimo 2: 1,026,859 (51,933 / 18,894), which finished it and passed verification; glm5.3-flash (escalation at session 3, after mimo's 2-session cap): 99,643 (9,395 / 2,696), which fixed the round-1 changelog findings. Sol review 1: 476,003 (45,691 / 4,584); review 2: 379,824 (46,046 / 3,666). Codex's "tokens used" lines were 50,275 and 49,712. 2 review rounds: round 1 asked for 2 P3 changelog fixes (the CLI exit code, and the false claim that the call graph was unchanged); round 2 approved. Sol ran with multi-agent disabled and reported using no other model.

#31 (model roles doc): no token data was recorded for its sessions, so it has no row. After merging main, its docs-only CI run (40200327340) took 41 s.

Values at 2026-10-25 08:40 Lima; #32 values are final.
