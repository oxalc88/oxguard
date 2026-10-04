'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { performance } = require('node:perf_hooks');
const { isDeepStrictEqual } = require('node:util');
const corpus = require('./cases.json');

function walk(directory) {
  return fs.readdirSync(directory).sort().flatMap(name => {
    const file = path.join(directory, name);
    return fs.statSync(file).isDirectory() ? walk(file) : [file];
  });
}
function corpusDigest() {
  const hash = crypto.createHash('sha256');
  for (const file of [path.join(__dirname, 'cases.json'), ...walk(path.join(__dirname, 'fixtures'))]) {
    hash.update(path.relative(__dirname, file).replaceAll('\\', '/'));
    hash.update('\0'); hash.update(fs.readFileSync(file)); hash.update('\0');
  }
  return hash.digest('hex');
}
function projectFinding(f) {
  return { gate: f.gate, rule: f.rule, category: f.category, status: f.status,
    ...(f.location ? { file: f.location.file, ...(f.location.line ? { line: f.location.line } : {}) } : {}) };
}
function match(expected, actual) {
  return Object.entries(expected).every(([key, value]) => actual[key] === value);
}
function detection(expected, actual) {
  const remaining = actual.map(projectFinding).filter(f => f.category === 'quality');
  let tp = 0;
  for (const finding of expected.filter(f => f.category === 'quality')) {
    const index = remaining.findIndex(f => match(finding, f));
    if (index >= 0) { tp++; remaining.splice(index, 1); }
  }
  return { true_positive: tp, false_positive: remaining.length,
    false_negative: expected.filter(f => f.category === 'quality').length - tp };
}
function validate(testCase, result, processResult, root) {
  const errors = [];
  const expect = (ok, message) => { if (!ok) errors.push(message); };
  expect(result.schema_version === '1', 'schema_version must be "1"');
  expect(result.command === testCase.command, 'command mismatch');
  expect(processResult.status === testCase.expected.exit_code, 'process exit code mismatch');
  expect(result.exit_code === processResult.status, 'JSON/process exit code mismatch');
  expect(result.status === testCase.expected.status, `status: expected ${testCase.expected.status}, got ${result.status}`);
  for (const key of ['findings', 'measurements', 'artifacts', 'diagnostics']) expect(Array.isArray(result[key]), `${key} must be an array`);
  expect(!Object.hasOwn(result, 'output'), 'human prose output field is not a machine contract');
  if (errors.some(e => e.endsWith('must be an array'))) return errors;
  let remaining = result.findings.map(projectFinding);
  for (const finding of testCase.expected.findings) {
    const index = remaining.findIndex(f => match(finding, f));
    expect(index >= 0, `missing expected finding: ${JSON.stringify(finding)}`);
    if (index >= 0) remaining.splice(index, 1);
  }
  expect(remaining.length === 0, `unexpected findings: ${JSON.stringify(remaining)}`);
  for (const f of result.findings) {
    expect(typeof f.id === 'string' && f.id.startsWith(`${f.rule}:`) && /^[a-f0-9]{64}$/.test(f.id.split(':').at(-1)), 'invalid stable finding ID');
    expect(Array.isArray(f.diagnostics), 'finding diagnostic IDs must be an array');
    for (const id of f.diagnostics || []) expect(result.diagnostics.some(d => d.id === id), `unknown diagnostic ID ${id}`);
  }
  for (const d of result.diagnostics) expect(typeof d.path === 'string' && fs.existsSync(path.resolve(root, d.path)), `missing diagnostic ${d.id}`);
  if (testCase.fta_measurement) {
    expect(result.measurements.some(m => m.metric === 'fta.score' && m.location.file === 'src/index.ts' && Number.isFinite(m.value)), 'missing FTA score');
    for (const f of result.findings) expect(f.threshold === 1 && f.observed > f.threshold, 'FTA finding must exceed the requested threshold');
  }
  if (testCase.criticality) {
    const measurements = result.measurements.filter(m => m.metric === 'criticality.in_degree').map(m => ({ file: m.location.file, symbol: m.location.symbol, value: m.value }));
    const sorted = values => values.toSorted((a, b) => JSON.stringify(a).localeCompare(JSON.stringify(b)));
    expect(isDeepStrictEqual(sorted(measurements), sorted(testCase.expected.measurements)), 'caller measurements differ from the known graph');
    expect(result.artifacts.some(a => a.kind === 'criticality' && a.path === 'CRITICALITY.md'), 'missing criticality artifact');
    const report = fs.readFileSync(path.join(root, 'CRITICALITY.md'), 'utf8');
    expect(report.includes('| 1 | `src/core.ts:1 target` | 3 |'), 'criticality report must rank target first');
  }
  if (testCase.root_log) expect(result.artifacts.some(a => a.kind === 'log' && a.path === 'nested/project.log'), 'log path must be project-relative through an explicit/alias root');
  return errors;
}

function evaluate({ command, revision = 'unspecified', suite = 'candidate', reportFile }) {
  if (!Array.isArray(command) || !command.length) throw new Error('command must be a non-empty argv array');
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'oxguard-eval-'));
  const invoke = (args, cwd) => {
    const started = performance.now();
    const result = spawnSync(command[0], [...command.slice(1), ...args], { cwd, encoding: 'utf8', timeout: 60000, maxBuffer: 8 * 1024 * 1024 });
    return { ...result, duration_ms: Math.round(performance.now() - started), stdout: result.stdout || '', stderr: result.stderr || '' };
  };
  try {
    const help = invoke(['--help'], temporary);
    const version = invoke(['--version'], temporary);
    const structured = help.status === 0 && /--output\s/.test(help.stdout);
    const criticality = /tsguard criticality\s/.test(help.stdout);
    const cases = [];
    for (const testCase of corpus.cases) {
      const entry = { id: testCase.id, origin: testCase.origin, outcome: 'fail', errors: [], duration_ms: 0, stdout_bytes: 0, stderr_bytes: 0 };
      cases.push(entry);
      if (!structured || (testCase.command === 'criticality' && !criticality)) {
        entry.outcome = help.status === 0 ? 'unsupported' : 'fail';
        entry.errors.push(help.status === 0 ? 'CLI does not advertise the required capability' : 'CLI preflight failed');
        continue;
      }
      const root = path.join(temporary, testCase.id);
      fs.cpSync(path.join(__dirname, 'fixtures', testCase.fixture), root, { recursive: true });
      for (const [file, content] of Object.entries(testCase.files || {})) fs.writeFileSync(path.join(root, file), content);
      let invocationRoot = root;
      let cwd = root;
      if (testCase.root_log) { cwd = path.join(root, 'nested'); fs.mkdirSync(cwd); }
      if (testCase.symlink) {
        invocationRoot = `${root}-alias`;
        try { fs.symlinkSync(root, invocationRoot, 'dir'); }
        catch (error) {
          entry.outcome = 'skipped'; entry.errors.push(`Symlink unavailable: ${error.code}`); continue;
        }
      }
      if (testCase.lock) fs.writeFileSync(path.join(root, '.tsguard.pid'), `${process.pid}\n`);
      const args = [testCase.command, '--output', 'json', '--dirs', 'src', '--timeout', '30',
        ...(testCase.root_log ? ['--root', invocationRoot, '--log-file', 'project.log'] : []), ...(testCase.args || [])];
      const response = invoke(args, cwd);
      entry.duration_ms = response.duration_ms;
      entry.stdout_bytes = Buffer.byteLength(response.stdout);
      entry.stderr_bytes = Buffer.byteLength(response.stderr);
      let result;
      try {
        if (response.error) throw response.error;
        result = JSON.parse(response.stdout);
        entry.errors.push(...validate(testCase, result, response, root));
        entry.actual = { status: result.status, exit_code: result.exit_code, findings: (result.findings || []).map(projectFinding) };
        if (testCase.detection) entry.detection = detection(testCase.expected.findings, result.findings);
        if (testCase.repeat) {
          const again = invoke(args, cwd);
          entry.duration_ms += again.duration_ms;
          if (again.status !== response.status || !isDeepStrictEqual(JSON.parse(again.stdout), result)) entry.errors.push('repeated run changed the normalized result');
        }
        if (testCase.agent) {
          const agentArgs = args.slice(); agentArgs[2] = 'agent';
          const agent = invoke(agentArgs, cwd);
          entry.duration_ms += agent.duration_ms;
          entry.agent_bytes = Buffer.byteLength(agent.stdout);
          entry.agent_lines = agent.stdout.trimEnd().split('\n').length;
          if (agent.error || agent.status !== response.status || entry.agent_bytes > 6144 || entry.agent_lines > 26 || !agent.stdout.includes('findings: 12\n') || !agent.stdout.includes('omitted: 2 (use --output json)')) entry.errors.push('agent summary must be bounded and disclose all omitted findings');
        }
      } catch (error) {
        entry.errors.push(error.message);
        if (testCase.detection && !entry.detection) entry.detection = detection(testCase.expected.findings, []);
      }
      entry.outcome = entry.errors.length ? 'fail' : 'pass';
      if (entry.outcome === 'fail') entry.failure_output = { stdout: response.stdout.slice(0, 4096), stderr: response.stderr.slice(0, 4096) };
    }
    const counts = Object.fromEntries(['pass', 'fail', 'unsupported', 'skipped'].map(state => [state, cases.filter(c => c.outcome === state).length]));
    const detectionTotals = cases.reduce((total, c) => {
      for (const key of Object.keys(total)) total[key] += c.detection?.[key] || 0;
      return total;
    }, { true_positive: 0, false_positive: 0, false_negative: 0 });
    const { true_positive: tp, false_positive: fp, false_negative: fn } = detectionTotals;
    const times = cases.filter(c => c.outcome === 'pass' || c.outcome === 'fail').map(c => c.duration_ms).sort((a, b) => a - b);
    const percentile = fraction => times.length ? times[Math.ceil(times.length * fraction) - 1] : null;
    let toolchain = null;
    if (command[0] === process.execPath && command[1] && fs.existsSync(command[1])) {
      const packageFile = path.resolve(path.dirname(command[1]), '..', 'package.json');
      if (fs.existsSync(packageFile)) toolchain = JSON.parse(fs.readFileSync(packageFile)).dependencies || null;
    }
    const report = { schema_version: '1', corpus_version: corpus.version, corpus_sha256: corpusDigest(), suite, revision,
      evaluator_sha256: crypto.createHash('sha256').update(fs.readFileSync(__filename)).digest('hex'), toolchain,
      costs: { total_case_duration_ms: times.reduce((sum, value) => sum + value, 0), p50_case_duration_ms: percentile(0.5), p95_case_duration_ms: percentile(0.95), max_agent_bytes: Math.max(0, ...cases.map(c => c.agent_bytes || 0)), max_agent_lines: Math.max(0, ...cases.map(c => c.agent_lines || 0)) },
      environment: { platform: process.platform, arch: process.arch, node: process.version },
      cli_version: version.stdout.trim(), execution_path: 'caller-supplied argv; installed launcher in CI',
      lanes: { cli: 'executed', distribution: 'separate installed integration tests', agent: 'scenario/rubric corpus only', pyguard: 'not evaluated' },
      totals: counts, detection: { ...detectionTotals, precision: tp + fp ? tp / (tp + fp) : null, recall: tp + fn ? tp / (tp + fn) : null }, cases };
    if (reportFile) {
      fs.mkdirSync(path.dirname(reportFile), { recursive: true });
      fs.writeFileSync(reportFile, JSON.stringify(report, null, 2) + '\n');
    }
    return report;
  } finally { fs.rmSync(temporary, { recursive: true, force: true }); }
}

if (require.main === module) {
  const options = {};
  for (let i = 2; i < process.argv.length; i += 2) {
    const flag = process.argv[i];
    if (!['--launcher', '--revision', '--report'].includes(flag) || !process.argv[i + 1] || process.argv[i + 1].startsWith('--')) throw new Error(`Invalid argument ${flag}`);
    options[flag.slice(2)] = process.argv[i + 1];
  }
  if (!options.launcher || !options.report) throw new Error('Usage: node evals/run.cjs --launcher /installed/bin/tsguard.cjs --report result.json [--revision SHA]');
  const report = evaluate({ command: [process.execPath, path.resolve(options.launcher)], reportFile: path.resolve(options.report), revision: options.revision });
  console.log(JSON.stringify({ totals: report.totals, detection: report.detection }));
  process.exitCode = report.totals.fail || report.totals.unsupported ? 1 : 0;
}
module.exports = { evaluate, validate, detection, corpusDigest };
