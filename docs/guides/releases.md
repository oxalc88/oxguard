# Publishing oxguard

Release tags must be annotated, use `vMAJOR.MINOR.PATCH` (with an optional
prerelease suffix), and point to a commit on `main`. Push the source changes and
wait for the npm distribution checks to pass before pushing the release tag.

For example, from an up-to-date `main`:

```sh
git pull --ff-only origin main
git tag -a v0.7.2 -m "Release v0.7.2: security and release fixes"
git push origin v0.7.2
```

The Release workflow validates the remote tag object and commit, runs
cross-platform checks, builds both CLIs, publishes GitHub archives and checksums,
then publishes the five native npm packages followed by `@oxguard/tsguard`.
Stable versions use npm's `latest` tag; prereleases use `next`.

A pushed Git tag alone does not mean publication succeeded. Check the workflow,
GitHub release and registry:

```sh
gh run list --repo oxalc88/oxguard --workflow release.yml --limit 3
gh release view v0.7.2 --repo oxalc88/oxguard
npm view @oxguard/tsguard@0.7.2 version
npm view @oxguard/tsguard dist-tags
```

If a release fails, inspect failed job logs before retrying. A rerun uses the
original workflow revision; merging a workflow fix does not change that rerun.
The workflow can also be dispatched from `main` for an existing tag, but all
builds still use that tag's verified source commit. It does not include newer
skills or code from `main`.

Before retrying a partially published version, inspect the GitHub assets and
all six npm package versions. npm does not allow replacing an existing published
version. Preserve existing tags and choose a new version when the source changes.
The `v0.7.1` validation failure published nothing; `v0.7.2` includes its security
fixes plus the corrected workflow, skills and documentation.
