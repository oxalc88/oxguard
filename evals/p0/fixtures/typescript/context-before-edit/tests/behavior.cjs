const assert = require('node:assert/strict');
const path = require('node:path');
const { runBatch } = require(path.join(process.env.P0_BUILD_DIR, 'entry.js'));
(async () => {
  const calls = [];
  const gateway = { async charge(id, amount) { calls.push([id, amount]); if (id === 'retry') throw new Error('unavailable'); return id; } };
  const oldError = console.error;
  console.error = () => {};
  try {
    assert.deepEqual(await runBatch([
      { id: 'paid', amount: 4, alreadyPaid: true },
      { id: 'invalid', amount: 0 },
      { id: 'ok', amount: 5 },
      { id: 'retry', amount: 6 },
      { id: 'last', amount: 7 },
    ], gateway), { paid: ['ok', 'last'], failed: ['invalid', 'retry'] });
    assert.deepEqual(calls, [['ok', 5], ['retry', 6], ['last', 7]]);
  } finally { console.error = oldError; }
})().catch(error => { console.error(error); process.exitCode = 1; });
