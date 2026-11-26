'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { execFileSync } = require('node:child_process');
const { classify, inspectDiff, requiredJobs, completeJobs, findEvidence, verifyBase } = require('./component-verification.cjs');
const { files, hash } = require('./verification-evidence.cjs');
const { parity } = require('../evals/parity.cjs');

const BASE = 'a'.repeat(40), CANDIDATE = 'b'.repeat(40), TREE = 'c'.repeat(40);
const FULL = { docs_only: false, component: 'none' };
const env = { GITHUB_REPOSITORY: 'owner/repo', ImageOS: 'ubuntu24', ImageVersion: 'test-image', RUNNER_ENVIRONMENT: 'github-hosted' };
const runtime = { platform: 'linux', arch: 'x64', node: { node: '24.22.3' } };
const metadata = { schema_version: '1', revision: BASE, evaluator_sha256: 'evaluator', corpus_sha256: 'corpus', toolchain: {}, environment: { platform: 'linux', arch: 'x64', node: 'v24.22.3' } };
function report(suite, cases, revision = BASE) {
  return { ...metadata, revision, suite, cases: cases.map(c => typeof c === 'string' ? { id: c, outcome: 'pass' } : { ...c, outcome: 'pass' }) };
}
const tsIds = require('../evals/cases.json').cases.map(c => c.id);
const pyIds = lane => require('../evals/python/cases.json').cases.filter(c => c.lane === lane).map(c => c.id);
const spec = require('../evals/maintainability-capabilities.json');
const pythonMaintain = spec.capabilities.flatMap(c => Object.entries(c.pyguard.cases || {}).flatMap(([kind, ids]) => ids.map(id => ({ id, kind, capability: c.id }))));
const raw = (file, status = 'M', oldMode = '100644', newMode = '100644') =>
  ':' + oldMode + ' ' + newMode + ' 1234567 7654321 ' + status + '\0' + file + '\0';

test('only one unambiguous component may reuse the other component', () => {
  assert.deepEqual(classify(raw('tsguard/main.go') + raw('npm/toolchain.json')), { docs_only: false, component: 'pyguard' });
  assert.deepEqual(classify(raw('pyguard/main.go') + raw('evals/python/requirements.txt')), { docs_only: false, component: 'npm' });
  assert.deepEqual(classify(raw('docs/guides/test.md') + raw('README.md')), { docs_only: true, component: 'none' });
  assert.deepEqual(classify(raw('evals/python/fixtures/example/README.md')), { docs_only: false, component: 'npm' });
  assert.deepEqual(classify(raw('tsguard/new.go', 'A', '000000')), { docs_only: false, component: 'pyguard' });
  assert.deepEqual(classify(raw('pyguard/old.go', 'D', '100644', '000000')), { docs_only: false, component: 'npm' });
  for (const file of ['contract/result.go', '.github/workflows/npm.yml', 'scripts/verification-evidence.cjs', 'evals/platform-smoke.cjs', 'evals/cases.json', 'tsguard/skill/SKILL.md', 'pyguard/skill/references/example.md', 'unknown.md', 'install.sh']) {
    assert.deepEqual(classify(raw(file)), FULL, file);
  }
  for (const diff of ['', 'invalid', raw('tsguard/a.go') + raw('pyguard/b.go'), raw('tsguard/a.go') + raw('docs/example.md'), raw('tsguard/a.go', 'R100') + 'docs/a.md\0', raw('tsguard/a.go', 'C100') + 'docs/a.md\0', raw('tsguard/link', 'A', '000000', '120000'), raw('pyguard/submodule', 'A', '000000', '160000'), raw('tsguard/a\nb.go'), raw('tsguard/a.go').repeat(3001)]) {
    assert.deepEqual(classify(diff), FULL);
  }
});

function temporary(t) {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'component-verification-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  return directory;
}

test('real Git merge parents and raw diff bind classification to the checked-out candidate', t => {
  const cwd = temporary(t);
  const git = (...args) => execFileSync('git', args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim();
  git('init', '-b', 'main');
  git('config', 'user.name', 'CI fixture');
  git('config', 'user.email', 'fixture@example.invalid');
  git('config', 'core.hooksPath', path.join(cwd, 'absent-hooks'));
  git('config', 'commit.gpgsign', 'false');
  fs.mkdirSync(path.join(cwd, 'tsguard'));
  fs.writeFileSync(path.join(cwd, 'tsguard/main.go'), 'package before\n');
  git('add', '.'); git('commit', '-m', 'base');
  const base = git('rev-parse', 'HEAD'), baseTree = git('rev-parse', 'HEAD^{tree}');
  git('checkout', '-b', 'change');
  fs.writeFileSync(path.join(cwd, 'tsguard/main.go'), 'package after\n');
  git('commit', '-am', 'component change');
  const head = git('rev-parse', 'HEAD');
  git('checkout', 'main'); git('merge', '--no-ff', '-m', 'PR merge fixture', 'change');
  const candidate = git('rev-parse', 'HEAD');
  const event = { GITHUB_EVENT_NAME: 'pull_request', PR_BASE_REF: 'main', PR_BASE: base, PR_HEAD: head, CANDIDATE_SHA: candidate };
  assert.deepEqual(inspectDiff(event, cwd), { docs_only: false, component: 'pyguard', base, candidate, baseTree });
  for (const change of [{ PR_BASE: BASE }, { PR_HEAD: BASE }, { CANDIDATE_SHA: BASE }, { PR_BASE_REF: 'feature' }, { GITHUB_EVENT_NAME: 'push' }, { GITHUB_EVENT_NAME: 'workflow_dispatch' }]) {
    assert.deepEqual(inspectDiff({ ...event, ...change }, cwd), FULL);
  }
  fs.writeFileSync(path.join(cwd, 'tsguard/main.go'), 'dirty');
  assert.deepEqual(inspectDiff(event, cwd), FULL);
});

const goodJobs = component => Object.entries(requiredJobs(component)).map(([name, steps]) => ({
  name, conclusion: 'success', steps: steps.map(name => ({ name, conclusion: 'success' })),
}));

test('trusted main run must have fresh successful platform work and the exact base artifact', async () => {
  const now = Date.parse('2026-11-26T15:00:00Z');
  const run = { id: 123, head_sha: BASE, event: 'push', head_branch: 'main', path: '.github/workflows/npm.yml', status: 'completed', conclusion: 'success', repository: { full_name: env.GITHUB_REPOSITORY }, updated_at: '2026-11-26T14:00:00Z' };
  for (const component of ['npm', 'pyguard']) {
    const plan = { component, base: BASE };
    const name = component === 'npm' ? 'oxguard-evals-ubuntu-latest' : 'oxguard-python-evals-ubuntu-latest';
    const artifact = { name, expired: false, workflow_run: { head_sha: BASE } };
    const request = (runs, jobs, artifacts) => async endpoint => endpoint.startsWith('actions/runs?') ? { workflow_runs: runs } : endpoint.includes('/jobs?') ? { jobs } : { artifacts };
    assert.equal(completeJobs(goodJobs(component), component), true);
    assert.deepEqual(await findEvidence(plan, env, request([run], goodJobs(component), [artifact]), now), { runId: '123', artifact: name });
    for (const change of [{ event: 'pull_request' }, { head_sha: CANDIDATE }, { conclusion: 'failure' }, { updated_at: '2026-11-24T14:00:00Z' }]) {
      assert.equal(await findEvidence(plan, env, request([{ ...run, ...change }], goodJobs(component), [artifact]), now), null);
    }
    const skipped = goodJobs(component);
    skipped[0].steps[0].conclusion = 'skipped';
    assert.equal(await findEvidence(plan, env, request([run], skipped, [artifact]), now), null);
    assert.equal(await findEvidence(plan, env, request([run], goodJobs(component).slice(1), [artifact]), now), null);
    for (const bad of [{ ...artifact, expired: true }, { ...artifact, workflow_run: { head_sha: CANDIDATE } }]) {
      assert.equal(await findEvidence(plan, env, request([run], goodJobs(component), [bad]), now), null);
    }
    assert.equal(await findEvidence(plan, env, request([run], goodJobs(component), []), now), null);
  }
});

function artifactFixture(t, component) {
  const directory = temporary(t);
  const plan = { component, base: BASE, candidate: CANDIDATE, baseTree: TREE };
  const inputs = { repository: env.GITHUB_REPOSITORY, revision: BASE, tree: TREE, workflow_revision: BASE, suite: component,
    group: component === 'npm' ? 'npm-ubuntu-latest-npm-bundled' : 'pyguard-ubuntu-latest',
    image_os: env.ImageOS, image_version: env.ImageVersion, runner_environment: env.RUNNER_ENVIRONMENT,
    ...runtime, ...(component === 'npm' ? { manager: 'npm' } : {}) };
  const write = (name, data) => {
    const file = path.join(directory, name);
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, JSON.stringify(data));
  };
  if (component === 'npm') {
    write('npm-linux-x64/candidate.json', report('candidate', tsIds));
    write('maintainability.json', report('maintainability.installed', spec.required_tsguard_cases));
  } else {
    write('candidate/behavior.json', report('pyguard.behavior', pyIds('behavior')));
    write('candidate/parity.json', report('pyguard.parity', pyIds('parity')));
    write('maintainability.json', report('maintainability.python', pythonMaintain));
  }
  const manifest = { schema: 1, status: 'pass', run_id: '123', event: 'push', ref: 'refs/heads/main', workflow: '.github/workflows/npm.yml',
    inputs, fingerprint: hash(JSON.stringify(inputs)), files: files(directory) };
  const save = () => write('verification.json', manifest);
  save();
  return { directory, plan, inputs, manifest, write, save };
}

test('base evidence verifies every original report hash, case set, provenance and environment', t => {
  for (const component of ['npm', 'pyguard']) {
    const f = artifactFixture(t, component);
    const verify = (plan = f.plan, environment = env, runner = runtime, run = '123') => verifyBase(f.directory, plan, run, environment, runner);
    assert.deepEqual(verify(), { revision: CANDIDATE, component, base: BASE, run_id: '123', tree: TREE });
    for (const change of [{ base: CANDIDATE }, { baseTree: BASE }, { component: 'none' }]) assert.throws(() => verify({ ...f.plan, ...change }));
    assert.throws(() => verify(f.plan, { ...env, ImageVersion: 'different-image' }));
    assert.throws(() => verify(f.plan, env, { ...runtime, node: { node: '25' } }));
    assert.throws(() => verify(f.plan, env, runtime, '999'));
    f.manifest.event = 'pull_request'; f.save(); assert.throws(() => verify());
    f.manifest.event = 'push'; f.save();
    const file = component === 'npm' ? 'npm-linux-x64/candidate.json' : 'candidate/parity.json';
    const original = JSON.parse(fs.readFileSync(path.join(f.directory, file)));
    f.write(file, { ...original, revision: CANDIDATE });
    assert.throws(() => verify(), /reports are missing or changed/);
    // Even if a malformed report is included in a hash index, it cannot
    // establish completeness, and reports cannot be relabeled as fresh.
    f.manifest.files = files(f.directory); f.save();
    assert.throws(() => verify(), /origin or environment mismatch/);
    f.write(file, { ...original, cases: original.cases.slice(1) });
    f.manifest.files = files(f.directory); f.save();
    assert.throws(() => verify(), /Incomplete or failing/);
    fs.rmSync(path.join(f.directory, 'maintainability.json'));
    f.manifest.files = files(f.directory); f.save();
    assert.throws(() => verify(), /missing or changed/);
  }
});

test('mixed-revision parity is rejected by default and only accepts the explicit unchanged component origin', () => {
  for (const component of ['npm', 'pyguard']) {
    const ts = report('candidate', tsIds, component === 'npm' ? BASE : CANDIDATE);
    const pyRevision = component === 'pyguard' ? BASE : CANDIDATE;
    const behavior = report('pyguard.behavior', pyIds('behavior'), pyRevision);
    const python = report('pyguard.parity', pyIds('parity'), pyRevision);
    const reuse = { component, base: BASE, revision: CANDIDATE, tree: TREE, run_id: '123' };
    assert.throws(() => parity(ts, behavior, python), /same candidate revision/);
    const result = parity(ts, behavior, python, { reuse });
    assert.equal(result.revision, CANDIDATE);
    assert.equal(result.shared_capabilities_ready, true);
    assert.deepEqual(result.reused_evidence, reuse);
    assert.throws(() => parity(ts, behavior, python, { reuse: { ...reuse, component: component === 'npm' ? 'pyguard' : 'npm' } }));
    assert.throws(() => parity(ts, behavior, python, { reuse: { ...reuse, run_id: 'invalid' } }));
    assert.throws(() => require('../evals/level1-ready.cjs').assess(ts, behavior, python, { strict: true, reuse }), /exact-candidate evidence/);
  }
});
