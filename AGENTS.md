# Document formatting

For every document you create or edit, keep each prose paragraph on one logical line. Do not insert manual line breaks to wrap prose at a fixed column width; let the editor or viewer wrap it visually. Preserve meaningful line breaks in headings, lists, tables, code blocks, and other structured content.

# Choosing models

Pick a model by role, not by habit:

- Coordination, review and hard coding: Codex.
- Scoped coding with clear instructions: `deepseek-v4-flash` or `glm5.3-flash`.
- Verification and second opinions: `qwen3.8-flash` or `mimo-v2.6-flash`.
- Mechanical edits (renames, formatting, bulk text): `gemma4` or `qwen3.6`.

The writer and the reviewer of a change must use different models. If a flash model fails verification twice on the same task, escalate it to Codex.

In Codex, default to GPT-6.1 Sol (or the available Sol model) with reasoning effort `medium`. Raise to `high` or `xhigh` only after failed verification or for a clearly hard task. Available levels are `low`, `medium`, `high`, `xhigh` and `max`.

Transition: the NaN Builders models served through pi are available only until 2026-10-30. From 2026-10-31 use Codex only: the scoped-coding role moves to Codex at `low` or `medium` effort, and second opinions come from a separate Codex session or a different effort level than the writer.

Evaluation captures (see `docs/guides/oxguard-evals.md`) are agent-neutral: any fresh, history-free agent CLI that runs the observer wrappers can record them. Record the actual agent, provider and model in the trace provenance.
