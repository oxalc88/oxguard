'use strict';
// Machine-readable runner events; no parsing of display text or test source.
module.exports = async function* (events) {
  for await (const { type, data } of events) {
    if (['test:pass', 'test:fail'].includes(type)) {
      yield JSON.stringify({ name: data.name, file: data.file, nesting: data.nesting,
        status: data.skip || data.todo ? 'skipped' : type === 'test:pass' ? 'pass' : 'fail',
        duration_ms: data.details?.duration_ms }) + '\n';
    }
  }
};
