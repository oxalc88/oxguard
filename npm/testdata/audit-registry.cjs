'use strict';

// A deterministic advisory provider for launcher/contract integration tests.
// Real npm/pnpm audit and audit-ci still execute; installs use the real registry.
const http = require('node:http');
const server = http.createServer((request, response) => {
  request.resume();
  response.setHeader('content-type', 'application/json');
  const vulnerable = request.url.startsWith('/vulnerable/') || request.url.startsWith('/low/');
  const low = request.url.startsWith('/low/');
  const route = request.url.replace(/^\/(?:clean|vulnerable|low)\//, '/');
  if (route === '/-/npm/v1/security/advisories/bulk') {
    response.end(JSON.stringify(vulnerable ? {
      'oxguard-eval-dependency': [{ id: 1234567, url: 'https://github.com/advisories/GHSA-xxxx-yyyy-zzzz', title: 'Controlled eval advisory', severity: low ? 'low' : 'high', vulnerable_versions: '<2.0.0', cwe: ['CWE-79'], cvss: { score: low ? 2.0 : 7.5, vectorString: null } }],
    } : {}));
  } else if (route === '/oxguard-eval-dependency') {
    response.end(JSON.stringify({ name: 'oxguard-eval-dependency', 'dist-tags': { latest: '2.0.0' }, versions: { '1.0.0': { name: 'oxguard-eval-dependency', version: '1.0.0' }, '2.0.0': { name: 'oxguard-eval-dependency', version: '2.0.0' } } }));
  } else if (route === '/-/npm/v1/security/audits/quick') {
    response.end(JSON.stringify({ advisories: {}, metadata: { vulnerabilities: { info: 0, low: 0, moderate: 0, high: 0, critical: 0 } } }));
  } else {
    response.statusCode = 404;
    response.end('{}');
  }
});
server.listen(0, '127.0.0.1', () => process.send({ registry: `http://127.0.0.1:${server.address().port}` }));
