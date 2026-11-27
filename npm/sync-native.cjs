'use strict';
// Run after changing the authoritative manifest; Go embeds this generated copy.
const fs = require('node:fs');
const path = require('node:path');
fs.copyFileSync(path.join(__dirname, 'native-tools.json'), path.join(__dirname, '../tsguard/native-tools.generated.json'));
