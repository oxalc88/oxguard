#!/bin/bash
set -euo pipefail
# Builds disposable replay fixtures. Prerequisites in $C: ts-toolchain/, npm-cache/, py-env/, uv-cache/,
# bin-real/pyguard and the packed tsguard *.tgz (see docs/guides/oxguard-evals.md).
C=${OXGUARD_CAPTURE_DIR:-/workspace/oxguard-capture}; R=$(cd "$(dirname "$0")/../.." && pwd); REC=$R/evals/record-agent-command.cjs
NODE=$(command -v node); NPM=$(command -v npm); UV=$(command -v uv)
rm -rf $C/replay $C/trial; mkdir -p $C/replay
TSBIN=$C/ts-toolchain/node_modules/.bin/tsguard
# ---------- TypeScript ----------
tsproj() { # session
  local S=$C/replay/typescript/$1; mkdir -p $S/project $S/bin
  cp $C/*.tgz $S/
  cp -a $C/ts-toolchain/node_modules $S/project/
  cp $C/ts-toolchain/package-lock.json $S/project/
  mkdir -p $S/project/.agents/skills && cp -a $R/tsguard/skill $S/project/.agents/skills/tsguard && rm -f $S/project/.agents/skills/tsguard/opencode.ts
  printf '## TypeScript quality gates (tsguard)\n\nRead `.agents/skills/tsguard/SKILL.md` before running or interpreting tsguard. Select the linked workflow for the task.\n' > $S/project/AGENTS.md
  echo '{"name":"consumer","private":true,"version":"1.0.0","devDependencies":{"@oxguard/tsguard":"file:../oxguard-tsguard-0.0.0-npm-test.tgz","@oxguard/tsguard-linux-x64":"file:../oxguard-tsguard-linux-x64-0.0.0-npm-test.tgz","typescript":"5.9.3"}}' > $S/project/package.json
  mkdir -p $S/project/src
  cat > $S/config.json <<J
{"tools":{"tsguard":"$S/project/node_modules/.bin/tsguard","npm":"$NPM","npx":"$(command -v npx)","tsc":"$S/project/node_modules/.bin/tsc"},
 "projects":[{"root":"$S/project","events":"$S/events.jsonl","env":{}}]}
J
  for t in tsguard npm npx tsc; do printf '#!/bin/sh\nexec %s %s %s %s "$@"\n' "$NODE" "$REC" "$S/config.json" "$t" > $S/bin/$t; chmod +x $S/bin/$t; done
}
for s in quality repair review change focused; do tsproj $s; done
# quality: 12 type errors
Q=$C/replay/typescript/quality/project
for i in $(seq 0 11); do echo "export const value$i: number = \"$i\";"; done > $Q/src/index.ts
# repair: compiler missing, lockfile declares it, cache available
P=$C/replay/typescript/repair/project
(cd $P && $NPM --cache $C/npm-cache install --offline --ignore-scripts --no-audit --no-fund --package-lock-only >/dev/null)
rm -rf $P/node_modules/typescript; echo 'export const value: number = 1;' > $P/src/index.ts
# review/focused: criticality input
for s in review focused; do
  cat > $C/replay/typescript/$s/project/src/index.ts <<'T'
export function target(x: number): number { return x + 1; }
export function a(): number { return target(1); }
export function b(): number { return target(2); }
export function c(): number { return target(3); }
T
done
# change: baseline commit + split candidate
G=$C/replay/typescript/change/project
cat > $G/src/index.ts <<'T'
export function original(x: number): number {
  let total = 0;
  if (x > 1) total += 1;
  if (x > 2) total += 2;
  if (x > 3) total += 3;
  if (x > 4) total += 4;
  return total;
}
T
(cd $G && git init -q && printf 'node_modules\n' > .gitignore && git add -A && git -c user.name=fixture -c user.email=fixture@example.invalid commit -qm baseline)
cat > $G/src/a.ts <<'T'
export function a(x: number): number {
  let total = 0;
  if (x > 1) total += 1;
  if (x > 2) total += 2;
  return total;
}
T
cat > $G/src/b.ts <<'T'
export function b(x: number): number {
  let total = 0;
  if (x > 3) total += 3;
  if (x > 4) total += 4;
  return total;
}
T
cat > $G/src/index.ts <<'T'
import { a } from "./a";
import { b } from "./b";
export function original(x: number): number {
  return a(x) + b(x);
}
T
(cd $G && git add -A && git -c user.name=fixture -c user.email=fixture@example.invalid commit -qm candidate)
git -C $G rev-parse HEAD~1 > $C/replay/typescript/change/baseline.txt
# ---------- Python ----------
pyproj() { # session envpython
  local S=$C/replay/python/$1; mkdir -p $S/project/src $S/bin
  cp -a $R/pyguard/skill $S/project/.agents-skill-tmp; mkdir -p $S/project/.agents/skills; mv $S/project/.agents-skill-tmp $S/project/.agents/skills/pyguard; rm -f $S/project/.agents/skills/pyguard/opencode.ts
  printf '## Python quality gates (pyguard)\n\nRead `.agents/skills/pyguard/SKILL.md` before running or interpreting pyguard. Select the linked workflow for the task.\n' > $S/project/AGENTS.md
  cp $R/evals/python/fixtures/clean/pyproject.toml $S/project/
  local PYENV=$2
  cat > $S/config.json <<J
{"tools":{"pyguard":"$C/bin-real/pyguard","uv":"$UV","python":"$PYENV/bin/python"},
 "projects":[{"root":"$S/project","events":"$S/events.jsonl","env":{"VIRTUAL_ENV":"$PYENV","PATH":"$PYENV/bin:/usr/local/bin:/usr/bin:/bin","UV_NO_SYNC":"1","UV_OFFLINE":"1","NO_COLOR":"1","UV_CACHE_DIR":"$C/uv-cache"}}]}
J
  for t in pyguard uv python; do printf '#!/bin/sh\nexec %s %s %s %s "$@"\n' "$NODE" "$REC" "$S/config.json" "$t" > $S/bin/$t; chmod +x $S/bin/$t; done
}
for s in quality review focused; do pyproj $s $C/py-env; done
# repair env: venv lacking mypy, hash lock for mypy available offline
RE=$C/replay/python/repair-env; rm -rf $RE; UV_CACHE_DIR=$C/uv-cache $UV venv -q --python $C/py-env/bin/python $RE
grep -v -i -E '^\s*$' $R/evals/python/requirements.txt > $C/req-full.txt
UV_CACHE_DIR=$C/uv-cache $UV pip install -q --offline --python $RE/bin/python --require-hashes -r $R/evals/python/requirements.txt
UV_CACHE_DIR=$C/uv-cache $UV pip uninstall -q --python $RE/bin/python mypy
pyproj repair $RE
cp $R/evals/python/requirements.txt $C/replay/python/repair/project/toolchain-requirements.lock
for i in $(seq 0 11); do echo "value$i: int = \"$i\""; done > $C/replay/python/quality/project/src/main.py
echo 'value: int = 1' > $C/replay/python/repair/project/src/main.py
for s in review focused; do cat > $C/replay/python/$s/project/src/main.py <<'T'
def target(x: int) -> int:
    return x + 1
def a() -> int:
    return target(1)
def b() -> int:
    return target(2)
def c() -> int:
    return target(3)
T
done
echo SETUP_OK
