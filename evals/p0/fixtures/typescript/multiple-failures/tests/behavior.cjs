const assert = require('node:assert/strict');
const path = require('node:path');
const { quote } = require(path.join(process.env.P0_BUILD_DIR, 'entry.js'));
assert.equal(quote(0, 0), 0);
assert.equal(quote(1, 20), 23);
assert.equal(quote(5, 100), 100);
assert.equal(quote(8, 500), 500);
assert.throws(() => quote(-1, 20), /negative weight/);
assert.throws(() => quote(1, Infinity), /finite total/);
