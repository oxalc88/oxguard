'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { hash, trustedRun, reusableEnvironment, validate, files, requiredReports } = require('./verification-evidence.cjs');
function fixture() {
  const inputs = { repository: 'owner/repo', revision: 'a'.repeat(40), workflow_revision: 'a'.repeat(40), tree: 'b'.repeat(40), suite: 'npm',
    group: 'npm-linux-npm', runner_environment: 'github-hosted', image_os: 'ubuntu24', image_version: '20261015',
    platform: 'linux', arch: 'x64', manager: 'npm', manager_version: '11', node: '24', go: '1.25' };
  const current = { inputs, fingerprint: hash(JSON.stringify(inputs)) };
  const actual = { 'maintainability.json': 'hash1', 'npm-linux-x64/candidate.json': 'hash2' };
  return { current, actual, source: { run_id: '123' }, manifest: { schema: 1, status: 'pass', run_id: '123',
    event: 'push', ref: 'refs/heads/main', workflow: '.github/workflows/npm.yml', ...current, files: actual } };
}
test('only a successful exact-candidate main workflow is a trusted producer', () => {
  const good = { head_sha: 'sha', event: 'push', head_branch: 'main', path: '.github/workflows/npm.yml',
    status: 'completed', conclusion: 'success', repository: { full_name: 'owner/repo' } };
  assert.equal(trustedRun(good, 'sha', 'owner/repo'), true);
  for (const changes of [{ head_sha: 'other' }, { event: 'pull_request' }, { head_branch: 'feature' },
    { status: 'in_progress' }, { conclusion: 'failure' }, { conclusion: 'cancelled' },
    { path: '.github/workflows/release.yml' }, { repository: { full_name: 'attacker/repo' } }]) {
    assert.equal(trustedRun({ ...good, ...changes }, 'sha', 'owner/repo'), false);
  }
});
test('valid complete evidence is reusable, unknown and inconsistent states are rejected', () => {
  const f = fixture();
  assert.equal(validate(f.manifest, f.current, f.source, f.actual), true);
  for (const changes of [{ schema: 2 }, { status: 'unknown' }, { status: 'fail' }, { run_id: '999' },
    { event: 'pull_request' }, { ref: 'refs/tags/v1.0.0' }, { workflow: 'release.yml' },
    { fingerprint: 'forged' }, { files: {} }]) {
    assert.throws(() => validate({ ...f.manifest, ...changes }, f.current, f.source, f.actual));
  }
});
test('source, tests, configuration, dependencies, toolchain, environment and group changes invalidate evidence', () => {
  const f = fixture();
  for (const [key, value] of Object.entries({ revision: 'changed', tree: 'changed', suite: 'pyguard',
    group: 'other', image_version: 'new', platform: 'win32', arch: 'arm64', manager_version: '12',
    node: '26', go: '1.26', repository: 'other/repo' })) {
    const inputs = { ...f.current.inputs, [key]: value };
    assert.throws(() => validate(f.manifest, { inputs, fingerprint: hash(JSON.stringify(inputs)) }, f.source, f.actual));
  }
  const forged = { ...f.manifest, inputs: { ...f.manifest.inputs, tree: 'changed' } };
  assert.throws(() => validate(forged, f.current, f.source, f.actual));
});
test('missing runner identity, missing reports and corrupted reports force verification', () => {
  const f = fixture();
  for (const change of [{ workflow_revision: 'different-workflow-code' }, { runner_environment: 'self-hosted' }, { image_version: '' }, { image_os: '' }]) {
    assert.equal(reusableEnvironment({ inputs: { ...f.current.inputs, ...change } }), false);
  }
  assert.throws(() => validate(f.manifest, f.current, f.source, {}));
  assert.throws(() => validate(f.manifest, f.current, f.source, { ...f.actual, 'maintainability.json': 'corrupt' }));
  assert.throws(() => validate({ ...f.manifest, files: {} }, f.current, f.source, {}));
});
test('required reports cover the maintainability evidence of each suite', () => {
  assert.deepEqual(requiredReports({ suite: 'pyguard' }), ['candidate/behavior.json', 'candidate/parity.json', 'maintainability.json']);
  assert.deepEqual(requiredReports({ suite: 'npm', manager: 'npm', platform: 'linux', arch: 'x64' }), ['npm-linux-x64/candidate.json', 'maintainability.json']);
  assert.deepEqual(requiredReports({ suite: 'npm', manager: 'pnpm' }), []);
});
test('report hashing covers nested payloads and detects tampering', t => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'oxguard-evidence-'));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  fs.mkdirSync(path.join(dir, 'nested'));
  fs.writeFileSync(path.join(dir, 'nested/report.json'), 'green');
  fs.writeFileSync(path.join(dir, 'verification.json'), '{}');
  const first = files(dir);
  assert.deepEqual(Object.keys(first), ['nested/report.json']);
  fs.writeFileSync(path.join(dir, 'nested/report.json'), 'changed');
  assert.notDeepEqual(files(dir), first);
});
