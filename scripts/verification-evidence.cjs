'use strict';
// CI evidence only. Never executes downloaded artifacts or selects individual tests.
const fs = require('node:fs');
const path = require('node:path');
const os = require('node:os');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');
const root = path.resolve(__dirname, '..');
const state = path.join(root, '.test-runtime');
const manifestName = 'verification.json';
const hash = value => crypto.createHash('sha256').update(value).digest('hex');
function command(executable, args, cwd = root) {
  return execFileSync(executable, args, { cwd, encoding: 'utf8', shell: process.platform === 'win32' && ['npm', 'pnpm'].includes(executable) }).trim();
}
function output(name, value) {
  if (process.env.GITHUB_OUTPUT) fs.appendFileSync(process.env.GITHUB_OUTPUT, `${name}=${value}\n`);
}
function snapshot(env = process.env) {
  const suite = env.VERIFICATION_SUITE;
  if (!['npm', 'pyguard'].includes(suite)) throw new Error('Unknown verification suite');
  if (command('git', ['status', '--porcelain', '--untracked-files=no'])) throw new Error('Tracked inputs are dirty');
  const module = suite === 'npm' ? 'tsguard' : 'pyguard';
  const inputs = {
    schema: 1, repository: env.GITHUB_REPOSITORY, revision: command('git', ['rev-parse', 'HEAD']),
    tree: command('git', ['rev-parse', 'HEAD^{tree}']), workflow_revision: env.GITHUB_WORKFLOW_SHA, suite, group: env.VERIFICATION_GROUP,
    image_os: env.ImageOS, image_version: env.ImageVersion, runner_environment: env.RUNNER_ENVIRONMENT,
    platform: process.platform, arch: process.arch, kernel: os.release(), node: process.versions,
    go: command('go', ['version'], path.join(root, module)),
    go_environment: JSON.parse(command('go', ['env', '-json', 'GOOS', 'GOARCH', 'CGO_ENABLED', 'GOEXPERIMENT', 'GOFLAGS', 'GOTOOLCHAIN'], path.join(root, module))),
  };
  if (suite === 'npm') {
    inputs.manager = env.TSGUARD_PACKAGE_MANAGER;
    if (!['npm', 'pnpm'].includes(inputs.manager)) throw new Error('Unknown package manager');
    inputs.manager_version = command(inputs.manager, ['--version']);
  } else {
    inputs.python = command(env.EVAL_PY, ['-c', 'import sys; print(sys.version)']);
    inputs.python_packages = JSON.parse(command(env.EVAL_PY, ['-c', 'import importlib.metadata as m,json; print(json.dumps(sorted((d.metadata["Name"], d.version) for d in m.distributions())))']));
  }
  return { inputs, fingerprint: hash(JSON.stringify(inputs)) };
}
function reusableEnvironment(s) {
  return (s.inputs.suite !== 'npm' || s.inputs.manager === 'npm') && s.inputs.runner_environment === 'github-hosted' && Boolean(s.inputs.image_os && s.inputs.image_version && s.inputs.repository && s.inputs.group) &&
    s.inputs.workflow_revision === s.inputs.revision;
}
function trustedRun(run, revision, repository) {
  return run.head_sha === revision && run.event === 'push' && run.head_branch === 'main' &&
    run.path === '.github/workflows/npm.yml' && run.status === 'completed' && run.conclusion === 'success' &&
    run.repository?.full_name === repository;
}
async function api(endpoint, env) {
  const response = await fetch(`https://api.github.com/repos/${env.GITHUB_REPOSITORY}/${endpoint}`, {
    headers: { Authorization: `Bearer ${env.GITHUB_TOKEN}`, Accept: 'application/vnd.github+json', 'X-GitHub-Api-Version': '2022-11-28' },
    signal: AbortSignal.timeout(15000),
  });
  if (!response.ok) throw new Error(`GitHub evidence lookup returned ${response.status}`);
  return response.json();
}
async function lookup(env = process.env, request = api) {
  output('run_id', '');
  const current = snapshot(env);
  fs.mkdirSync(state, { recursive: true });
  fs.writeFileSync(path.join(state, 'verification-inputs.json'), JSON.stringify(current));
  fs.rmSync(path.join(state, 'verification-source.json'), { force: true });
  if (env.REUSE_VERIFICATION !== 'true' || env.GITHUB_EVENT_NAME === 'pull_request' || !reusableEnvironment(current)) return;
  try {
    const runs = await request(`actions/runs?event=push&branch=main&head_sha=${current.inputs.revision}&status=success&per_page=20`, env);
    for (const run of runs.workflow_runs || []) {
      if (!trustedRun(run, current.inputs.revision, env.GITHUB_REPOSITORY)) continue;
      const artifacts = await request(`actions/runs/${run.id}/artifacts?name=${encodeURIComponent(env.VERIFICATION_ARTIFACT)}&per_page=100`, env);
      const artifact = artifacts.artifacts?.find(a => a.name === env.VERIFICATION_ARTIFACT && !a.expired && a.workflow_run?.head_sha === current.inputs.revision);
      if (!artifact) continue;
      fs.writeFileSync(path.join(state, 'verification-source.json'), JSON.stringify({ run_id: String(run.id), artifact_id: artifact.id }));
      output('run_id', String(run.id));
      return;
    }
  } catch (error) {
    console.log(`${error.message}; running existing checks.`);
  }
}
function files(directory, prefix = '') {
  const result = {};
  if (!fs.existsSync(directory)) return result;
  for (const name of fs.readdirSync(directory).sort()) {
    const relative = prefix + name;
    if (relative === manifestName) continue;
    const entry = path.join(directory, name);
    const stat = fs.lstatSync(entry);
    if (stat.isSymbolicLink()) throw new Error('Evidence cannot contain symlinks');
    if (stat.isDirectory()) Object.assign(result, files(entry, relative + '/'));
    else if (stat.isFile()) result[relative] = hash(fs.readFileSync(entry));
    else throw new Error('Unexpected evidence file type');
  }
  return result;
}
function requiredReports(inputs) {
  if (inputs.suite === 'pyguard') return ['candidate/behavior.json', 'candidate/parity.json', 'maintainability.json'];
  return inputs.manager === 'npm' ? [`npm-${inputs.platform}-${inputs.arch}/candidate.json`, 'maintainability.json'] : [];
}
function validate(manifest, current, source, actualFiles) {
  if (!reusableEnvironment(current) || manifest.schema !== 1 || manifest.status !== 'pass' ||
      manifest.run_id !== source.run_id || manifest.event !== 'push' || manifest.ref !== 'refs/heads/main' ||
      manifest.workflow !== '.github/workflows/npm.yml' || manifest.fingerprint !== current.fingerprint ||
      hash(JSON.stringify(manifest.inputs)) !== manifest.fingerprint ||
      JSON.stringify(manifest.inputs) !== JSON.stringify(current.inputs)) throw new Error('Evidence inputs or provenance do not match');
  if (JSON.stringify(manifest.files) !== JSON.stringify(actualFiles) || requiredReports(current.inputs).some(f => !actualFiles[f])) throw new Error('Evidence reports are missing or changed');
  return true;
}
function check(directory) {
  output('hit', 'false');
  try {
    const current = JSON.parse(fs.readFileSync(path.join(state, 'verification-inputs.json')));
    const source = JSON.parse(fs.readFileSync(path.join(state, 'verification-source.json')));
    const manifestFile = path.join(directory, manifestName);
    if (fs.lstatSync(manifestFile).isSymbolicLink()) throw new Error('Evidence manifest cannot be a symlink');
    const manifest = JSON.parse(fs.readFileSync(manifestFile));
    validate(manifest, current, source, files(directory));
    output('hit', 'true');
    console.log(`Reusing trusted main ${current.inputs.suite === 'npm' ? 'regression prechecks (distribution stays fresh)' : 'Python verification'} from run ${source.run_id}.`);
    if (process.env.GITHUB_STEP_SUMMARY) fs.appendFileSync(process.env.GITHUB_STEP_SUMMARY,
      `Verified inputs match [main run ${source.run_id}](https://github.com/${current.inputs.repository}/actions/runs/${source.run_id}); ${current.inputs.suite === 'npm' ? 'Go/Node regression prechecks are reused; distribution stays fresh because npm transitives are unlocked' : 'the hash-pinned Python suite is reused'}.\n`);
  } catch (error) {
    console.log(`${error.message}; running existing checks.`);
    fs.rmSync(directory, { recursive: true, force: true });
  }
}
function record(directory, env = process.env) {
  const current = snapshot(env);
  const initial = JSON.parse(fs.readFileSync(path.join(state, 'verification-inputs.json')));
  if (current.fingerprint !== initial.fingerprint) throw new Error('Verification inputs changed during execution');
  const reports = files(directory);
  if (requiredReports(current.inputs).some(f => !reports[f])) throw new Error('Cannot record incomplete verification reports');
  for (const file of requiredReports(current.inputs).filter(f => f !== 'maintainability.json')) {
    if (JSON.parse(fs.readFileSync(path.join(directory, file))).revision !== current.inputs.revision) throw new Error('Report revision does not match verified source');
  }
  fs.mkdirSync(directory, { recursive: true });
  fs.writeFileSync(path.join(directory, manifestName), JSON.stringify({ schema: 1, status: 'pass',
    run_id: env.GITHUB_RUN_ID, event: env.GITHUB_EVENT_NAME, ref: env.GITHUB_REF,
    workflow: '.github/workflows/npm.yml', ...current, files: reports }, null, 2) + '\n');
}
if (require.main === module) {
  const [mode, directory] = process.argv.slice(2);
  Promise.resolve().then(() => {
    if (mode === 'lookup') return lookup();
    if (mode === 'check' && directory) return check(path.resolve(directory));
    if (mode === 'record' && directory) return record(path.resolve(directory));
    throw new Error('Usage: verification-evidence.cjs lookup | check <reports-dir> | record <reports-dir>');
  }).catch(error => { console.error(error.message); process.exitCode = 1; });
}
module.exports = { lookup, hash, reusableEnvironment, trustedRun, files, requiredReports, validate };
