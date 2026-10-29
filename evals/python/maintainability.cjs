'use strict';
// Seeded Python maintainability evals for issue #23. The harness builds small
// fixture projects in a temp directory and runs the built pyguard binary's
// informational `criticality` command and the `radon` gate (radon + complexipy)
// against them; no production project and no LLM is involved. Case IDs,
// capabilities and kinds mirror evals/maintainability-capabilities.json
// (enforced by this suite's tests and by evals/maintainability-parity.cjs when
// it validates this report).
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

function complete(result, gateName = 'criticality') {
  assert.equal(result.assessment, 'complete', `${gateName} assessment was ${result.assessment}`);
  const gate = result.gates.find(g => g.name === gateName);
  assert.ok(gate, `${gateName} gate missing`);
  // Advisory gate: it reports 'advisory' with ranked findings and 'passed'
  // without them; 'error'/'not_run' are analysis failures, never a pass.
  assert.ok(['passed', 'advisory'].includes(gate.status), `${gateName} gate was ${gate.status}`);
}

// The only outcome this harness treats as blocking: a failed/error run, a
// non-zero exit or a failed/error gate. Advisory status never counts.
function blockingOutcome(result) {
  return result.exit_code !== 0 || ['fail', 'error'].includes(result.status)
    || result.gates.some(g => ['failed', 'error'].includes(g.status));
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
// cognitive fixtures below are score-verified against complexipy 8.3.0: a
// chain of d nested ifs scores d(d+1)/2 (6 → 21, 8 → 36) while radon CC stays
// at d+1 (7, 9), so the radon gate itself stays green in every fixture.
const nestChain = (depth, name) => {
  const lines = [`def ${name}(x):`, '    if x > 0:'];
  for (let level = 1; level < depth; level++) {
    const indent = '    '.repeat(level + 1);
    lines.push(`${indent}if x > ${level}:`, `${indent}    pass`);
  }
  lines.push('    return x');
  return lines.join('\n') + '\n';
};
// Two independent chains of four score (1+2+3+4) × 2 = 20 at exactly the
// threshold, with radon CC 9.
const twoChains = (name) => {
  const lines = [`def ${name}(x, y):`];
  for (const param of ['x', 'y']) {
    lines.push(`    if ${param} > 0:`);
    for (let level = 1; level <= 3; level++) {
      const indent = '    '.repeat(level + 1);
      lines.push(`${indent}if ${param} > ${level}:`, `${indent}    pass`);
    }
  }
  lines.push('    return x + y');
  return lines.join('\n') + '\n';
};
const flatFunction = 'def flat(value):\n    return value + 1\n';
// complexipy folds nested functions into the enclosing function: an inner
// chain of eight scores 36 and is reported once under the outer name.
const nestedFunctionChain = (depth, outer, inner) => {
  const lines = [`def ${outer}(x):`, `    def ${inner}(y):`, '        if y > 0:'];
  for (let level = 1; level < depth; level++) {
    const indent = '    '.repeat(level + 2);
    lines.push(`${indent}if y > ${level}:`, `${indent}    pass`);
  }
  lines.push('        return y', `    return ${inner}(x)`);
  return lines.join('\n') + '\n';
};

function cognitiveMeasurement(result, symbol) {
  return result.measurements.find(m => m.metric === 'complexipy.cognitive' && m.location.symbol === symbol);
}
function cognitiveWarning(result, symbol) {
  return result.findings.find(f => f.rule === 'pyguard.complexipy.cognitive_exceeded' && f.location.symbol === symbol);
}

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
  {
    id: 'complexipy.cognitive.exceeded_is_advisory_warning',
    capability: 'quality.cognitive_complexity',
    kind: 'positive',
    command: 'radon',
    files: { 'src/complexity.py': flatFunction + '\n\n' + nestChain(6, 'above') },
    baselineFiles: { 'src/clean.py': flatFunction },
    check: ({ result, baseline }) => {
      // complexipy 8.3.0 scores the chain-of-six function exactly 21 (>20).
      // The warning must never change the exit code or the gate/overall
      // pass-fail status versus a clean project: both runs exit 0 and neither
      // the run nor the gate fails (blockingOutcome is false for both). The
      // result contract records advisory findings as status 'advisory' — the
      // only difference a warning may make, never 'fail'. pyguard has no
      // strict flag or config, so advisory behavior is unconditional.
      complete(result, 'radon');
      const warning = cognitiveWarning(result, 'above');
      assert.ok(warning, `missing cognitive warning; findings: ${JSON.stringify(result.findings)}`);
      assert.equal(warning.severity, 'warning');
      assert.equal(warning.status, 'advisory');
      assert.equal(warning.gate, 'radon');
      assert.equal(warning.category, 'quality');
      assert.equal(warning.observed, 21);
      assert.equal(warning.threshold, 20);
      assert.equal(warning.location.file, 'src/complexity.py');
      const measurement = cognitiveMeasurement(result, 'above');
      assert.ok(measurement, 'missing complexipy.cognitive measurement');
      assert.equal(measurement.value, 21);
      assert.equal(measurement.level, 'code');
      assert.equal(measurement.unit, 'score');
      assert.equal(measurement.threshold, 20);
      assert.ok(cognitiveMeasurement(result, 'flat'), 'per-function measurement for every function');
      // Clean baseline project through the same command.
      assert.equal(baseline.assessment, 'complete');
      assert.equal(baseline.exit_code, 0);
      assert.equal(baseline.status, 'pass');
      assert.equal(baseline.findings.length, 0);
      assert.equal(result.exit_code, 0);
      assert.equal(result.exit_code, baseline.exit_code, 'warning changed the exit code');
      assert.equal(blockingOutcome(result), blockingOutcome(baseline), 'warning changed the gate/overall outcome');
      assert.equal(blockingOutcome(result), false);
      assert.equal(result.status, 'advisory');
      assert.equal(result.gates.find(g => g.name === 'radon').status, 'advisory');
    },
  },
  {
    id: 'complexipy.cognitive.flat_function_no_finding',
    capability: 'quality.cognitive_complexity',
    kind: 'negative',
    command: 'radon',
    files: { 'src/flat.py': flatFunction },
    check: ({ result }) => {
      // A flat function scores 0: a measurement is emitted, no warning, and
      // the run is a clean pass exactly like a project without complexity.
      complete(result, 'radon');
      assert.equal(result.status, 'pass');
      assert.equal(result.exit_code, 0);
      assert.equal(result.findings.length, 0, JSON.stringify(result.findings));
      const measurement = cognitiveMeasurement(result, 'flat');
      assert.ok(measurement, 'missing complexipy.cognitive measurement');
      assert.equal(measurement.value, 0);
      assert.equal(measurement.threshold, 20);
    },
  },
  {
    id: 'complexipy.cognitive.nested_never_blocking',
    capability: 'quality.cognitive_complexity',
    kind: 'negative',
    command: 'radon',
    files: { 'src/nested.py': nestChain(8, 'deeply_nested') },
    check: ({ result }) => {
      // complexipy scores the deep chain 36 — far above the threshold — yet
      // the warning advisory never blocks: exit 0, no fail/error anywhere and
      // no blocking finding, no matter how extreme the score.
      complete(result, 'radon');
      const warning = cognitiveWarning(result, 'deeply_nested');
      assert.ok(warning, `missing cognitive warning; findings: ${JSON.stringify(result.findings)}`);
      assert.equal(warning.observed, 36);
      assert.equal(warning.severity, 'warning');
      assert.equal(warning.status, 'advisory');
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
      for (const f of result.findings) assert.notEqual(f.status, 'blocking');
      assert.equal(cognitiveMeasurement(result, 'deeply_nested').value, 36);
    },
  },
  {
    id: 'complexipy.cognitive.test_files_excluded',
    capability: 'quality.cognitive_complexity',
    kind: 'negative',
    command: 'radon',
    files: {
      'src/main.py': flatFunction,
      'src/test_root.py': nestChain(6, 'test_root'),
      'src/root_test.py': nestChain(6, 'root_test'),
      'src/conftest.py': nestChain(6, 'conftest_root'),
      'src/tests/test_nested.py': nestChain(6, 'test_in_tests'),
      'src/test/test_nested.py': nestChain(6, 'test_in_test'),
      'src/pkg/tests/test_nested.py': nestChain(6, 'test_in_pkg_tests'),
      'src/pkg/test/test_nested.py': nestChain(6, 'test_in_pkg_test'),
    },
    check: ({ result }) => {
      // Root-level test_*.py, *_test.py, conftest.py and whole tests/test
      // directories at any depth are outside complexipy's advisory scope. Each
      // excluded fixture scores 21, so a leak would add measurements/warnings;
      // only the clean src/main.py function may remain.
      complete(result, 'radon');
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
      assert.ok(cognitiveMeasurement(result, 'flat'), 'clean non-test function must be measured');
      assert.equal(result.measurements.filter(m => m.metric === 'complexipy.cognitive').length, 1, JSON.stringify(result.measurements));
      for (const symbol of ['test_root', 'root_test', 'conftest_root', 'test_in_tests', 'test_in_test', 'test_in_pkg_tests', 'test_in_pkg_test']) {
        assert.equal(cognitiveMeasurement(result, symbol), undefined, `${symbol} must be excluded from measurements`);
        assert.equal(cognitiveWarning(result, symbol), undefined, `${symbol} must be excluded from warnings`);
      }
      assert.equal(result.findings.filter(f => f.rule === 'pyguard.complexipy.cognitive_exceeded').length, 0);
    },
  },
  {
    id: 'complexipy.cognitive.nested_function_folds_into_outer',
    capability: 'quality.cognitive_complexity',
    kind: 'boundary',
    command: 'radon',
    files: { 'src/nested_function.py': nestedFunctionChain(8, 'outer', 'inner') },
    check: ({ result }) => {
      // complexipy reports only the enclosing function and folds the inner
      // chain (36) into it. pyguard keeps exactly that one outer measurement
      // and one advisory warning; it never invents an inner symbol or blocks.
      complete(result, 'radon');
      const measurements = result.measurements.filter(m => m.metric === 'complexipy.cognitive');
      assert.equal(measurements.length, 1, JSON.stringify(measurements));
      assert.equal(measurements[0].location.symbol, 'outer');
      assert.equal(measurements[0].value, 36);
      assert.equal(cognitiveMeasurement(result, 'inner'), undefined, 'nested function must not get a separate measurement');
      const warning = cognitiveWarning(result, 'outer');
      assert.ok(warning, `missing outer warning; findings: ${JSON.stringify(result.findings)}`);
      assert.equal(warning.observed, 36);
      assert.equal(warning.status, 'advisory');
      assert.equal(cognitiveWarning(result, 'inner'), undefined, 'nested function must not get a separate warning');
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
      for (const f of result.findings) assert.notEqual(f.status, 'blocking');
    },
  },
  {
    id: 'complexipy.cognitive.threshold_boundary',
    capability: 'quality.cognitive_complexity',
    kind: 'boundary',
    command: 'radon',
    files: { 'src/boundary.py': twoChains('boundary') + '\n\n' + nestChain(6, 'above') },
    check: ({ result }) => {
      // Exactly 20 is clean and 21 warns: both functions are measured, only
      // the one strictly above the threshold yields a finding.
      complete(result, 'radon');
      const at = cognitiveMeasurement(result, 'boundary');
      const over = cognitiveMeasurement(result, 'above');
      assert.ok(at && over, 'both functions must be measured');
      assert.equal(at.value, 20);
      assert.equal(over.value, 21);
      assert.equal(cognitiveWarning(result, 'boundary'), undefined, 'exactly 20 must not warn');
      const warning = cognitiveWarning(result, 'above');
      assert.ok(warning, `21 must warn; findings: ${JSON.stringify(result.findings)}`);
      assert.equal(result.findings.filter(f => f.rule === 'pyguard.complexipy.cognitive_exceeded').length, 1);
      assert.equal(result.exit_code, 0);
      assert.equal(blockingOutcome(result), false);
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

function project(definition, base, variant = 'project') {
  const dir = path.join(base, definition.id, variant);
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
      const args = [definition.command || 'criticality', '--dirs', 'src', '--output', 'json', '--allow-pipe', '--timeout', '30'];
      try {
        const response = execute({ project: project(definition, base), args });
        assert.ok(!response.error, `pyguard could not be started: ${response.error}`);
        assert.equal(response.status, 0, `pyguard ${args[0]} exited with ${response.status}: ${String(response.stderr || '').slice(0, 1024)}`);
        const result = JSON.parse(response.stdout);
        let baseline;
        if (definition.baselineFiles) {
          const clean = execute({ project: project({ id: definition.id, files: definition.baselineFiles }, base, 'baseline'), args });
          assert.ok(!clean.error, `pyguard baseline could not be started: ${clean.error}`);
          assert.equal(clean.status, 0, `clean baseline exited with ${clean.status}: ${String(clean.stderr || '').slice(0, 1024)}`);
          baseline = JSON.parse(clean.stdout);
        }
        definition.check({ result, baseline });
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
