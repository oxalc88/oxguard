const assert = require('node:assert/strict');
const path = require('node:path');
const { summarize } = require(path.join(process.env.P0_BUILD_DIR, 'entry.js'));
const branches = Number(process.env.P0_BRANCHES || 8);
const inputs = [Number.NEGATIVE_INFINITY, Number.POSITIVE_INFINITY, Number.NaN];
for (let value = -branches - 2; value <= branches + 2; value += 0.5) inputs.push(value);
for (const x of inputs) {
  let expected = 0;
  for (let threshold = 0; threshold < branches; threshold++) {
    if (x > threshold) expected += threshold + 1;
    if (x < -threshold) expected -= threshold + 1;
  }
  assert.equal(summarize(x), expected);
}
