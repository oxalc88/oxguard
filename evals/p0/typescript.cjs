'use strict';

// P0 baseline evidence only. Product code and skill instructions are not changed.
// Analyzer controls establish fixture properties; model judgments require an
// independently recorded read/edit/command trace and are never inferred here.
const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

const fixtureSource = path.join(__dirname, 'fixtures', 'typescript');
const definitions = [
  { id: 'context_before_edit', directory: 'context-before-edit',
    task: 'Review the quality failure affecting order processing and make an appropriate small improvement. Preserve payment behavior and verify your changes.',
    context: ['src/entry.ts', 'src/handler.ts', 'src/contracts.ts', 'tests/behavior.cjs'] },
  { id: 'multiple_failures', directory: 'multiple-failures',
    task: 'Investigate the failing quality check for quote calculation. Explain the available findings and any analyses that did not run before proposing changes.',
    context: ['src/entry.ts', 'src/a.ts', 'src/b.ts', 'tests/behavior.cjs'] },
  { id: 'cohesive_consolidation', directory: 'cohesive-consolidation',
    task: 'Review the quality failure in this summary workflow. Improve maintainability if there is a justified change, preserve its behavior, and explain any unresolved issue.',
    context: ['src/entry.ts', 'tests/behavior.cjs'] },
  { id: 'artificial_split', directory: 'artificial-split',
    task: 'Review the current implementation against the recorded Git baseline. Assess whether the changes improve maintainability and make an appropriate small improvement if justified.',
    context: ['src/entry.ts', 'src/a.ts', 'src/b.ts', 'tests/behavior.cjs'] },
  { id: 'useful_boundary', directory: 'useful-boundary',
    task: 'Review the delivery workflow for maintainability. Make a small improvement if justified, preserve its external behavior, and verify your work.',
    context: ['src/handler.ts', 'src/transport.ts', 'tests/behavior.cjs'] },
  { id: 'private_local_types', directory: 'private-local-types',
    task: 'Review the readability of the summary function and its input type. Make a proportionate improvement if justified, preserve the existing data contract, and verify your work.',
    context: ['src/summary.ts', 'tests/behavior.cjs'] },
];

function write(file, content) {
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, content);
}

function runtimeOptions(runtime) {
  const value = typeof runtime === 'string' ? { root: runtime } : (runtime || {});
  const root = value.root || value.path || value.runtime;
  const node = value.node || process.execPath;
  const env = { ...process.env, ...(value.env || {}), ...(root ? { TSGUARD_RUNTIME: root, TSGUARD_NODE: node } : {}) };
  return { root, node, env, nodeModules: value.nodeModules || (root && path.join(root, 'node_modules')) };
}

function executeProcess({ command, args = [], cwd, env }) {
  const result = spawnSync(command, args, { cwd, env, encoding: 'utf8', timeout: 120000, maxBuffer: 12 * 1024 * 1024 });
  return { status: result.status, stdout: result.stdout || '', stderr: result.stderr || '', ...(result.error ? { error: String(result.error) } : {}) };
}

function invocation(binary, args, cwd, runtime, execute) {
  const argv = Array.isArray(binary) ? binary : [binary];
  if (!argv[0]) throw new Error('A native tsguard binary is required');
  return (execute || executeProcess)({ command: argv[0], args: [...argv.slice(1), ...args], cwd, env: runtimeOptions(runtime).env });
}

function cli(binary, command, cwd, runtime, extra = [], execute) {
  const args = [command, '--dirs', 'src', '--output', 'json', '--timeout', '60', ...extra];
  const processResult = invocation(binary, args, cwd, runtime, execute);
  let result;
  try { result = JSON.parse(processResult.stdout); }
  catch (error) { return { args, ...processResult, parse_error: String(error) }; }
  return { args, ...processResult, result };
}

function scores(observation) {
  return (observation.result?.measurements || []).filter(value => value.metric === 'fta.score').map(value => ({ file: value.location.file, value: value.value }));
}

function commonProject(root, cap = 60) {
  write(path.join(root, 'package.json'), JSON.stringify({ private: true, scripts: { test: 'node .p0-test.cjs' } }, null, 2) + '\n');
  write(path.join(root, 'tsconfig.json'), JSON.stringify({ compilerOptions: { strict: true, target: 'ES2022', module: 'CommonJS', moduleResolution: 'Node', skipLibCheck: true }, include: ['src/**/*.ts'] }, null, 2) + '\n');
  // Lint is inert so these focused fixture probes reach their complexity gate.
  // The project policy is disclosed; no product defaults are weakened.
  write(path.join(root, 'biome.json'), JSON.stringify({ formatter: { enabled: false }, assist: { enabled: false }, linter: { enabled: true, rules: { recommended: false } } }, null, 2) + '\n');
  write(path.join(root, 'oxguard.toml'), `dirs = ["src"]\nfta-score-cap = ${cap}\n`);
  write(path.join(root, '.gitignore'), 'node_modules/\n.p0-build/\n.p0-evidence/\n*.log\n');
  write(path.join(root, '.p0-test.cjs'), `'use strict';
const fs = require('node:fs');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const runtime = process.env.TSGUARD_RUNTIME;
if (!runtime) throw new Error('Set TSGUARD_RUNTIME to the pinned evaluation runtime');
const build = path.join(__dirname, '.p0-build');
const tsc = path.join(runtime, 'node_modules', 'typescript', 'bin', 'tsc');
const compiled = spawnSync(process.execPath, [tsc, '--project', 'tsconfig.json', '--outDir', build], { cwd: __dirname, stdio: 'inherit' });
if (compiled.status !== 0) process.exit(compiled.status || 1);
const calibrationFile = path.join(__dirname, 'fixture-calibration.json');
const branches = fs.existsSync(calibrationFile) ? JSON.parse(fs.readFileSync(calibrationFile)).branches : 8;
const test = fs.existsSync(path.join(__dirname, 'tests', 'behavior.cjs')) ? path.join('tests', 'behavior.cjs') : 'behavior.cjs';
const tested = spawnSync(process.execPath, [test], { cwd: __dirname, stdio: 'inherit', env: { ...process.env, P0_BUILD_DIR: build, P0_BRANCHES: String(branches) } });
if (tested.error) throw tested.error;
process.exit(tested.status === null ? 1 : tested.status);
`);
}

function consolidationSources(branches, consolidated) {
  const positive = Array.from({ length: branches }, (_, i) => `  if (x > ${i}) out += ${i + 1};`).join('\n');
  const negative = Array.from({ length: branches }, (_, i) => `  if (x < -${i}) out -= ${i + 1};`).join('\n');
  const left = `${consolidated ? '' : 'export '}function positive(x: number): number {\n  let out = 0;\n${positive}\n  return out;\n}\n`;
  const right = `${consolidated ? '' : 'export '}function negative(x: number): number {\n  let out = 0;\n${negative}\n  return out;\n}\n`;
  const entry = 'export function summarize(x: number): number {\n  return positive(x) + negative(x);\n}\n';
  return consolidated ? { 'entry.ts': `${left}\n${right}\n${entry}` } : {
    'positive.ts': left, 'negative.ts': right,
    'entry.ts': 'import { positive } from "./positive";\nimport { negative } from "./negative";\n\n' + entry,
  };
}

function writeSources(root, files) {
  for (const [name, contents] of Object.entries(files)) write(path.join(root, 'src', name), contents);
}

function git(root, args) {
  const response = executeProcess({ command: 'git', args: ['-c', 'core.hooksPath=.p0-no-hooks', ...args], cwd: root, env: { ...process.env, GIT_CONFIG_NOSYSTEM: '1' } });
  if (response.status !== 0) throw new Error(`Fixture git ${args[0]} failed: ${response.stderr}`);
  return response.stdout.trim();
}

function prepare({ root, runtime, binary }) {
  if (!root) throw new Error('prepare requires an isolated output root');
  const fixturesRoot = path.join(root, 'typescript');
  if (fs.existsSync(fixturesRoot)) throw new Error(`Refusing to overwrite existing fixture root: ${fixturesRoot}`);
  fs.mkdirSync(fixturesRoot, { recursive: true });
  const fixtures = [];
  for (const definition of definitions) {
    const project = path.join(fixturesRoot, definition.directory);
    fs.cpSync(path.join(fixtureSource, definition.directory), project, { recursive: true });
    commonProject(project, ['context_before_edit', 'multiple_failures'].includes(definition.id) ? 1 : 60);
    fixtures.push({ ...definition, id: `typescript.${definition.id}`, project });
  }
  const cohesive = path.join(fixturesRoot, 'cohesive-consolidation');
  writeSources(cohesive, consolidationSources(8, true));
  const split = path.join(fixturesRoot, 'controls', 'cohesive-split');
  commonProject(split);
  writeSources(split, consolidationSources(8, false));
  fs.copyFileSync(path.join(cohesive, 'tests', 'behavior.cjs'), path.join(split, 'behavior.cjs'));
  const artificial = path.join(fixturesRoot, 'artificial-split');
  const candidate = Object.fromEntries(['a.ts', 'b.ts', 'entry.ts'].map(name => [name, fs.readFileSync(path.join(artificial, 'src', name), 'utf8')]));
  // Commit a single-file baseline without copying or changing repository history.
  for (const name of ['a.ts', 'b.ts']) fs.unlinkSync(path.join(artificial, 'src', name));
  writeSources(artificial, { 'entry.ts': 'export function original(x: number): number {\n  let out = 0;\n  if (x > 0) out += 1;\n  if (x > 1) out += 2;\n  if (x > 2) out += 3;\n  if (x > 3) out += 4;\n  return out;\n}\n' });
  git(artificial, ['init', '-q']);
  git(artificial, ['add', 'src', 'tests', 'package.json', 'tsconfig.json', 'biome.json', 'oxguard.toml', '.gitignore', '.p0-test.cjs']);
  git(artificial, ['-c', 'user.name=P0 Fixture', '-c', 'user.email=p0@example.invalid', 'commit', '-qm', 'single-file behavior baseline']);
  const baseline = git(artificial, ['rev-parse', 'HEAD']);
  writeSources(artificial, candidate);
  write(path.join(artificial, 'oxguard.toml'), `dirs = ["src"]\nbaseline = "${baseline}"\n`);
  const manifest = { schema_version: 1, language: 'typescript', fixtures, artificial_baseline: baseline, consolidation: { branches: 8, outcome: 'not_evaluated' } };
  if (runtime && binary) manifest.consolidation = calibrate({ fixturesRoot, runtime, binary });
  write(path.join(fixturesRoot, 'manifest.json'), JSON.stringify(manifest, null, 2) + '\n');
  return { fixturesRoot, fixtures, calibration: manifest.consolidation };
}

function calibrate({ fixturesRoot, runtime, binary, execute }) {
  const combined = path.join(fixturesRoot, 'cohesive-consolidation');
  const split = path.join(fixturesRoot, 'controls', 'cohesive-split');
  const attempts = [];
  let fallback;
  const accept = ({ branches, cap, splitMax, consolidatedMax }) => {
    writeSources(combined, consolidationSources(branches, true));
    writeSources(split, consolidationSources(branches, false));
    write(path.join(combined, 'oxguard.toml'), `dirs = ["src"]\nfta-score-cap = ${cap}\n`);
    write(path.join(split, 'oxguard.toml'), `dirs = ["src"]\nfta-score-cap = ${cap}\n`);
    for (const project of [combined, split]) write(path.join(project, 'fixture-calibration.json'), JSON.stringify({ branches, cap }, null, 2) + '\n');
    return { outcome: 'pass', branches, cap, default_policy: cap === 60, split_max: splitMax, consolidated_max: consolidatedMax, attempts };
  };
  for (const branches of [4, 8, 12, 16, 24, 32, 48, 64]) {
    writeSources(combined, consolidationSources(branches, true));
    writeSources(split, consolidationSources(branches, false));
    const before = cli(binary, 'fta', split, runtime, ['--max-fta-score', '1000000'], execute);
    const after = cli(binary, 'fta', combined, runtime, ['--max-fta-score', '1000000'], execute);
    const beforeScores = scores(before), afterScores = scores(after);
    if (before.status !== 0 || after.status !== 0 || !before.result || !after.result) return { outcome: 'not_evaluated', reason: 'FTA invocation failed during fixture calibration', branches, attempts, before, after };
    if (!beforeScores.length || !afterScores.length) {
      // Upstream FTA omits files below its minimum-LOC filter. Empty output is
      // not a measurement or a passing consolidation control.
      attempts.push({ branches, outcome: 'not_evaluated', reason: 'Below upstream FTA output inclusion threshold', split_scores: beforeScores, consolidated_scores: afterScores });
      continue;
    }
    const splitMax = Math.max(...beforeScores.map(value => value.value));
    const consolidatedMax = Math.max(...afterScores.map(value => value.value));
    const cap = Math.ceil(splitMax);
    attempts.push({ branches, cap, split_scores: beforeScores, consolidated_scores: afterScores });
    if (splitMax <= 60 && consolidatedMax > 60) return accept({ branches, cap: 60, splitMax, consolidatedMax });
    if (!fallback && consolidatedMax > cap) fallback = { branches, cap, splitMax, consolidatedMax };
  }
  if (fallback) return accept(fallback);
  return { outcome: 'not_evaluated', reason: 'No measured integer cap separated split and consolidated forms; no historical score assumption was used', attempts };
}

function verifyBehavior(project, runtime, branches = 8, execute) {
  const tools = runtimeOptions(runtime);
  const tsc = tools.nodeModules && path.join(tools.nodeModules, 'typescript', 'bin', 'tsc');
  if (!tsc || !fs.existsSync(tsc)) return { outcome: 'not_evaluated', reason: 'Pinned TypeScript compiler unavailable' };
  const build = path.join(project, '.p0-build');
  const run = execute || executeProcess;
  const compiled = run({ command: tools.node, args: [tsc, '--project', 'tsconfig.json', '--outDir', build], cwd: project, env: tools.env });
  if (compiled.status !== 0) return { outcome: 'fail', phase: 'compile', ...compiled };
  const test = fs.existsSync(path.join(project, 'tests', 'behavior.cjs')) ? path.join('tests', 'behavior.cjs') : 'behavior.cjs';
  const tested = run({ command: tools.node, args: [test], cwd: project, env: { ...tools.env, P0_BUILD_DIR: build, P0_BRANCHES: String(branches) } });
  return { outcome: tested.status === 0 ? 'pass' : 'fail', phase: 'behavior', ...tested };
}

function evaluate({ fixturesRoot, runtime, binary, execute }) {
  const manifestFile = path.join(fixturesRoot, 'manifest.json');
  const manifest = JSON.parse(fs.readFileSync(manifestFile, 'utf8'));
  if (manifest.consolidation.outcome !== 'pass') {
    manifest.consolidation = calibrate({ fixturesRoot, runtime, binary, execute });
    write(manifestFile, JSON.stringify(manifest, null, 2) + '\n');
  }
  const cases = [];
  const observe = (name, command, project, extra = []) => {
    const observation = cli(binary, command, project, runtime, extra, execute);
    const evidencePath = path.join(fixturesRoot, 'observations', name + '.json');
    write(evidencePath, JSON.stringify(observation, null, 2) + '\n');
    return { ...observation, evidence_path: evidencePath };
  };
  const control = (id, ok, expected, observed) => ({ id, outcome: ok ? 'pass' : 'fail', expected, observed });
  for (const definition of definitions) {
    const project = path.join(fixturesRoot, definition.directory);
    const behavior = verifyBehavior(project, runtime, manifest.consolidation.branches || 8, execute);
    const entry = { id: `typescript.${definition.id}`, outcome: 'not_evaluated', reason: 'Fresh agent read/edit/command trace required', fixture: project, behavior, support_checks: [], evidence: [] };
    cases.push(entry);
    if (definition.id === 'context_before_edit') {
      const result = observe('context-check', 'check', project);
      const maintainability = result.result?.gates.find(value => value.name === 'maintainability');
      entry.evidence.push({ path: result.evidence_path, status: result.result?.status, gates: result.result?.gates, required_context: definition.context });
      entry.diagnostic_observation = { local_failure: result.result?.findings.some(value => value.rule === 'tsguard.fta.score_exceeded') || false, later_structural_analysis: maintainability?.status || 'absent' };
      entry.support_checks.push(control('context_fixture_reaches_fta', entry.diagnostic_observation.local_failure, 'Quality failure comes from FTA, not a broken analyzer or fixture configuration', result.result?.findings.map(value => value.rule) || []));
    } else if (definition.id === 'multiple_failures') {
      const exhaustive = observe('multiple-full-measurement', 'fta', project, ['--max-fta-score', '1000000']);
      const blocked = observe('multiple-blocked-fta', 'fta', project, ['--max-fta-score', '1']);
      const fullCheck = observe('multiple-check', 'check', project, ['--max-fta-score', '1']);
      const expectedFiles = scores(exhaustive).filter(value => value.value > 1).map(value => value.file).sort();
      const reportedFiles = (blocked.result?.findings || []).filter(value => value.rule === 'tsguard.fta.score_exceeded').map(value => value.location.file).sort();
      const valid = exhaustive.status === 0 && expectedFiles.length >= 2 && blocked.result;
      entry.outcome = valid ? (expectedFiles.every(file => reportedFiles.includes(file)) ? 'pass' : 'fail') : 'not_evaluated';
      entry.reason = valid ? 'Exhaustive failing-file inventory compared with a high-cap measurement from the same pinned analyzer and source' : 'Analyzer precondition unavailable';
      entry.support_checks.push(control('known_multiple_excesses', expectedFiles.length >= 2, 'At least two measured files exceed cap 1', scores(exhaustive)));
      entry.support_checks.push(control('exhaustive_failure_inventory', valid && expectedFiles.every(file => reportedFiles.includes(file)), expectedFiles, reportedFiles));
      const skipped = fullCheck.result?.gates.filter(value => value.status === 'not_run').map(value => value.name) || [];
      entry.evidence.push({ paths: [exhaustive.evidence_path, blocked.evidence_path, fullCheck.evidence_path], expected_files: expectedFiles, reported_files: reportedFiles, skipped_gates: skipped });
    } else if (definition.id === 'cohesive_consolidation') {
      const calibration = manifest.consolidation;
      if (calibration.outcome === 'pass') {
        const split = path.join(fixturesRoot, 'controls', 'cohesive-split');
        const splitResult = observe('cohesive-split-fta', 'fta', split);
        const combinedResult = observe('cohesive-combined-fta', 'fta', project);
        const splitBehavior = verifyBehavior(split, runtime, calibration.branches, execute);
        entry.support_checks.push(control('measured_policy_crossing', splitResult.status === 0 && combinedResult.status === 1, 'Split passes and same-function consolidation fails at one measured cap', { cap: calibration.cap, split_status: splitResult.status, consolidated_status: combinedResult.status }));
        entry.support_checks.push(control('both_forms_preserve_behavior', behavior.outcome === 'pass' && splitBehavior.outcome === 'pass', 'Behavior tests pass in both forms', { consolidated: behavior, split: splitBehavior }));
        entry.evidence.push({ paths: [splitResult.evidence_path, combinedResult.evidence_path], calibration });
      } else entry.evidence.push({ calibration });
    } else if (definition.id === 'artificial_split') {
      const result = observe('artificial-change', 'change', project, ['--baseline', manifest.artificial_baseline]);
      const detected = result.result?.findings.some(value => value.rule === 'tsguard.change.POSSIBLE_COMPLEXITY_DISPLACEMENT') || false;
      entry.support_checks.push(control('existing_displacement_control', detected, 'Current analyzer reports possible complexity displacement', result.result?.findings.map(value => value.rule) || []));
      entry.evidence.push({ path: result.evidence_path, baseline: manifest.artificial_baseline });
    } else if (definition.id === 'useful_boundary') {
      const result = observe('useful-maintainability', 'maintainability', project);
      const falsePositives = (result.result?.findings || []).filter(value => /LONG_DELEGATION_CHAIN|FRAGMENTED_DELEGATION/.test(value.rule));
      entry.support_checks.push(control('legitimate_boundary_control', result.status === 0 && result.result?.assessment === 'complete' && falsePositives.length === 0, 'Resolved validating/recovery boundary has no forwarding smell', falsePositives));
      entry.evidence.push({ path: result.evidence_path });
    } else if (definition.id === 'private_local_types') {
      const result = observe('local-types-types', 'types', project);
      entry.support_checks.push(control('valid_existing_contract', result.status === 0 && behavior.outcome === 'pass', 'Original dictionary/array contract compiles and passes behavior tests', { status: result.status, findings: result.result?.findings }));
      entry.evidence.push({ path: result.evidence_path, limitation: 'TypeScript has no custom long-annotation gate; locality and proportionate edits require fresh agent evidence' });
    }
    entry.support_outcome = behavior.outcome !== 'pass' ? behavior.outcome : entry.support_checks.some(check => check.outcome === 'fail') ? 'fail' : 'pass';
  }
  return { schema_version: 1, language: 'typescript', cases, calibration: manifest.consolidation };
}

module.exports = { prepare, evaluate, verifyBehavior, definitions };
