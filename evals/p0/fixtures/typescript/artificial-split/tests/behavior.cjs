const assert = require('node:assert/strict');
const path = require('node:path');
const { original } = require(path.join(process.env.P0_BUILD_DIR, 'entry.js'));
for (const [input, expected] of [[-1, 0], [0, 0], [1, 1], [2, 3], [3, 6], [4, 10], [99, 10]]) {
  assert.equal(original(input), expected);
}
