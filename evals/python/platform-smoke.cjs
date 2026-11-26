'use strict';

const { python: behaviorIds, pythonMaintainability: maintainabilityIds } = require('../platform-smoke.cjs');

const [binary, source, python] = process.argv.slice(2);
if (process.argv.length !== 5 || !binary || !source || !python) {
  throw new Error('Usage: node evals/python/platform-smoke.cjs <binary> <source> <python>');
}
const behavior = require('./run.cjs').evaluate({ binary, source, python, caseIds: behaviorIds });
const maintainability = require('./maintainability.cjs').evaluate({ binary, python, caseIds: maintainabilityIds });
const cases = [...behavior.behavior.cases, ...behavior.parity.cases, ...maintainability.cases];
const failed = cases.filter(c => c.outcome !== 'pass');
console.log(JSON.stringify({ suite: 'pyguard.platform-smoke', passed: cases.length - failed.length, failed }, null, 2));
process.exitCode = failed.length ? 1 : 0;
