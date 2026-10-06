# Repository Git identity

SSH authentication selects the GitHub account used for repository access. Git's
author and committer name/email select attribution; annotated tags also store a
tagger identity. Configure them separately for each repository.

Install the local identity guard from the repository root:

```sh
scripts/install-identity-hooks.sh "Your Personal Name" "your-personal-email@example.com"
```

The installer sets repository-local `user.name`, `user.email`, the expected
identity, and `core.hooksPath=.githooks`. It refuses to disable existing hooks
that need integration. Other clones must run the installer themselves; Git
does not automatically enable hooks from checked-in files.

`prepare-commit-msg` checks the effective author and committer, including
`--author` and environment overrides. Unlike `pre-commit`, it still runs with
`git commit --no-verify`. `pre-push` checks the current identity and annotated
taggers before uploading refs. These are local safeguards: changing the hooks
path, Git plumbing, and skipping the push hook can bypass them. They do not
provide server-side enforcement or prevent intentionally forged metadata.

Run the isolated regression checks with:

```sh
python3 scripts/test-identity-hooks.py
```

Correcting published author/committer metadata changes commit hashes and all
descendant hashes. Correcting a published annotated tag changes the tag object
and its target. Collaborators must fetch the new refs and realign their branches
after preserving local work. Old clones, cached GitHub pages, workflow logs,
source archives, and existing release artifacts may retain the original hashes
or attribution; rewriting refs cannot guarantee erasure of historical copies.

For the v0.7.0 identity correction, the four commit trees remain identical and
the tag keeps the same release contents. The rewritten release commit includes
`[skip ci]` to avoid triggering the tag's npm publication workflow again. The
subsequent identity-hook commit runs normal branch checks. Published npm package
versions and existing release binaries are not replaced. A local recovery bundle
is retained under `.git/identity-rewrite-backup.bundle`; do not upload it, because
it contains the original metadata.
