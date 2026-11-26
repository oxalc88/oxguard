'use strict';

// PR-only, component-level evidence reuse. Full main and release verification
// is still mandatory. Never execute a downloaded artifact or relabel a report.
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const { execFileSync } = require('node:child_process');
const { api, trustedRun, validate, files } = require('./verification-evidence.cjs');

const root = path.resolve(__dirname, '..');
const SHA = /^[0-9a-f]{40}$/;
const FRESH_MS = 24 * 60 * 60 * 1000;
const OS = ['ubuntu-latest', 'macos-latest', 'windows-latest'];
const artifacts = { npm: 'oxguard-evals-ubuntu-latest', pyguard: 'oxguard-python-evals-ubuntu-latest' };
const full = { docs_only: false, component: 'none' };

function git(cwd, args) {
  return execFileSync('git', args, { cwd, encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });
}

function classify(raw) {
  if (!raw || !raw.endsWith('\0') || raw.includes('\uFFFD')) return full;
  const fields = raw.slice(0, -1).split('\0');
  const owners = new Set();
  let count = 0;
  for (let i = 0; i < fields.length;) {
    // Raw diff includes old/new modes. Renames, copies, symlinks, submodules,
    // unusual paths and unknown statuses are deliberately not reusable.
    const meta = /^:(000000|100644|100755) (000000|100644|100755) [0-9a-f]+ [0-9a-f]+ ([AMD])$/.exec(fields[i++]);
    const file = fields[i++];
    if (!meta || !file || /[\x00-\x1f\\]/.test(file) || ++count > 3000) return full;
    if (/^(tsguard|pyguard)\/skill\//.test(file)) return full;
    if (/^(docs\/|\.github\/ISSUE_TEMPLATE\/)/.test(file) || ['README.md', 'CHANGELOG.md', 'PROGRESS.md', 'AGENTS.md', 'LICENSE'].includes(file)) owners.add('docs');
    else if (/^(tsguard|npm)\//.test(file)) owners.add('npm');
    else if (/^(pyguard|evals\/python)\//.test(file)) owners.add('pyguard');
    else return full;
  }
  if (owners.size !== 1) return full;
  if (owners.has('docs')) return { docs_only: true, component: 'none' };
  return { docs_only: false, component: owners.has('npm') ? 'pyguard' : 'npm' };
}

function inspectDiff(env = process.env, cwd = root) {
  const { PR_BASE, PR_HEAD, PR_BASE_REF, CANDIDATE_SHA, GITHUB_EVENT_NAME } = env;
  if (GITHUB_EVENT_NAME !== 'pull_request' || PR_BASE_REF !== 'main' || ![PR_BASE, PR_HEAD, CANDIDATE_SHA].every(s => SHA.test(s || ''))) return full;
  try {
    // Use the exact checked-out merge commit, not a mutable PR-files API response.
    if (git(cwd, ['rev-parse', 'HEAD']).trim() !== CANDIDATE_SHA) return full;
    const parents = git(cwd, ['show', '-s', '--format=%P', 'HEAD']).trim().split(' ');
    if (parents.length !== 2 || parents[0] !== PR_BASE || parents[1] !== PR_HEAD) return full;
    if (git(cwd, ['status', '--porcelain', '--untracked-files=no']).trim()) return full;
    const diff = git(cwd, ['diff', '--raw', '-z', '--find-renames', '--find-copies', '--find-copies-harder', PR_BASE, CANDIDATE_SHA, '--']);
    return { ...classify(diff), base: PR_BASE, candidate: CANDIDATE_SHA, baseTree: git(cwd, ['rev-parse', PR_BASE + '^{tree}']).trim() };
  } catch (error) {
    console.log('Cannot prove exact PR impact: ' + error.message);
    return full;
  }
}

function requiredJobs(component) {
  const jobs = { 'Report shared capabilities and outstanding gaps': ['Produce parity matrix (gaps are not passes)', 'Require Level 1 contract completion (agent traces block releases only)'] };
  if (component === 'npm') {
    for (const os of OS) {
      jobs['npm bundled / ' + os] = ['Fresh distribution verification (unlocked npm transitive dependencies)'];
      if (os !== 'windows-latest') jobs['npm bundled / ' + os].push('Go regression tests');
      if (os === 'ubuntu-latest') jobs['npm bundled / ' + os].push('Eval harness regression tests', 'Record successful verification inputs', 'Upload capability eval reports');
      jobs['pnpm 12.6.0 / ' + os] = ['Fresh distribution verification (unlocked npm transitive dependencies)'];
    }
  } else if (component === 'pyguard') {
    for (const os of OS) {
      jobs['PyGuard evals / ' + os] = ['Repository identity hook regression tests', 'Shared result contract tests', 'PyGuard regression tests', 'Owned helper source-location tests'];
      jobs['PyGuard evals / ' + os].push(...(os === 'ubuntu-latest'
        ? ['Evaluate Python behavior and contract parity', 'Evaluate Python maintainability parity', 'Record successful verification inputs', 'Upload Python behavior and parity reports']
        : ['Python platform smoke (existing cases)']));
    }
  } else throw new Error('Unknown component');
  return jobs;
}

function completeJobs(jobs, component) {
  return Object.entries(requiredJobs(component)).every(([name, steps]) => {
    const matches = jobs.filter(j => j.name === name);
    return matches.length === 1 && matches[0].conclusion === 'success' && steps.every(step =>
      matches[0].steps?.filter(s => s.name === step && s.conclusion === 'success').length === 1);
  });
}

async function findEvidence(plan, env = process.env, request = api, now = Date.now()) {
  if (!artifacts[plan.component] || !SHA.test(plan.base || '')) return null;
  const runs = await request('actions/runs?event=push&branch=main&head_sha=' + plan.base + '&status=success&per_page=20', env);
  for (const run of runs.workflow_runs || []) {
    const age = now - Date.parse(run.updated_at);
    if (!trustedRun(run, plan.base, env.GITHUB_REPOSITORY) || !Number.isFinite(age) || age < 0 || age > FRESH_MS) continue;
    const jobs = await request('actions/runs/' + run.id + '/jobs?per_page=100', env);
    if (!completeJobs(jobs.jobs || [], plan.component)) continue;
    const listing = await request('actions/runs/' + run.id + '/artifacts?name=' + encodeURIComponent(artifacts[plan.component]) + '&per_page=100', env);
    const matches = (listing.artifacts || []).filter(a => a.name === artifacts[plan.component] && !a.expired && a.workflow_run?.head_sha === plan.base);
    if (matches.length !== 1) continue;
    return { runId: String(run.id), artifact: artifacts[plan.component] };
  }
  return null;
}

function read(directory, name) {
  const file = path.join(directory, name);
  if (!fs.lstatSync(file).isFile() || fs.lstatSync(file).isSymbolicLink()) throw new Error('Invalid evidence file: ' + name);
  return JSON.parse(fs.readFileSync(file, 'utf8'));
}

function equalCases(report, expected, suite) {
  if (report.schema_version !== '1' || report.suite !== suite || !report.evaluator_sha256 ||
      !Array.isArray(report.cases) || report.cases.some(c => c.outcome !== 'pass') ||
      JSON.stringify(report.cases.map(c => c.id).sort()) !== JSON.stringify([...expected].sort())) {
    throw new Error('Incomplete or failing ' + suite + ' evidence');
  }
}

function verifyReports(directory, plan, node) {
  const maintain = read(directory, 'maintainability.json');
  const spec = require('../evals/maintainability-capabilities.json');
  const reports = plan.component === 'npm'
    ? [['npm-linux-x64/candidate.json', require('../evals/cases.json').cases.map(c => c.id), 'candidate']]
    : ['behavior', 'parity'].map(lane => ['candidate/' + lane + '.json', require('../evals/python/cases.json').cases.filter(c => c.lane === lane).map(c => c.id), 'pyguard.' + lane]);
  for (const [file, ids, suite] of reports) {
    const report = read(directory, file);
    equalCases(report, ids, suite);
    if (report.revision !== plan.base || !report.corpus_sha256 || !report.toolchain || report.environment?.platform !== 'linux' ||
        report.environment?.arch !== 'x64' || report.environment?.node !== 'v' + node.node) throw new Error('Capability report origin or environment mismatch');
  }
  if (plan.component === 'npm') {
    equalCases(maintain, spec.required_tsguard_cases, 'maintainability.installed');
    if (!require('../evals/maintainability-parity.cjs').assess(maintain).tsguard_capabilities_passed) throw new Error('Incomplete Tsguard maintainability');
  } else {
    const expected = spec.capabilities.flatMap(c => Object.values(c.pyguard.cases || {}).flat());
    equalCases(maintain, expected, 'maintainability.python');
    for (const capability of spec.capabilities) {
      for (const [kind, ids] of Object.entries(capability.pyguard.cases || {})) {
        if (ids.some(id => !maintain.cases.some(c => c.id === id && c.capability === capability.id && c.kind === kind))) throw new Error('Misdeclared Python maintainability evidence');
      }
    }
  }
}

function verifyBase(directory, plan, runId, env = process.env, runtime = { platform: process.platform, arch: process.arch, node: process.versions }) {
  if (!artifacts[plan.component] || !SHA.test(plan.base || '') || !SHA.test(plan.baseTree || '') || !/^\d+$/.test(runId || '')) throw new Error('No valid base evidence plan');
  const manifest = read(directory, 'verification.json');
  const inputs = manifest.inputs;
  // The exact base tree and unchanged-component proof come from Git; runner
  // and Node identity are independently checked against the current Linux job.
  if (!inputs || inputs.revision !== plan.base || inputs.tree !== plan.baseTree || inputs.repository !== env.GITHUB_REPOSITORY ||
      inputs.suite !== plan.component || inputs.group !== (plan.component === 'npm' ? 'npm-ubuntu-latest-npm-bundled' : 'pyguard-ubuntu-latest') ||
      inputs.image_os !== env.ImageOS || inputs.image_version !== env.ImageVersion || inputs.runner_environment !== env.RUNNER_ENVIRONMENT ||
      runtime.platform !== 'linux' || runtime.arch !== 'x64' || inputs.platform !== runtime.platform || inputs.arch !== runtime.arch ||
      JSON.stringify(inputs.node) !== JSON.stringify(runtime.node) ||
      (plan.component === 'npm' && inputs.manager !== 'npm')) throw new Error('Base inputs or runner identity differ');
  validate(manifest, { inputs, fingerprint: manifest.fingerprint }, { run_id: runId }, files(directory));
  verifyReports(directory, plan, runtime.node);
  return { revision: plan.candidate, component: plan.component, base: plan.base, run_id: runId, tree: plan.baseTree };
}

function output(name, value) {
  if (process.env.GITHUB_OUTPUT) fs.appendFileSync(process.env.GITHUB_OUTPUT, name + '=' + value + '\n');
}

async function planReuse() {
  output('docs_only', 'false'); output('component', 'none'); output('run_id', ''); output('artifact', '');
  const plan = inspectDiff();
  if (plan.docs_only) { output('docs_only', 'true'); return; }
  if (plan.component === 'none') return;
  try {
    const source = await findEvidence(plan);
    if (!source) return console.log('No fresh trusted main evidence; run both components.');
    output('component', plan.component); output('run_id', source.runId); output('artifact', source.artifact);
  } catch (error) { console.log(error.message + '; run both components.'); }
}

function verifyReuse(directory) {
  output('valid', 'false');
  try {
    const plan = inspectDiff();
    if (plan.component !== process.env.REUSED_COMPONENT) throw new Error('Component plan changed');
    const origin = verifyBase(directory, plan, process.env.REUSE_RUN_ID);
    console.log('Verified base evidence: ' + JSON.stringify(origin));
    output('valid', 'true');
  } catch (error) { console.log(error.message + '; run both components.'); }
}

function combinedReport(directory) {
  const plan = inspectDiff();
  if (plan.component !== process.env.REUSED_COMPONENT) throw new Error('Component plan changed before parity');
  const tsdir = path.join(directory, 'tsguard'), pydir = path.join(directory, 'pyguard');
  const reuse = verifyBase(plan.component === 'npm' ? tsdir : pydir, plan, process.env.REUSE_RUN_ID);
  const ts = read(tsdir, 'npm-linux-x64/candidate.json'), behavior = read(pydir, 'candidate/behavior.json'), python = read(pydir, 'candidate/parity.json');
  const { parity, markdown } = require('../evals/parity.cjs');
  const shared = parity(ts, behavior, python, { reuse });
  const maintain = require('../evals/maintainability-parity.cjs');
  const tsMaintain = read(tsdir, 'maintainability.json'), pyMaintain = read(pydir, 'maintainability.json');
  const quality = maintain.assess(tsMaintain, pyMaintain, { strict: true });
  const level1 = require('../evals/level1-ready.cjs').assess(ts, behavior, python, { reuse });
  const write = (name, value) => fs.writeFileSync(path.join(directory, name), JSON.stringify(value, null, 2) + '\n');
  write('parity.json', shared);
  fs.writeFileSync(path.join(directory, 'parity.md'), markdown(shared));
  write('maintainability-parity.json', { ...quality, reused_evidence: reuse });
  write('level1-ready.json', { ...level1, reused_evidence: reuse });
  write('verification-provenance.json', reuse);
  if (!level1.blocking_ready || maintain.exitStatus(quality, pyMaintain, true)) throw new Error('Component parity gate failed');
  console.log('Parity passed using verified unchanged-component evidence: ' + JSON.stringify(reuse));
}

if (require.main === module) {
  const [mode, directory] = process.argv.slice(2);
  Promise.resolve().then(() => {
    if (mode === 'plan') return planReuse();
    if (mode === 'verify' && directory) return verifyReuse(path.resolve(directory));
    if (mode === 'parity' && directory) return combinedReport(path.resolve(directory));
    throw new Error('Usage: component-verification.cjs plan | verify <reports> | parity <eval-reports>');
  }).catch(error => { console.error(error.message); process.exitCode = 1; });
}
module.exports = { classify, inspectDiff, requiredJobs, completeJobs, findEvidence, verifyBase };
