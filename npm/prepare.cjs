'use strict';

const fs = require('node:fs');
const path = require('node:path');

const platforms = ['linux-x64', 'linux-arm64', 'darwin-x64', 'darwin-arm64', 'win32-x64'];
function prepare(tag, binaries, output, selected = platforms) {
  const version = tag.replace(/^v/, '');
  // Release tags intentionally support stable and prerelease versions, not build metadata.
  if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$/.test(version) ||
      version.split('-').slice(1).join('-').split('.').some(part => /^0\d+$/.test(part))) {
    throw new Error(`Invalid release version: ${tag}`);
  }
  for (const platform of selected) {
    if (!platforms.includes(platform)) throw new Error(`Unsupported platform: ${platform}`);
    const bin = platform.startsWith('win32') ? 'tsguard.exe' : 'tsguard';
    if (!fs.statSync(path.join(binaries, platform, bin)).isFile()) throw new Error(`Missing binary: ${platform}`);
  }
  const common = {
    version,
    description: 'Native Go quality gate CLI for TypeScript projects',
    repository: { type: 'git', url: 'git+https://github.com/oxalc88/oxguard.git' },
    publishConfig: { access: 'public' },
  };
  function writePackage(directory, manifest) {
    fs.mkdirSync(directory, { recursive: true });
    fs.writeFileSync(path.join(directory, 'package.json'), JSON.stringify(manifest, null, 2) + '\n');
    fs.copyFileSync(path.join(__dirname, 'README.md'), path.join(directory, 'README.md'));
  }
  const main = path.join(output, 'tsguard');
  writePackage(main, {
    ...common, name: '@oxguard/tsguard', engines: { node: '^22.13.0 || ^24.0.0 || >=26.0.0' },
    bin: { tsguard: 'bin/tsguard.cjs' }, files: ['bin', 'config'],
    dependencies: require('./toolchain.json'),
    optionalDependencies: Object.fromEntries(platforms.map(p => [`@oxguard/tsguard-${p}`, version])),
  });
  fs.mkdirSync(path.join(main, 'bin'), { recursive: true });
  fs.cpSync(path.join(__dirname, 'tsguard/bin'), path.join(main, 'bin'), { recursive: true });
  fs.cpSync(path.join(__dirname, 'tsguard/config'), path.join(main, 'config'), { recursive: true });
  fs.chmodSync(path.join(main, 'bin/tsguard.cjs'), 0o755);
  for (const platform of selected) {
    const [os, cpu] = platform.split('-');
    const directory = path.join(output, `tsguard-${platform}`);
    writePackage(directory, { ...common, name: `@oxguard/tsguard-${platform}`, os: [os], cpu: [cpu], files: ['bin', 'licenses'] });
    const bin = os === 'win32' ? 'tsguard.exe' : 'tsguard';
    fs.mkdirSync(path.join(directory, 'bin'), { recursive: true });
    fs.copyFileSync(path.join(binaries, platform, bin), path.join(directory, 'bin', bin));
    fs.chmodSync(path.join(directory, 'bin', bin), 0o755);
    for (const engine of [os === 'win32' ? 'opengrep.exe' : 'opengrep', ...(os === 'linux' ? ['opengrep-musl'] : [])]) {
      fs.copyFileSync(path.join(binaries, platform, engine), path.join(directory, 'bin', engine));
      fs.chmodSync(path.join(directory, 'bin', engine), 0o755);
    }
    fs.cpSync(path.join(binaries, platform, 'licenses'), path.join(directory, 'licenses'), { recursive: true });
  }
}

if (require.main === module) {
  const [tag, binaries, output, platform] = process.argv.slice(2);
  if (!tag || !binaries || !output) throw new Error('Usage: node npm/prepare.cjs <tag> <binaries-dir> <output-dir> [single-platform]');
  prepare(tag, path.resolve(binaries), path.resolve(output), platform ? [platform] : platforms);
}
module.exports = { prepare, platforms };
