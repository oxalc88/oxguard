#!/bin/bash
# usage: run-codex.sh <lang> <session> <promptfile> [effort]
set -uo pipefail
[ -x ~/.local/bin/mise ] && eval "$(~/.local/bin/mise activate bash)"
S=${OXGUARD_CAPTURE_DIR:-/workspace/oxguard-capture}/replay/$1/$2; E=${4:-medium}
cd $S/project
PATH="$S/bin:$PATH" timeout 1500 codex exec --json --ephemeral --ignore-user-config --ignore-rules --skip-git-repo-check \
  -m gpt-6.1-sol -c model_reasoning_effort="\"$E\"" -c features.memories=false -c allow_login_shell=false \
  --sandbox danger-full-access -C $S/project \
  -o $S/agent-last-message.txt "$(cat $3)" < /dev/null > $S/agent-stream.jsonl 2> $S/agent-stderr.log
echo "exit $? effort $E model gpt-6.1-sol" > $S/agent-exit.txt
