#!/bin/sh
set -eu
if [ "$#" -ne 2 ] || [ -z "$1" ] || [ -z "$2" ]; then
  echo 'Usage: scripts/install-identity-hooks.sh "Your Name" "personal-email@example.com"' >&2
  exit 1
fi
root=$(git rev-parse --show-toplevel)
existing=$(git config --get core.hooksPath || true)
if [ -n "$existing" ] && [ "$existing" != .githooks ]; then
  echo "Existing hooks path $existing needs integration; refusing to replace it." >&2
  exit 1
fi
for hook in pre-commit prepare-commit-msg commit-msg pre-push; do
  if [ -f "$(git rev-parse --git-path "hooks/$hook")" ] && [ "$existing" != .githooks ]; then
    echo "Existing $hook hook needs integration; refusing to disable it." >&2
    exit 1
  fi
done
git config --local user.name "$1"
git config --local user.email "$2"
git config --local oxguard.identityName "$1"
git config --local oxguard.identityEmail "$2"
git config --local core.hooksPath .githooks
"$root/.githooks/check-identity"
echo 'Repository identity hooks installed.'
