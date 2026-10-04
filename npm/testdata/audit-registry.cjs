'use strict';

// A deterministic advisory provider for launcher/contract integration tests.
// Real npm/pnpm audit and audit-ci still execute; installs use the real registry.
const http = require('node:http');
const server = http.createServer((request, response) => {
  request.resume();
  response.setHeader('content-type', 'application/json');
  if (request.url === '/-/npm/v1/security/advisories/bulk') {
    response.end('{}');
  } else if (request.url === '/-/npm/v1/security/audits/quick') {
    response.end(JSON.stringify({ advisories: {}, metadata: { vulnerabilities: { info: 0, low: 0, moderate: 0, high: 0, critical: 0 } } }));
  } else {
    response.statusCode = 404;
    response.end('{}');
  }
});
server.listen(0, '127.0.0.1', () => process.send({ registry: `http://127.0.0.1:${server.address().port}` }));
