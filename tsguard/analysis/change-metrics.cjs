'use strict';
// Native metrics from inert, bounded source projections. Never load application
// code, baseline dependencies, package scripts or project analyzer configs.
emitAnalysis(input => {
  const fs = require('node:fs');
  const path = require('node:path');
  const os = require('node:os');
  const crypto = require('node:crypto');
  const { createRequire } = require('node:module');
  const { spawnSync } = require('node:child_process');
  const owned = createRequire(path.join(input.runtime || process.cwd(), input.runtime ? 'bin/tool.cjs' : 'package.json'));
  const ts = owned('typescript');
  const digest = value => crypto.createHash('sha256').update(value).digest('hex');
  const compareText = (a,b) => a < b ? -1 : a > b ? 1 : 0;
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'tsguard-change-metrics-'));
  const policy = { fta_exclude_under: 0, duplicate_min_tokens: 50, duplicate_min_lines: 5,
    cognitive_diagnostic_threshold: 1, source_policy: 'identical pinned analyzers; only graph-selected source; no project configs',
    toolchain: Object.fromEntries(['fta-cli','@biomejs/biome','jscpd'].map(name=>[name,JSON.parse(fs.readFileSync(owned.resolve(`${name}/package.json`),'utf8')).version])) };
  function run(tool, args, cwd) {
    const pkg = owned.resolve(`${tool}/package.json`);
    const manifest = JSON.parse(fs.readFileSync(pkg, 'utf8'));
    const binName = tool === '@biomejs/biome' ? 'biome' : tool === 'fta-cli' ? 'fta' : tool;
    let executable = path.join(path.dirname(pkg), typeof manifest.bin === 'string' ? manifest.bin : manifest.bin[binName]);
    let command = process.execPath;
    if (tool === 'fta-cli') {
      const targets = { 'linux-x64': 'x86_64-unknown-linux-musl', 'linux-arm64': 'aarch64-unknown-linux-musl', 'darwin-x64': 'x86_64-apple-darwin', 'darwin-arm64': 'aarch64-apple-darwin', 'win32-x64': 'x86_64-pc-windows-msvc' };
      const target = targets[`${process.platform}-${process.arch}`];
      if (!target) throw new Error('Unsupported FTA platform');
      executable = path.join(path.dirname(pkg), 'binaries', `fta-${target}`, process.platform === 'win32' ? 'fta.exe' : 'fta');
      command = executable;
    }
    const result = spawnSync(command, tool === 'fta-cli' ? args : [executable, ...args], {
      cwd, encoding: 'utf8', timeout: input.timeout * 1000, maxBuffer: 64 * 1024 * 1024,
      env: { ...process.env, NO_COLOR: '1', JSCPD_NO_TIPS: '1' },
    });
    if (result.error) throw Object.assign(result.error, { category: result.error.code === 'ETIMEDOUT' ? 'timeout' : result.error.code === 'ENOENT' ? 'tool_missing' : 'analyzer_failure' });
    if (result.status !== 0) throw new Error(`${tool} failed (${result.status}): ${result.stderr.slice(0, 2048)}`);
    return result.stdout;
  }
  function fingerprint(fragment) {
    const scanner = ts.createScanner(ts.ScriptTarget.Latest, true, ts.LanguageVariant.JSX, fragment);
    const tokens = [];
    while (scanner.scan() !== ts.SyntaxKind.EndOfFileToken) tokens.push(scanner.getTokenText());
    return digest(JSON.stringify(tokens));
  }
  function profile(label, root, snapshot) {
    const directory = path.join(temporary, label);
    const source = path.join(directory, 'source');
    fs.mkdirSync(source, { recursive: true });
    const aliases = new Map(), contents = new Map();
    let bytes = 0;
    if (snapshot.modules.length > 20000) throw new Error('Source metrics exceed 20000 files');
    for (const [index, module] of snapshot.modules.entries()) {
      const file = path.resolve(root, module.id);
      const rel = path.relative(root, file);
      if (path.isAbsolute(rel) || rel.startsWith('..' + path.sep) || rel === '..' || fs.lstatSync(file).isSymbolicLink()) throw new Error('Unsafe metric source path');
      const real = path.relative(fs.realpathSync(root), fs.realpathSync(file));
      if (path.isAbsolute(real) || real.startsWith('..' + path.sep)) throw new Error('Metric source escapes project root');
      const buffer = fs.readFileSync(file);
      bytes += buffer.length;
      if (buffer.length > 8 * 1024 * 1024 || bytes > 128 * 1024 * 1024) throw new Error('Source metrics exceed analysis budget');
      const text = buffer.toString('utf8');
      if (!module.source_hash || digest(text) !== module.source_hash) throw new Error('Source changed after structural analysis; rerun change');
      const extension = /\.(?:tsx|jsx)$/.exec(module.id)?.[0] || (/\.(?:ts|mts|cts)$/.test(module.id) ? '.ts' : '.js');
      const alias = `source-${String(index).padStart(5, '0')}${extension}`;
      fs.writeFileSync(path.join(source, alias), text);
      aliases.set(alias, module);
      contents.set(module.id, text);
    }
    const result = { files: [], functions: [], duplicates: { clones: 0, tokens: 0, lines: 0, percentage: 0, groups: [] }, cognitive_complete: true };
    if (!aliases.size) return result;
    fs.writeFileSync(path.join(directory, 'fta.json'), '{}');
    const files = JSON.parse(run('fta-cli', ['--json', '--exclude-under', '0', '--score-cap', '1000000000', '--config-path', path.join(directory, 'fta.json'), source], directory));
    if (!Array.isArray(files) || files.length !== aliases.size) throw new Error('FTA did not measure every selected source file');
    const seen = new Set();
    for (const file of files) {
      const module = aliases.get(path.basename(file.file_name));
      if (!module || seen.has(module.id) || ![file.fta_score, file.cyclo, file.halstead?.volume, file.line_count].every(n => Number.isFinite(n) && n >= 0)) throw new Error('Invalid native FTA measurement');
      seen.add(module.id);
      result.files.push({ file: module.id, source_hash: module.source_hash, fta_score: file.fta_score, cyclo: file.cyclo, halstead_volume: file.halstead.volume, lines: file.line_count });
    }
    fs.writeFileSync(path.join(directory, 'biome.json'), JSON.stringify({ root: true, formatter: { enabled: false }, assist: { enabled: false },
      files: { maxSize: 8 * 1024 * 1024 }, linter: { rules: { recommended: false, complexity: { noExcessiveCognitiveComplexity: { level: 'warn', options: { maxAllowedComplexity: 1 } } } } } }));
    const biome = JSON.parse(run('@biomejs/biome', ['lint', '--config-path', directory, '--reporter=json', '--max-diagnostics=none', source], directory));
    if (!Array.isArray(biome.diagnostics) || biome.summary?.skipped || biome.summary?.diagnosticsNotPrinted || biome.summary?.unchanged !== aliases.size) throw new Error('Incomplete cognitive metrics');
    const suppressed = new Set();
    for (const [file, text] of contents) {
      const scanner = ts.createScanner(ts.ScriptTarget.Latest, false, ts.LanguageVariant.JSX, text);
      let token;
      while ((token = scanner.scan()) !== ts.SyntaxKind.EndOfFileToken) {
        if ((token === ts.SyntaxKind.SingleLineCommentTrivia || token === ts.SyntaxKind.MultiLineCommentTrivia) && /biome-ignore(?:-all)?\s+(?:all|lint(?:\s|:|\/complexity(?:\s|:|\/noExcessiveCognitiveComplexity)))/.test(scanner.getTokenText())) suppressed.add(file);
      }
    }
    result.functions = snapshot.functions.map(f => ({ id: f.id, location: f.location, fingerprint: f.fingerprint,
      cognitive_min: 0, cognitive_max: suppressed.has(f.location.file) ? null : 1 }));
    result.cognitive_complete = suppressed.size === 0;
    for (const d of biome.diagnostics) {
      if (d.category !== 'lint/complexity/noExcessiveCognitiveComplexity') throw new Error('Unexpected cognitive diagnostic');
      const match = /^Excessive complexity of (\d+) detected \(max: 1\)\.$/.exec(d.message);
      const module = aliases.get(path.basename(d.location?.path || ''));
      const point = d.location?.start;
      if (!match || !module || !point) throw new Error('Unsupported native cognitive diagnostic');
      const candidates = snapshot.functions.filter(f => f.location.file === module.id &&
        (f.location.line < point.line || f.location.line === point.line && f.location.column <= point.column) &&
        (f.end_line > point.line || f.end_line === point.line && f.end_column >= point.column));
      candidates.sort((a,b) => b.location.line - a.location.line || b.location.column - a.location.column);
      const fact = candidates[0];
      if (!fact) throw new Error('Cognitive diagnostic has no matching function');
      const functionMetric = result.functions.find(f => f.id === fact.id);
      functionMetric.cognitive_min = functionMetric.cognitive_max = Number(match[1]);
    }
    const output = path.join(directory, 'duplicates');
    fs.writeFileSync(path.join(directory, 'jscpd.json'), JSON.stringify({ minTokens: 50, minLines: 5, mode: 'mild' }));
    run('jscpd', ['--config', path.join(directory, 'jscpd.json'), '--reporters', 'json', '--output', output, source], directory);
    const duplicates = JSON.parse(fs.readFileSync(path.join(output, 'jscpd-report.json'), 'utf8'));
    const total = duplicates.statistics?.total;
    if (!Array.isArray(duplicates.duplicates) || ![total?.clones, total?.duplicatedTokens, total?.duplicatedLines, total?.percentage].every(n => Number.isFinite(n) && n >= 0)) throw new Error('Invalid native duplication measurement');
    const groups = new Map();
    for (const clone of duplicates.duplicates) {
      if (typeof clone.fragment !== 'string') throw new Error('Missing native clone fragment');
      const key = fingerprint(clone.fragment);
      const group = groups.get(key) || { fingerprint: key, count: 0, locations: [] };
      group.count++;
      for (const part of [clone.firstFile, clone.secondFile]) {
        const module = aliases.get(path.basename(part?.name || ''));
        if (!module || !Number.isInteger(part.startLoc?.line)) throw new Error('Invalid native clone location');
        group.locations.push({ file: module.id, line: part.startLoc.line, column: part.startLoc.column + 1 });
      }
      groups.set(key, group);
    }
    for (const group of groups.values()) group.locations.sort((a,b)=>compareText(a.file,b.file)||a.line-b.line||a.column-b.column);
    result.duplicates = { clones: total.clones, tokens: total.duplicatedTokens, lines: total.duplicatedLines, percentage: total.percentage, groups: [...groups.values()].sort((a,b) => compareText(a.fingerprint,b.fingerprint)) };
    result.files.sort((a,b) => compareText(a.file,b.file));
    return result;
  }
  try {
    const baseline = profile('baseline', input.baselineRoot, input.baselineSnapshot);
    const candidate = profile('candidate', process.cwd(), input.candidateSnapshot);
    return { findings: [], measurements: [], partial: !baseline.cognitive_complete || !candidate.cognitive_complete,
      limitation: 'Inline Biome cognitive suppressions prevent complete cognitive measurement; structural, FTA and duplication comparisons remain available.',
      comparison_metrics: { baseline, candidate, policy } };
  } finally { fs.rmSync(temporary, { recursive: true, force: true }); }
});
