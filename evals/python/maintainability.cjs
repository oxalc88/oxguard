'use strict';
// Seeded Python maintainability evals for issue #23. The harness builds small
// fixture projects in a temp directory and runs the built pyguard binary's
// informational `criticality` command against them; no production project and
// no LLM is involved. Case IDs, capabilities and kinds mirror
// evals/maintainability-capabilities.json (enforced by this suite's tests and
// by evals/maintainability-parity.cjs when it validates this report).
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { performance } = require('node:perf_hooks');

const ANALYSIS = path.join(__dirname, '..', '..', 'pyguard', 'analysis');
const EVALUATOR_SHA256 = crypto.createHash('sha256').update(fs.readFileSync(__filename)).digest('hex');
// managed=false keeps uv in the caller-provided pinned environment (same
// policy as evals/python/fixtures); a project-local .venv would hide pyan3.
const PYPROJECT = ['[project]', 'name = "oxguard-maintainability-eval"', 'version = "0.0.0"', 'requires-python = ">=3.12"', '[tool.uv]', 'managed = false', ''].join('\n');

function complete(result) {
  assert.equal(result.assessment, 'complete', `criticality assessment was ${result.assessment}`);
  const gate = result.gates.find(g => g.name === 'criticality');
  assert.ok(gate, 'criticality gate missing');
  // Advisory gate: it reports 'advisory' with ranked findings and 'passed'
  // without them; 'error'/'not_run' are analysis failures, never a pass.
  assert.ok(['passed', 'advisory'].includes(gate.status), `criticality gate was ${gate.status}`);
}

function inDegree(result, file, symbol) {
  const found = result.measurements.find(m => m.metric === 'criticality.in_degree' && m.location.file === file && m.location.symbol === symbol);
  assert.ok(found, `missing criticality.in_degree for ${symbol} in ${file}; measurements: ${JSON.stringify(result.measurements)}`);
  return found.value;
}

function ranked(result, symbol) {
  return result.findings.find(f => f.rule === 'pyguard.criticality.ranked' && f.location.symbol === symbol);
}

// Known answers below were verified against the built binary. pyan3 derives
// module names from the files under a single requested root, so every fixture
// keeps its sources directly in src/ and symbols read `<module>.<function>`.
const definitions = [
  {
    id: 'criticality.distinct_callers_cross_module',
    capability: 'structure.function_graph',
    kind: 'positive',
    files: {
      'src/core.py': 'def target():\n    return 1\n',
      'src/caller_one.py': 'import core\n\n\ndef one():\n    core.target()\n    core.target()\n    return core.target()\n',
      'src/caller_two.py': 'import core\n\n\ndef two():\n    return core.target()\n',
      'src/caller_three.py': 'import core\n\n\ndef three():\n    return core.target()\n',
    },
    check: ({ result }) => {
      // Three distinct callers across three modules rank target at exactly 3;
      // one caller repeating the call three times still counts once.
      complete(result);
      assert.equal(inDegree(result, 'src/core.py', 'core.target'), 3, 'criticality.in_degree for core.target');
      assert.equal(ranked(result, 'core.target').observed, 3, 'ranked criticality finding for core.target');
      assert.equal(inDegree(result, 'src/caller_one.py', 'caller_one.one'), 0, 'criticality.in_degree for caller_one.one');
    },
  },
  {
    id: 'criticality.uncalled_function_zero',
    capability: 'structure.function_graph',
    kind: 'negative',
    files: { 'src/lonely.py': 'def never_called():\n    return 1\n' },
    check: ({ result }) => {
      // A function nobody calls stays at in_degree 0 and never enters the
      // ranked findings; silence must not be fabricated into a caller count.
      complete(result);
      assert.equal(inDegree(result, 'src/lonely.py', 'lonely.never_called'), 0, 'criticality.in_degree for lonely.never_called');
      assert.equal(ranked(result, 'lonely.never_called'), undefined, 'ranked criticality finding for lonely.never_called');
      assert.equal(result.findings.length, 0);
    },
  },
  {
    id: 'criticality.same_caller_not_inflated',
    capability: 'structure.function_graph',
    kind: 'negative',
    files: {
      'src/hub.py': 'def hub():\n    return 1\n',
      'src/busy.py': 'import hub\n\n\ndef busy():\n    hub.hub()\n    hub.hub()\n    hub.hub()\n    return hub.hub()\n',
    },
    check: ({ result }) => {
      // Four calls from one caller are one distinct caller, not four.
      complete(result);
      assert.equal(inDegree(result, 'src/hub.py', 'hub.hub'), 1, 'criticality.in_degree for hub.hub');
      assert.equal(ranked(result, 'hub.hub').observed, 1, 'ranked criticality finding for hub.hub');
      assert.equal(inDegree(result, 'src/busy.py', 'busy.busy'), 0, 'criticality.in_degree for busy.busy');
    },
  },
  {
    id: 'criticality.self_recursion_counts_once',
    capability: 'structure.function_graph',
    kind: 'boundary',
    files: { 'src/recursion.py': 'def countdown(n):\n    if n <= 0:\n        return 0\n    return countdown(n - 1)\n' },
    check: ({ result }) => {
      // Documented boundary: a self-call is one defined caller, so recursion
      // scores exactly 1 — neither 0 (the function does call itself) nor a
      // value that grows with recursion depth or repeated self-calls.
      complete(result);
      assert.equal(inDegree(result, 'src/recursion.py', 'recursion.countdown'), 1, 'criticality.in_degree for recursion.countdown');
      assert.equal(ranked(result, 'recursion.countdown').observed, 1, 'ranked criticality finding for recursion.countdown');
    },
  },
];

// A .cjs/.mjs/.js "binary" is a Node stand-in used by tests so CLI exit codes
// can be exercised on every platform; real pyguard binaries run directly.
function launcher(binary) {
  return /\.(?:c|m)js$/i.test(binary) ? [process.execPath, binary] : [binary];
}

function spawnInvoke({ binary, python }) {
  const prefix = launcher(binary);
  const env = {
    ...process.env,
    PATH: `${path.dirname(python)}${path.delimiter}${process.env.PATH}`,
    VIRTUAL_ENV: path.dirname(path.dirname(python)),
    UV_NO_SYNC: '1', UV_OFFLINE: '1', NO_COLOR: '1',
  };
  return ({ project, args }) => {
    const response = spawnSync(prefix[0], [...prefix.slice(1), ...args], {
      cwd: project, env, encoding: 'utf8', timeout: 60000, maxBuffer: 8 * 1024 * 1024,
    });
    return {
      status: response.status,
      stdout: response.stdout || '',
      stderr: response.stderr || '',
      error: response.error ? response.error.message : undefined,
    };
  };
}

function project(definition, base) {
  const dir = path.join(base, definition.id, 'project');
  fs.mkdirSync(path.join(dir, 'src'), { recursive: true });
  fs.mkdirSync(path.join(dir, 'tools'), { recursive: true });
  fs.writeFileSync(path.join(dir, 'pyproject.toml'), PYPROJECT);
  fs.cpSync(ANALYSIS, path.join(dir, 'tools', 'analysis'), { recursive: true });
  for (const [file, text] of Object.entries(definition.files)) {
    fs.mkdirSync(path.dirname(path.join(dir, file)), { recursive: true });
    fs.writeFileSync(path.join(dir, file), text);
  }
  return dir;
}

function evaluate({ binary, python, root = os.tmpdir(), invoke } = {}) {
  const execute = invoke || spawnInvoke({ binary, python });
  const base = fs.realpathSync(fs.mkdtempSync(path.join(root, 'pyguard-maintainability-')));
  const cases = [];
  try {
    for (const definition of definitions) {
      const start = performance.now();
      let outcome = 'pass';
      let error;
      try {
        const args = ['criticality', '--dirs', 'src', '--output', 'json', '--allow-pipe', '--timeout', '30'];
        const response = execute({ project: project(definition, base), args });
        assert.ok(!response.error, `pyguard could not be started: ${response.error}`);
        assert.equal(response.status, 0, `pyguard criticality exited with ${response.status}: ${String(response.stderr || '').slice(0, 1024)}`);
        definition.check({ result: JSON.parse(response.stdout) });
      } catch (failure) {
        outcome = 'fail';
        error = String((failure && failure.message) || failure);
      }
      const entry = { id: definition.id, capability: definition.capability, kind: definition.kind, outcome, duration_ms: Math.round(performance.now() - start) };
      if (error) entry.error = error;
      cases.push(entry);
    }
    return { schema_version: '1', suite: 'maintainability.python', evaluator_sha256: EVALUATOR_SHA256, cases };
  } finally {
    fs.rmSync(base, { recursive: true, force: true });
  }
}

if (require.main === module) {
  const options = {};
  for (let i = 2; i < process.argv.length; i += 2) {
    const flag = process.argv[i];
    if (!['--binary', '--python', '--output', '--root'].includes(flag) || !process.argv[i + 1] || process.argv[i + 1].startsWith('--')) throw new Error(`Invalid argument ${flag}`);
    options[flag.slice(2)] = process.argv[i + 1];
  }
  if (!options.binary || !options.python || !options.output) throw new Error('Usage: node evals/python/maintainability.cjs --binary <pyguard> --python <python> --output <report.json> [--root <dir>]');
  // Resolve against the invocation cwd (like --python), so a relative
  // --binary survives the spawn inside the temporary fixture project.
  const report = evaluate({ binary: path.resolve(options.binary), python: path.resolve(options.python), root: options.root ? path.resolve(options.root) : os.tmpdir() });
  const output = path.resolve(options.output);
  fs.mkdirSync(path.dirname(output), { recursive: true });
  fs.writeFileSync(output, JSON.stringify(report, null, 2) + '\n');
  const failed = report.cases.filter(c => c.outcome === 'fail');
  console.log(JSON.stringify({ suite: report.suite, passed: report.cases.length - failed.length, failed: failed.length, failures: failed.map(c => ({ id: c.id, error: c.error })) }, null, 2));
  process.exitCode = failed.length ? 1 : 0;
}

module.exports = { evaluate, definitions };
