const assert = require('node:assert/strict');
const path = require('node:path');
const { summarize } = require(path.join(process.env.P0_BUILD_DIR, 'summary.js'));
const input = { west: [{ orderId: 'a', lines: [{ sku: 's', quantity: 2, price: 3 }] }], east: [] };
assert.deepEqual(summarize(input), { west: 6, east: 0 });
assert.deepEqual(input, { west: [{ orderId: 'a', lines: [{ sku: 's', quantity: 2, price: 3 }] }], east: [] });
assert.deepEqual(summarize({}), {});
