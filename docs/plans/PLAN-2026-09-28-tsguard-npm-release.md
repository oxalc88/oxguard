---
summary: Merge the ready-to-use tsguard distribution and validate its first npm release.
read_when:
  - Preparing or troubleshooting the first published @oxguard/tsguard release.
status: in-progress
---

# tsguard npm release

## Objective

Merge the tested Go-backed npm distribution into main, publish a 0.6.0 release
candidate without changing the default npm or GitHub latest release, verify
installation from the registry with npm and pnpm, then publish stable 0.6.0.
Preserve the existing GitHub Release/install-script distribution.

## Stages

1. Mark prerelease GitHub tags as prereleases; validate the workflow and commit.
2. Confirm the remote main branch, release tags, GitHub permissions and npm
   publishing secret; fast-forward merge the feature branch into main.
3. Push main, wait for cross-platform CI, and publish `v0.6.0-rc.1` from main.
   Verify all six npm packages have identical versions and `next` points to RC.
4. Install the published RC in fresh npm and pnpm projects; verify version,
   help, full check and audit with the package-owned toolchain.
5. If RC checks pass, publish `v0.6.0`, verify `latest`, and record results.

Publishing is irreversible for a package name/version. Do not create a release
tag until the source commit and remote state are verified. If a release partially
publishes, inspect the registry before retrying; never republish an existing
version blindly.
