'use strict';

// Build-time only: installations never download native engines in lifecycle hooks.
const fs = require('node:fs');
const path = require('node:path');
const { createHash } = require('node:crypto');
const { inflateRawSync } = require('node:zlib');
const manifest = require('./native-tools.json');
const hash = data => createHash('sha256').update(data).digest('hex');
const MAX_ARCHIVE = 220 * 1024 * 1024;

async function download(url, expected, size) {
  const response = await fetch(url, { signal: AbortSignal.timeout(180000) });
  if (!response.ok) throw new Error(`${response.status} downloading ${url}`);
  const chunks = [];
  let length = 0;
  for await (const chunk of response.body) {
    length += chunk.length;
    if (length > (size || MAX_ARCHIVE)) throw new Error(`Oversized download: ${url}`);
    chunks.push(chunk);
  }
  const data = Buffer.concat(chunks);
  if ((size && data.length !== size) || hash(data) !== expected) throw new Error(`Checksum/size mismatch: ${url}`);
  return data;
}

// Only exact, flat, pinned regular files are accepted. No paths are extracted.
// Validate the archive digest before parsing, then each decompressed file before writing.
function extractZip(data, asset) {
  if (data.length !== asset.size || data.length > MAX_ARCHIVE || hash(data) !== asset.sha256) throw new Error('ZIP checksum/size mismatch');
  const end = data.length - 22;
  if (end < 0 || data.readUInt32LE(end) !== 0x06054b50 || data.readUInt16LE(end + 20) !== 0 ||
      data.readUInt16LE(end + 4) !== 0 || data.readUInt16LE(end + 6) !== 0) throw new Error('Unsupported ZIP directory');
  const count = data.readUInt16LE(end + 10);
  let offset = data.readUInt32LE(end + 16);
  if (count !== Object.keys(asset.files).length || count > 16 || data.readUInt16LE(end + 8) !== count ||
      offset + data.readUInt32LE(end + 12) !== end) throw new Error('Invalid ZIP directory');
  const files = new Map();
  let total = 0;
  for (let index = 0; index < count; index++) {
    if (offset + 46 > end || data.readUInt32LE(offset) !== 0x02014b50) throw new Error('Invalid ZIP entry');
    const flags = data.readUInt16LE(offset + 8), method = data.readUInt16LE(offset + 10);
    const compressed = data.readUInt32LE(offset + 20), size = data.readUInt32LE(offset + 24);
    const length = data.readUInt16LE(offset + 28), extra = data.readUInt16LE(offset + 30), comment = data.readUInt16LE(offset + 32);
    const mode = data.readUInt32LE(offset + 38) >>> 16;
    const local = data.readUInt32LE(offset + 42);
    if (offset + 46 + length + extra + comment > end) throw new Error('Truncated ZIP entry');
    const name = data.subarray(offset + 46, offset + 46 + length).toString('utf8');
    const pin = Object.hasOwn(asset.files, name) && asset.files[name];
    if (!/^[a-zA-Z0-9_.+-]+$/.test(name) || !pin || files.has(name) ||
        ((mode & 0o170000) !== 0 && (mode & 0o170000) !== 0o100000) || (flags & 1) ||
        ![0, 8].includes(method) || size !== pin.size || (total += size) > MAX_ARCHIVE) throw new Error('Unsafe ZIP entry');
    if (local + 30 > offset || data.readUInt32LE(local) !== 0x04034b50 ||
        data.readUInt16LE(local + 6) !== flags || data.readUInt16LE(local + 8) !== method ||
        data.readUInt16LE(local + 26) !== length ||
        !data.subarray(local + 30, local + 30 + length).equals(Buffer.from(name))) throw new Error('Invalid local ZIP entry');
    const start = local + 30 + length + data.readUInt16LE(local + 28);
    if (start + compressed > offset) throw new Error('Invalid ZIP data bounds');
    const packed = data.subarray(start, start + compressed);
    const content = method === 0 ? packed : inflateRawSync(packed, { maxOutputLength: pin.size });
    if (content.length !== pin.size || hash(content) !== pin.sha256) throw new Error('Extracted file checksum/size mismatch');
    files.set(name, content);
    offset += 46 + length + extra + comment;
  }
  if (offset !== end) throw new Error('Invalid ZIP directory size');
  return files;
}

async function fetchNative(output, platform) {
  if (!Object.hasOwn(manifest.platforms, platform)) throw new Error(`Unsupported platform: ${platform}`);
  const asset = manifest.platforms[platform];
  const directory = path.join(output, platform);
  fs.mkdirSync(directory, { recursive: true });
  const valid = Object.entries(asset.files).every(([name, pin]) => {
    const file = path.join(directory, name);
    return fs.existsSync(file) && fs.lstatSync(file).isFile() && fs.statSync(file).size === pin.size && hash(fs.readFileSync(file)) === pin.sha256;
  });
  const staging = fs.mkdtempSync(path.join(directory, '.opengrep-'));
  try {
    if (!valid) {
      const data = await download(`https://github.com/opengrep/opengrep/releases/download/${manifest.version}/${asset.asset}`, asset.sha256, asset.size);
      const files = asset.asset.endsWith('.zip') ? extractZip(data, asset) : new Map([['opengrep', data]]);
      for (const [name, content] of files) {
        const pin = asset.files[name];
        if (!pin || content.length !== pin.size || hash(content) !== pin.sha256) throw new Error('Executable checksum/size mismatch');
        fs.writeFileSync(path.join(staging, name), content, { mode: 0o755, flag: 'wx' });
      }
      for (const name of files.keys()) fs.renameSync(path.join(staging, name), path.join(directory, name));
    }
    const license = await download(`https://raw.githubusercontent.com/opengrep/opengrep/${manifest.version}/LICENSE`, manifest.licenseSha256);
    fs.mkdirSync(path.join(directory, 'licenses'), { recursive: true });
    fs.writeFileSync(path.join(directory, 'licenses', 'OPENGREP-LICENSE'), license);
    fs.writeFileSync(path.join(directory, 'licenses', 'OPENGREP-SOURCE.txt'),
      `Opengrep ${manifest.version}\nSource: https://github.com/opengrep/opengrep/tree/${manifest.version}\nSource archive: https://github.com/opengrep/opengrep/archive/refs/tags/${manifest.version}.tar.gz\nUnmodified upstream release binaries. See OPENGREP-LICENSE for LGPL-2.1 terms.\n`);
  } finally { fs.rmSync(staging, { recursive: true, force: true }); }
}

if (require.main === module) {
  const [output, platform] = process.argv.slice(2);
  if (!output || !platform) throw new Error('Usage: node npm/fetch-native.cjs <binaries-directory> <platform>');
  fetchNative(path.resolve(output), platform).catch(error => { console.error(error.message); process.exitCode = 1; });
}
module.exports = { fetchNative, extractZip };
