'use strict';

// Generate detector input only inside temporary eval projects. This is deliberately
// invalid key material: a repeated alphabet, not an encoded cryptographic key.
function generatedInput(kind) {
  if (kind !== 'synthetic_private_key') throw new Error(`Unknown generated eval input: ${kind}`);
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwx0123456789+/';
  const payload = 'MI' + alphabet.repeat(3);
  const label = ['RSA', 'PRIVATE', 'KEY'].join(' ');
  const marker = phase => ['-----', phase, ' ', label, '-----'].join('');
  return { content: `${marker('BEGIN')}\n${payload}\n${marker('END')}\n`, forbiddenText: payload };
}

module.exports = { generatedInput };
