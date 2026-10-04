'use strict';
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');
const { isDeepStrictEqual } = require('node:util');
const { performance } = require('node:perf_hooks');
const { validate } = require('../run.cjs');
const { compare, markdown } = require('../compare.cjs');
const corpus = require('./cases.json');
function digest(files, baseDirectory = __dirname) {
  const hash = crypto.createHash('sha256');
  for (const file of files.toSorted()) { hash.update(path.relative(baseDirectory, file).replaceAll('\\', '/')); hash.update('\0'); hash.update(fs.readFileSync(file)); hash.update('\0'); }
  return hash.digest('hex');
}
function walk(dir) { return fs.readdirSync(dir).toSorted().flatMap(name => { const file = path.join(dir, name); return fs.statSync(file).isDirectory() ? walk(file) : [file]; }); }
function relativeSource(root, file) {
  return path.relative(fs.realpathSync(root), fs.realpathSync(path.resolve(root, file))).replaceAll('\\', '/');
}
function radonValue(json) {
  return Object.entries(json).find(([file]) => file.replaceAll('\\', '/') === 'src/main.py')?.[1]?.[0]?.complexity;
}
function checked(response, context) {
  if (response.error || response.signal || response.status === null) throw new Error(`${context}: ${response.error?.message || response.signal || 'no exit status'}`);
  return response;
}
function evaluate({ binary, source, python, revision = 'unspecified', reportDirectory }) {
  binary = path.resolve(binary); source = path.resolve(source); python = path.resolve(python);
  const temporary = fs.realpathSync(fs.mkdtempSync(path.join(os.tmpdir(), 'pyguard-evals-')));
  const env = { ...process.env, PATH: `${path.dirname(python)}${path.delimiter}${process.env.PATH}`, VIRTUAL_ENV: path.dirname(path.dirname(python)), UV_NO_SYNC: '1', UV_OFFLINE: '1', NO_COLOR: '1' };
  const invoke = (argv, cwd, overrides = {}) => {
    const start = performance.now();
    const result = spawnSync(argv[0], argv.slice(1), { cwd, env: { ...env, ...overrides }, encoding: 'utf8', timeout: 60000, maxBuffer: 8 * 1024 * 1024 });
    return { ...result, stdout: result.stdout || '', stderr: result.stderr || '', duration_ms: Math.round(performance.now() - start) };
  };
  try {
    const preflight = checked(invoke([binary, '--help'], temporary), 'CLI help');
    if (preflight.status !== 0 || !preflight.stdout.includes('pyguard')) throw new Error('PyGuard help failed');
    const version = checked(invoke([binary, '--version'], temporary), 'CLI version');
    if (version.status !== 0) throw new Error('PyGuard version failed');
    const versions = checked(invoke([python, '-c', 'import importlib.metadata,json,sys; print(json.dumps({"python":sys.version,"packages":{d.metadata["Name"].lower():d.version for d in importlib.metadata.distributions()}}))'], temporary), 'toolchain');
    if (versions.status !== 0) throw new Error('Python toolchain preflight failed');
    const uv = checked(invoke(['uv', '--version'], temporary), 'uv');
    if (uv.status !== 0) throw new Error('uv preflight failed');
    const installed = JSON.parse(versions.stdout);
    installed.packages = Object.fromEntries(Object.entries(installed.packages).map(([name, version]) => [name.toLowerCase().replace(/[-_.]+/g, '-'), version]));
    const toolchain = { ...installed, uv: uv.stdout.trim(), requirements_sha256: digest([path.join(__dirname, 'requirements.txt')]) };
    for (const name of ['ruff', 'mypy', 'radon', 'pyan3', 'networkx', 'pydot']) if (!toolchain.packages[name]) throw new Error(`Missing eval dependency: ${name}`);
    for (const line of fs.readFileSync(path.join(__dirname, 'requirements.txt'), 'utf8').split('\n')) {
      const pin = line.match(/^([a-zA-Z0-9-]+)==([^\s]+)/);
      if (pin && toolchain.packages[pin[1].toLowerCase().replace(/[-_.]+/g, '-')]  !== pin[2]) throw new Error(`Eval dependency does not match lock: ${pin[1]}`);
    }
    const outputFlag = /--output\s/.test(preflight.stdout);
    const rootFlag = /--root\s/.test(preflight.stdout);
    const cases = [];
    for (const c of corpus.cases) {
      const root = path.join(temporary, c.id, 'project'); fs.mkdirSync(path.dirname(root), { recursive: true }); fs.cpSync(path.join(__dirname, 'fixtures', c.fixture), root, { recursive: true });
      fs.mkdirSync(path.join(root, 'tools'), { recursive: true });
      fs.cpSync(path.join(source, 'pyguard', 'analysis'), path.join(root, 'tools', 'analysis'), { recursive: true });
      const args = [binary, c.command, '--dirs', 'src', '--timeout', '30', ...(c.pipe ? [] : ['--allow-pipe'])];
      if (c.lock) fs.writeFileSync(path.join(root, '.pyguard.pid'), `${process.pid}\n`);
      if (c.json || c.agent) args.push('--output', c.agent ? 'agent' : 'json');
      if (c.log) args.push('--log-file', path.join(root, 'raw.log'));
      if (c.root) args.push('--root', root);
      args.push(...(c.args || []));
      let response;
      if (c.pipe) {
        // Node's spawn uses sockets on Unix. Python subprocess.PIPE creates
        // an actual OS pipe, which exercises Go's ModeNamedPipe check.
        const wrapper = checked(invoke([python, '-c', 'import subprocess,sys,json;r=subprocess.run(sys.argv[1:],capture_output=True,text=True,timeout=45);print(json.dumps({"status":r.returncode,"stdout":r.stdout,"stderr":r.stderr}))', ...args], root), 'pipe wrapper');
        if (wrapper.status !== 0) throw new Error('Pipe wrapper failed');
        response = { ...wrapper, ...JSON.parse(wrapper.stdout) };
      } else response = invoke(args, c.root ? temporary : root, c.missing_tool ? { PATH: path.join(temporary, 'no-tools') } : {});
      const entry = { id: c.id, origin: c.origin, outcome: 'fail', errors: [], duration_ms: response.duration_ms, stdout_bytes: Buffer.byteLength(response.stdout), stderr_bytes: Buffer.byteLength(response.stderr), actual: { exit_code: response.status } };
      cases.push(entry);
      try {
        checked(response, c.id);
        if (((c.json || c.agent) && !outputFlag) || (c.root && !rootFlag)) {
          entry.outcome = 'unsupported'; entry.errors.push('Required CLI interface is not advertised; probe was still executed');
          entry.probe = { stdout: response.stdout.slice(0, 4096), stderr: response.stderr.slice(0, 4096) }; continue;
        }
        if (response.status !== c.expected.exit_code) entry.errors.push(`Expected exit ${c.expected.exit_code}, got ${response.status}`);
        if (c.json) {
          const normalized = JSON.parse(response.stdout);
          entry.errors.push(...validate(c, normalized, response, root));
          const again = checked(invoke(args, root, c.missing_tool ? { PATH: path.join(temporary, 'no-tools') } : {}), 'repeat JSON');
          if (again.status !== response.status || !isDeepStrictEqual(JSON.parse(again.stdout), JSON.parse(response.stdout))) entry.errors.push('Normalized result changed across identical runs');
        }
        if (c.agent) {
          if (entry.stdout_bytes > 6144 || response.stdout.trimEnd().split('\n').length > 26 || !response.stdout.includes('findings: 12\n') || !response.stdout.includes('omitted: 2 (use --output json)')) entry.errors.push('Agent output must be bounded and disclose omitted findings');
        }
        if (c.oracle) {
          let argv;
          if (c.oracle === 'mypy') argv = ['uv', 'run', 'mypy', 'src', '--output', 'json'];
          if (c.oracle === 'ruff') argv = ['uv', 'run', 'ruff', 'check', 'src', '--output-format', 'json'];
          if (c.oracle === 'radon') argv = ['uv', 'run', 'radon', 'cc', 'src', '--json'];
          if (c.oracle === 'annotations') argv = [python, '-c', 'import sys,json;sys.path.insert(0,"tools/analysis");import check_type_complexity as c; print(json.dumps([{"line":v.line,"depth":v.depth,"length":v.length} for v in c.check_file(__import__("pathlib").Path("src/main.py"))]))'];
          const oracle = checked(invoke(argv, root), 'analyzer oracle'); entry.duration_ms += oracle.duration_ms;
          if (c.oracle === 'radon' || c.oracle === 'annotations') {
            if (oracle.status !== 0) throw new Error('Analyzer oracle failed');
          } else if (oracle.status !== (c.expected.findings.length ? 1 : 0)) throw new Error('Analyzer oracle exit mismatch');
          if (c.oracle === 'mypy' || c.oracle === 'ruff') {
            const rows = c.oracle === 'mypy' ? oracle.stdout.trim().split('\n').filter(Boolean).map(line => JSON.parse(line)) : JSON.parse(oracle.stdout);
            const findings = rows.map(f => ({ rule: f.code, file: relativeSource(root, f.file || f.filename), line: f.line || f.location.row }));
            entry.actual.oracle_findings = findings;
            if (!isDeepStrictEqual(findings, c.expected.findings)) entry.errors.push('Analyzer findings differ from independent answers');
          } else if (c.oracle === 'radon') {
            entry.actual.complexity = radonValue(JSON.parse(oracle.stdout));
            if (entry.actual.complexity !== c.expected.complexity) entry.errors.push('Cyclomatic complexity differs from hand-counted answer');
          } else {
            entry.actual.violations = JSON.parse(oracle.stdout);
            if (!isDeepStrictEqual(entry.actual.violations, c.expected.violations)) entry.errors.push('Annotation boundary differs from independent answer');
          }
          if (!response.stdout.includes(c.expected.exit_code === 0 ? '[OK]' : '[FAIL]')) entry.errors.push('CLI did not report the expected gate result');
        }
        if (c.log && !fs.readFileSync(path.join(root, 'raw.log'), 'utf8').includes(c.expected.log_contains)) entry.errors.push('Full analyzer evidence missing from log');
        if (c.expected.stops_at && (!response.stdout.includes('FAILED: ruff') || response.stdout.includes('starting mypy'))) entry.errors.push('Check must fail fast at lint');
        if (c.expected.callers) {
          const artifact = path.join(root, 'CRITICALITY.md');
          const text = fs.existsSync(artifact) ? fs.readFileSync(artifact, 'utf8') : '';
          entry.actual.report_excerpt = text.slice(0, 4096);
          // This checks the existing Markdown artifact, not a machine result contract.
          for (const [symbol, count] of Object.entries(c.expected.callers)) {
            const rows = text.split('\n').filter(line => line.startsWith('|'));
            if (!rows.some(row => row.includes(`\`${symbol}\``) && row.split('|')[3]?.trim() === String(count))) entry.errors.push(`Known caller count missing: ${symbol}=${count}`);
          }
        }
        entry.outcome = entry.errors.length ? 'fail' : 'pass';
      } catch (error) { entry.errors.push(error.message); }
      if (entry.outcome === 'fail') entry.failure_output = { stdout: response.stdout.slice(0, 4096), stderr: response.stderr.slice(0, 4096) };
    }
    const base = { schema_version: '1', corpus_version: corpus.version, corpus_sha256: digest([path.join(__dirname, 'cases.json'), ...walk(path.join(__dirname, 'fixtures'))]), evaluator_sha256: digest([__filename, path.join(__dirname, '../run.cjs'), path.join(__dirname, '../compare.cjs')]), revision, toolchain, environment: { platform: process.platform, arch: process.arch, node: process.version }, cli_version: version.stdout.trim(), source_analysis_sha256: digest(walk(path.join(source, 'pyguard', 'analysis')), path.join(source, 'pyguard', 'analysis')), execution_path: 'native Go CLI + uv + pinned Python environment; scripts copied from evaluated source, as setup deploys them', detection: { precision: null, recall: null, reason: 'Analyzer oracles cover legacy behavior; contract cases assert exact findings separately. No aggregate production accuracy is claimed' } };
    const report = lane => {
      const selected = cases.filter(c => corpus.cases.find(spec => spec.id === c.id).lane === lane);
      return { ...base, suite: `pyguard.${lane}`, totals: Object.fromEntries(['pass', 'fail', 'unsupported', 'skipped'].map(outcome => [outcome, selected.filter(c => c.outcome === outcome).length])), cases: selected };
    };
    const behavior = report('behavior'); const parity = report('parity');
    const result = { behavior, parity, agent_ready: parity.cases.filter(c => c.id.startsWith('contract.') || c.id.startsWith('input.')).every(c => c.outcome === 'pass'), parity_ready: parity.cases.every(c => c.outcome === 'pass') };
    if (reportDirectory) {
      fs.mkdirSync(reportDirectory, { recursive: true });
      for (const [lane, value] of Object.entries({ behavior, parity })) fs.writeFileSync(path.join(reportDirectory, `${lane}.json`), JSON.stringify(value, null, 2) + '\n');
    }
    return result;
  } finally { fs.rmSync(temporary, { recursive: true, force: true }); }
}
function guard(before, after) {
  const behavior = compare(before.behavior, after.behavior);
  const parity = compare(before.parity, after.parity);
  // Known missing interfaces remain visible and never count as passes. This
  // eval PR guards existing behavior and refuses loss of any passing parity case.
  const newFailure = parity.cases.some(c => (c.baseline === 'pass' && c.candidate !== 'pass') || (c.candidate === 'fail' && c.baseline !== 'fail') || (c.baseline === 'fail' && c.candidate === 'unsupported') || c.candidate === 'skipped');
  return { behavior, parity, regression_guard_passed: behavior.candidate_ready && !newFailure, agent_ready: after.parity.cases.filter(c => c.id.startsWith('contract.') || c.id.startsWith('input.')).every(c => c.outcome === 'pass') };
}
if (require.main === module) {
  const options = {};
  for (let i = 2; i < process.argv.length; i += 2) {
    if (!['--binary', '--source', '--python', '--revision', '--report-dir', '--baseline-binary', '--baseline-source', '--baseline-revision'].includes(process.argv[i]) || !process.argv[i + 1] || process.argv[i + 1].startsWith('--')) throw new Error(`Invalid argument ${process.argv[i]}`);
    options[process.argv[i].slice(2)] = process.argv[i + 1];
  }
  if (!options.binary || !options.source || !options.python || !options['report-dir']) throw new Error('Required: --binary --source --python --report-dir');
  const candidate = evaluate({ ...options, reportDirectory: path.join(options['report-dir'], 'candidate') });
  let ready = candidate.behavior.totals.fail + candidate.behavior.totals.unsupported === 0 && candidate.parity_ready;
  if (options['baseline-binary']) {
    if (!options['baseline-source'] || !options['baseline-revision']) throw new Error('Baseline requires source and revision');
    const baseline = evaluate({ binary: options['baseline-binary'], source: options['baseline-source'], python: options.python, revision: options['baseline-revision'], reportDirectory: path.join(options['report-dir'], 'baseline') });
    const comparison = guard(baseline, candidate); ready &&= comparison.regression_guard_passed;
    fs.writeFileSync(path.join(options['report-dir'], 'comparison.json'), JSON.stringify(comparison, null, 2) + '\n');
    fs.writeFileSync(path.join(options['report-dir'], 'comparison.md'), markdown(comparison.behavior) + '\n' + markdown(comparison.parity) + `\nAgent ready: ${comparison.agent_ready}\nRegression guard passed: ${comparison.regression_guard_passed}\n`);
  }
  console.log(JSON.stringify({ behavior: candidate.behavior.totals, parity: candidate.parity.totals, agent_ready: candidate.agent_ready, regression_guard_passed: ready }));
  process.exitCode = ready ? 0 : 1;
}
module.exports = { evaluate, guard, checked, relativeSource, radonValue };
