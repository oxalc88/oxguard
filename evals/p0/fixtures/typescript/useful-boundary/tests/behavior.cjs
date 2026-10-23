const assert = require('node:assert/strict');
const path = require('node:path');
const { handle } = require(path.join(process.env.P0_BUILD_DIR, 'handler.js'));
(async () => {
  const payloads = [];
  const transport = { async send(payload) { payloads.push(payload); return 'receipt'; } };
  assert.deepEqual(await handle(transport, 'abc'), { orderId: 'abc', receipt: 'receipt' });
  assert.deepEqual(payloads.map(JSON.parse), [{ order_id: 'abc', version: 1 }]);
  await assert.rejects(() => handle(transport, ' '), /order id required/);
  const cause = new Error('network');
  await assert.rejects(() => handle({ async send() { throw cause; } }, 'abc'), error => error.message === 'delivery unavailable' && error.cause === cause);
})().catch(error => { console.error(error); process.exitCode = 1; });
