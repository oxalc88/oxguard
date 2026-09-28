'use strict';

// Build-time only: installations never download native engines in lifecycle hooks.
const fs = require('node:fs');
const path = require('node:path');
const { createHash } = require('node:crypto');
const assets = require('./native-tools.json');
const version = 'v1.23.0';
const LICENSE_SHA256 = '20c17d8b8c48a600800dfd14f95d5cb9ff47066a9641ddeab48dc54aec96e331';

async function download(url, file, sha256) {
  if (fs.existsSync(file) && createHash('sha256').update(fs.readFileSync(file)).digest('hex') === sha256) return;
  const response = await fetch(url, { signal: AbortSignal.timeout(180000) });
  if (!response.ok) throw new Error(`${response.status} downloading ${url}`);
  const data = Buffer.from(await response.arrayBuffer());
  if (createHash('sha256').update(data).digest('hex') !== sha256) throw new Error(`Checksum mismatch: ${url}`);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(`${file}.tmp`, data);
  fs.renameSync(`${file}.tmp`, file);
}

async function fetchNative(output, platform) {
  if (!Object.hasOwn(assets, platform)) throw new Error(`Unsupported platform: ${platform}`);
  const directory = path.join(output, platform);
  for (const [name, asset] of Object.entries(assets[platform])) {
    const file = path.join(directory, name);
    await download(`https://github.com/opengrep/opengrep/releases/download/${version}/${asset.asset}`, file, asset.sha256);
    fs.chmodSync(file, 0o755);
  }
  const license = path.join(directory, 'licenses', 'OPENGREP-LICENSE');
  await download(`https://raw.githubusercontent.com/opengrep/opengrep/${version}/LICENSE`, license, LICENSE_SHA256);
  fs.writeFileSync(path.join(directory, 'licenses', 'OPENGREP-SOURCE.txt'),
    `Opengrep ${version}\nSource: https://github.com/opengrep/opengrep/tree/${version}\nSource archive: https://github.com/opengrep/opengrep/archive/refs/tags/${version}.tar.gz\nUnmodified upstream release binaries. See OPENGREP-LICENSE for LGPL-2.1 terms.\n`);
}

if (require.main === module) {
  const [output, platform] = process.argv.slice(2);
  if (!output || !platform) throw new Error('Usage: node npm/fetch-native.cjs <binaries-directory> <platform>');
  fetchNative(path.resolve(output), platform).catch(error => { console.error(error.message); process.exitCode = 1; });
}
module.exports = { fetchNative };
