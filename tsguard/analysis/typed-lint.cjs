// Use mature type-aware rules with an explicit compiler program, including the
// zero-tsconfig case. Do not load or overwrite the consumer's ESLint config.
emitAnalysis(async input => {
  const { program, sources, relative, owned, options } = loadProject(input);
  const { ESLint } = owned('eslint');
  const tseslint = owned('typescript-eslint');
  const rules = {
    '@typescript-eslint/no-unsafe-assignment': 'error',
    '@typescript-eslint/no-unsafe-argument': 'error',
    '@typescript-eslint/no-unsafe-call': 'error',
    '@typescript-eslint/no-unsafe-member-access': 'error',
    '@typescript-eslint/no-unsafe-return': 'error',
    '@typescript-eslint/no-unnecessary-type-assertion': 'error',
  };
  // This rule requires strictNullChecks. Never bypass that prerequisite.
  const strictNullChecks = options.strictNullChecks ?? options.strict ?? false;
  if (strictNullChecks) rules['@typescript-eslint/no-unnecessary-condition'] = 'error';
  const eslint = new ESLint({ overrideConfigFile: true, ignore: false, overrideConfig: [{
    files: ['**/*.{ts,tsx,mts,cts}'],
    languageOptions: { parser: tseslint.parser, parserOptions: { programs: [program] } },
    plugins: { '@typescript-eslint': tseslint.plugin }, rules,
  }] });
  const findings = [];
  const selected = sources.filter(s => /\.(ts|tsx|mts|cts)$/.test(s.fileName));
  for (const source of selected) {
    const reports = await eslint.lintText(source.text, { filePath: source.fileName });
    for (const report of reports) for (const message of report.messages) {
      if (message.fatal || !message.ruleId) throw new Error(message.message);
      findings.push({ rule: message.ruleId, evidence: message.message,
        location: { file: relative(source.fileName), line: message.line, column: message.column }, severity: 'error', status: 'blocking', category: 'quality' });
    }
  }
  return { findings, measurements: [{ metric: 'typed_lint.files', level: 'code', location: { file: '.' }, value: selected.length, unit: 'files' }], partial: !strictNullChecks || selected.length === 0,
    limitation: selected.length === 0 ? 'No TypeScript source files selected; typed lint was not evaluated.' : !strictNullChecks ? 'no-unnecessary-condition not evaluated: project strictNullChecks is disabled.' : '' };
});
