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

#32: 2 implementation rounds, 2 review rounds (round 1 requested 3 fixes, round 2 approved). Per session: mimo 681,389 (44,670 not cached, 29,103 output) and 1,161,033 (52,078 / 17,755); Sol 155,916 (31,342 / 1,950) and 288,963 (35,629 / 2,326). Codex's own "tokens used" lines for the two reviews were 33,292 and 37,955. During review 2, Sol started a `gpt-6-luna` session (142,660 total) that was not requested; it is not counted as a review.

#31 (model roles doc): no token data was recorded for its sessions, so it has no row.

Values at 2026-10-25 08:40 Lima; #32 values are final.
