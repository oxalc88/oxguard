# Repository Git identity

SSH authentication selects the GitHub account; Git's author, committer and tagger name/email select attribution. Configure them per repository by installing the local identity guard from the repository root:

```sh
scripts/install-identity-hooks.sh "Your Personal Name" "your-personal-email@example.com"
```

The installer sets repository-local `user.name`, `user.email`, the expected identity and `core.hooksPath=.githooks`. Each clone must run it. `prepare-commit-msg` checks the effective author and committer, even with `--no-verify`; `pre-push` checks the identity and annotated taggers. These are local safeguards, not server-side enforcement.

Test them with `python3 scripts/test-identity-hooks.py`. Rewriting published metadata changes every descendant hash; avoid it.
