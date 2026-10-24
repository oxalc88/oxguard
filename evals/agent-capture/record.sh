#!/bin/bash
# usage: record.sh <effort> [<previous-replay-dir>] lang/session ...
# Builds fresh fixtures, runs the bounded setup commands, keeps the sessions that
# are not re-recorded from <previous-replay-dir>, then runs one fresh agent per
# selected session in parallel. Assemble with: node assemble.cjs captures <lang> $C/replay
set -e
D=$(cd "$(dirname "$0")" && pwd); C=${OXGUARD_CAPTURE_DIR:-/workspace/oxguard-capture}; R=$C/replay
E=$1; shift; P=; [ -d "${1:-}" ] && { P=$(cd "$1" && pwd); shift; }
[ -e "$R" ] && { echo "move $R aside first" >&2; exit 1; }
$D/setup.sh
cd $R/typescript/quality/project && ../bin/tsguard types --dirs src --output agent >/dev/null || true
cd $R/typescript/repair/project && ../bin/tsguard types --dirs src --output json >/dev/null || true
for s in review focused; do cd $R/typescript/$s/project && ../bin/tsguard criticality --dirs src --output json >/dev/null; done
cd $R/typescript/change/project && ../bin/tsguard change --dirs src --baseline $(cat ../baseline.txt) --output json >/dev/null
cd $R/python/quality/project && ../bin/pyguard mypy --dirs src --output agent >/dev/null || true
cd $R/python/repair/project && ../bin/pyguard mypy --dirs src --output json >/dev/null || true
for s in review focused; do cd $R/python/$s/project && ../bin/pyguard criticality --dirs src --output json >/dev/null; done
if [ -n "$P" ]; then
  for L in typescript python; do for s in quality repair review focused change; do
    case " $* " in *" $L/$s "*) ;; *) [ -d $P/$L/$s ] && rm -rf $R/$L/$s && cp -a $P/$L/$s $R/$L/$s;; esac
  done; done
  rm -rf $R/python/repair-env && cp -a $P/python/repair-env $R/python/repair-env
fi
for x in "$@"; do $D/run-codex.sh ${x%/*} ${x#*/} $D/prompts/${x%/*}-${x#*/}.txt $E & done; wait
for x in "$@"; do echo "$x $(cat $R/$x/agent-exit.txt)"; done
