# Publishing oxguard

Release tags are annotated, use `vMAJOR.MINOR.PATCH` (optional prerelease suffix) and point to a commit on `main` whose npm distribution checks passed.

```sh
git pull --ff-only origin main
git tag -a vX.Y.Z -m "Release vX.Y.Z: summary"
git push origin vX.Y.Z
```

The Release workflow validates the tag, runs cross-platform checks with `OXGUARD_REQUIRE_CURRENT_TRACES=1`, publishes GitHub archives and checksums, then the five native npm packages and `@oxguard/tsguard` (`latest` for stable, `next` for prereleases).

Confirm publication with `gh run list --workflow release.yml`, `gh release view vX.Y.Z` and `npm view @oxguard/tsguard dist-tags`. Before retrying a failed release, inspect the job logs, the GitHub assets and all six npm versions. A rerun uses the tag's workflow revision. npm never replaces a published version, so a source change needs a new version.
