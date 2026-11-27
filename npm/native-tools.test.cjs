'use strict';
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { createHash } = require('node:crypto');
const { extractZip } = require('./fetch-native.cjs');
const hash = data => createHash('sha256').update(data).digest('hex');
function fixture(names = ['opengrep.exe'], mode = 0o100755) {
  const data = Buffer.from('trusted executable');
  const local = [], central = [];
  let offset = 0;
  for (const name of names) {
    const n = Buffer.from(name), l = Buffer.alloc(30), c = Buffer.alloc(46);
    l.writeUInt32LE(0x04034b50); l.writeUInt32LE(data.length, 18); l.writeUInt32LE(data.length, 22); l.writeUInt16LE(n.length, 26);
    c.writeUInt32LE(0x02014b50); c.writeUInt32LE(data.length, 20); c.writeUInt32LE(data.length, 24); c.writeUInt16LE(n.length, 28); c.writeUInt32LE((mode * 65536) >>> 0, 38); c.writeUInt32LE(offset, 42);
    local.push(l, n, data); central.push(c, n); offset += l.length+n.length+data.length;
  }
  const directory = Buffer.concat(central), end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50); end.writeUInt16LE(names.length, 8); end.writeUInt16LE(names.length, 10); end.writeUInt32LE(directory.length, 12); end.writeUInt32LE(offset, 16);
  const zip = Buffer.concat([...local, directory, end]);
  return { zip, asset: { sha256: hash(zip), size: zip.length, files: { 'opengrep.exe': { sha256: hash(data), size: data.length } } } };
}
test('ZIP extraction verifies archive, paths, regular files, sizes and executable bytes', () => {
  const { zip, asset } = fixture();
  assert.equal(extractZip(zip, asset).get('opengrep.exe').toString(), 'trusted executable');
  assert.throws(() => extractZip(zip, { ...asset, sha256: 'invalid' }), /checksum/);
  assert.throws(() => extractZip(zip, { ...asset, files: { ...asset.files, 'runtime.dll': asset.files['opengrep.exe'] } }), /directory/);
  assert.throws(() => extractZip(zip, { ...asset, files: { 'opengrep.exe': { ...asset.files['opengrep.exe'], sha256: 'invalid' } } }), /checksum/);
  assert.throws(() => extractZip(zip, { ...asset, files: { 'opengrep.exe': { ...asset.files['opengrep.exe'], size: 300000000 } } }), /Unsafe/);
  for (const name of ['../opengrep.exe', 'C:opengrep.exe', 'dir/opengrep.exe', 'dir\\opengrep.exe']) {
    const f = fixture([name]);
    f.asset.files = { [name]: asset.files['opengrep.exe'] };
    assert.throws(() => extractZip(f.zip, f.asset), /Unsafe/);
  }
  const link = fixture(['opengrep.exe'], 0o120777);
  assert.throws(() => extractZip(link.zip, link.asset), /Unsafe/);
  const duplicate = fixture(['opengrep.exe', 'opengrep.exe']);
  duplicate.asset.files['runtime.dll'] = asset.files['opengrep.exe'];
  assert.throws(() => extractZip(duplicate.zip, duplicate.asset), /Unsafe/);
});
