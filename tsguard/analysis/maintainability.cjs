// Extract compiler facts. Go owns policy and thresholds.
emitAnalysis(input => {
  const { ts, program, sources, relative } = loadProject(input);
  const checker = program.getTypeChecker();
  const crypto = require('node:crypto');
  const declarations = new Map();
  const functions = [];
  const handlers = [];
  const digest = value => crypto.createHash('sha256').update(value).digest('hex');
  const tokenFingerprint = (text, variant) => {
    const scanner = ts.createScanner(ts.ScriptTarget.Latest, true, variant, text);
    const tokens = [];
    while (scanner.scan() !== ts.SyntaxKind.EndOfFileToken) tokens.push(scanner.getTokenText());
    return digest(JSON.stringify(tokens));
  };
  const modules = sources.map(s => ({ id: relative(s.fileName), location: { file: relative(s.fileName) }, source_hash: digest(s.text) }));
  const moduleIDs = new Set(modules.map(m => m.id));
  const imports = new Map();
  let externalImports = 0, unresolvedImports = 0, typeImports = 0;
  const isFunction = node => ts.isFunctionDeclaration(node) || ts.isMethodDeclaration(node) || ts.isConstructorDeclaration(node) || ts.isGetAccessorDeclaration(node) || ts.isSetAccessorDeclaration(node) || ts.isArrowFunction(node) || ts.isFunctionExpression(node);
  const location = (node, source, symbol) => {
    const pos = source.getLineAndCharacterOfPosition(node.getStart(source));
    return { file: relative(source.fileName), line: pos.line + 1, column: pos.character + 1, ...(symbol ? { symbol } : {}) };
  };
  const name = node => node.name?.getText() || (ts.isVariableDeclaration(node.parent) ? node.parent.name.getText() : '<anonymous>');
  function register(node, owners, source) {
    let next = owners;
    if (ts.isClassDeclaration(node) || ts.isClassExpression(node)) next = [...owners, node.name?.text || '<class>'];
    if (isFunction(node) && node.body) {
      const loc = location(node, source, [...owners, name(node)].join('.'));
      const id = `${loc.file}:${loc.line}:${loc.column}:${loc.symbol}`;
      const end = source.getLineAndCharacterOfPosition(node.end);
      const fact = { id, location: loc, end_line: end.line + 1, end_column: end.character + 1,
        fingerprint: tokenFingerprint(node.getText(source), source.languageVariant), forward_target: '', forwarding_mode: '', nested_branches: 0, max_branch_nesting: 0, branch_locations: [] };
      functions.push(fact);
      declarations.set(node, fact);
      next = [...owners, name(node)];
    }
    ts.forEachChild(node, child => register(child, next, source));
  }
  for (const source of sources) register(source, [], source);
  function target(call) {
    const signature = checker.getResolvedSignature(call);
    const declaration = signature?.declaration;
    if (declarations.has(declaration)) return declarations.get(declaration);
    let symbol = checker.getSymbolAtLocation(ts.isPropertyAccessExpression(call.expression) ? call.expression.name : call.expression);
    if (symbol?.flags & ts.SymbolFlags.Alias) symbol = checker.getAliasedSymbol(symbol);
    for (const decl of symbol?.declarations || []) {
      const fact = declarations.get(decl) || declarations.get(decl.initializer);
      if (fact) return fact;
    }
  }
  const edges = new Map();
  let unresolved = 0;
  function visit(node, owner, source) {
    if (isFunction(node)) owner = declarations.get(node);
    if (owner && (ts.isCallExpression(node) || ts.isNewExpression(node))) {
      const callee = target(node);
      if (callee) edges.set(JSON.stringify([owner.id, callee.id]), { caller: owner.id, callee: callee.id });
      else unresolved++;
    }
    if (ts.isCatchClause(node)) {
      // Exact token identity, preserving identifiers/literals and failure policy.
      // Short handlers are below jscpd's normal minimum token threshold.
      const scanner = ts.createScanner(ts.ScriptTarget.Latest, true, source.languageVariant, node.block.getText(source));
      const tokens = [];
      while (scanner.scan() !== ts.SyntaxKind.EndOfFileToken) tokens.push(scanner.getTokenText());
      const statement = node.block.statements.length === 1 ? node.block.statements[0] : undefined;
      const fallback = statement && ts.isReturnStatement(statement) && (!statement.expression || ts.isLiteralExpression(statement.expression) || [ts.SyntaxKind.NullKeyword, ts.SyntaxKind.FalseKeyword, ts.SyntaxKind.TrueKeyword].includes(statement.expression.kind) || ts.isIdentifier(statement.expression) && (!node.variableDeclaration || checker.getSymbolAtLocation(statement.expression) !== checker.getSymbolAtLocation(node.variableDeclaration.name)) || ts.isArrayLiteralExpression(statement.expression) && !statement.expression.elements.length);
      const silentExit = statement && (ts.isContinueStatement(statement) || ts.isBreakStatement(statement));
      handlers.push({ location: location(node, source), tokens: tokens.length, fallback: !!fallback, silent_exit: !!silentExit,
        fingerprint: crypto.createHash('sha256').update(JSON.stringify(tokens)).digest('hex') });
    }
    ts.forEachChild(node, child => visit(child, owner, source));
  }
  for (const [node, fact] of declarations) {
    // Only single-return positional forwarding to a resolved function. Async
    // return/return-await are advisory boundaries, never equivalence claims.
    // Exclude methods, generic/default/rest/optional/destructured parameters.
    let expression = ts.isBlock(node.body) ? (node.body.statements.length === 1 && ts.isReturnStatement(node.body.statements[0]) ? node.body.statements[0].expression : undefined) : node.body;
    if (!(ts.isFunctionDeclaration(node) || ts.isArrowFunction(node) || ts.isFunctionExpression(node)) || node.typeParameters?.length) continue;
    const asyncBoundary = !!node.modifiers?.some(m => m.kind === ts.SyntaxKind.AsyncKeyword);
    const awaited = expression && ts.isAwaitExpression(expression);
    if (awaited) expression = expression.expression;
    if (!expression || !ts.isCallExpression(expression) || !ts.isIdentifier(expression.expression)) continue;
    if (node.parameters.some(p => !ts.isIdentifier(p.name) || p.initializer || p.dotDotDotToken || p.questionToken)) continue;
    if (expression.arguments.length !== node.parameters.length || expression.arguments.some((a, i) => !ts.isIdentifier(a) || checker.getSymbolAtLocation(a) !== checker.getSymbolAtLocation(node.parameters[i].name))) continue;
    const callee = target(expression);
    if (!callee || callee.id === fact.id) continue;
    const callerType = checker.getSignatureFromDeclaration(node);
    const calleeType = checker.getResolvedSignature(expression);
    if (!callerType || !calleeType || checker.typeToString(checker.getReturnTypeOfSignature(callerType)) !== checker.typeToString(checker.getReturnTypeOfSignature(calleeType))) continue;
    fact.forward_target = callee.id;
    fact.forwarding_mode = asyncBoundary ? (awaited ? 'async-return-await' : 'async-return') : 'sync';
  }
  for (const source of sources) visit(source, undefined, source);
  function dependency(node, source) {
    let specifier;
    let typeOnly = false;
    if (ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) {
      specifier = node.moduleSpecifier;
      const clause = node.importClause || node.exportClause;
      typeOnly = node.isTypeOnly || clause?.isTypeOnly || (!clause?.name && clause?.namedBindings && ts.isNamedImports(clause.namedBindings) && clause.namedBindings.elements.length > 0 && clause.namedBindings.elements.every(e => e.isTypeOnly)) || (clause && ts.isNamedExports(clause) && clause.elements.length > 0 && clause.elements.every(e => e.isTypeOnly));
    } else if (ts.isImportEqualsDeclaration(node) && ts.isExternalModuleReference(node.moduleReference)) {
      specifier = node.moduleReference.expression;
      typeOnly = node.isTypeOnly;
    } else if (ts.isCallExpression(node) && node.arguments.length > 0) {
      const expr = node.expression;
      if (expr.kind === ts.SyntaxKind.ImportKeyword) specifier = node.arguments[0];
      if (ts.isIdentifier(expr) && expr.text === 'require') {
        const decl = checker.getSymbolAtLocation(expr)?.valueDeclaration;
        if (!decl || decl.getSourceFile().isDeclarationFile) specifier = node.arguments[0];
      }
    }
    if (specifier) {
      if (typeOnly) typeImports++;
      else if (!ts.isStringLiteralLike(specifier)) unresolvedImports++;
      else {
        const resolved = ts.resolveModuleName(specifier.text, source.fileName, program.getCompilerOptions(), ts.sys).resolvedModule;
        const callee = resolved ? relative(resolved.resolvedFileName) : undefined;
        const caller = relative(source.fileName);
        if (moduleIDs.has(callee)) imports.set(JSON.stringify([caller,callee]), { id: JSON.stringify([caller,callee]), caller, callee });
        else if (!resolved && specifier.text.startsWith('.')) unresolvedImports++;
        else externalImports++;
      }
    }
    ts.forEachChild(node, child => dependency(child, source));
  }
  for (const source of sources) dependency(source, source);
  // Branch counts are compiler facts for comparison, not a new complexity score.
  function count(node, owner, depth) {
    if (isFunction(node)) { owner = declarations.get(node); depth = 0; }
    const branch = ts.isIfStatement(node) || ts.isSwitchStatement(node) || ts.isConditionalExpression(node) || ts.isForStatement(node) || ts.isForOfStatement(node) || ts.isForInStatement(node) || ts.isWhileStatement(node) || ts.isDoStatement(node) || ts.isCatchClause(node);
    if (owner && branch) {
      owner.nested_branches++;
      owner.max_branch_nesting = Math.max(owner.max_branch_nesting, depth + 1);
      owner.branch_locations.push(location(node, node.getSourceFile()));
    }
    ts.forEachChild(node, child => count(child, owner, depth + (branch ? 1 : 0)));
  }
  for (const source of sources) count(source, undefined, 0);
  const failureFindings = collectFailureFindings({ ts, checker, sources, location, isFunction });
  const ordered = values => values.sort((a,b) => JSON.stringify(a) < JSON.stringify(b) ? -1 : JSON.stringify(a) > JSON.stringify(b) ? 1 : 0);
  return { findings: failureFindings, measurements: [], partial: unresolvedImports > 0,
    limitation: unresolvedImports > 0 ? `${unresolvedImports} local or dynamic module references could not be resolved; the graph is partial.` : '',
    snapshot: { compiler_version: ts.version, functions: ordered(functions), calls: ordered([...edges.values()]), handlers: ordered(handlers), unresolved_calls: unresolved,
      modules: ordered(modules), imports: ordered([...imports.values()]), external_imports: externalImports, unresolved_imports: unresolvedImports, type_imports: typeImports } };
});

// Bounded, symbol-aware failure paths. Do not infer project recovery policy.
function collectFailureFindings({ ts, checker, sources, location, isFunction }) {
  const findings = [];
  const add = (rule, node, evidence) => findings.push({ rule: `tsguard.maintainability.${rule}`, level: 'code',
    severity: 'warning', status: 'advisory', category: 'quality', location: location(node, node.getSourceFile()), evidence,
    remediation: 'Check the project recovery policy and retain failure visibility; do not require every recovery path to throw.' });
  const symbol = n => checker.getSymbolAtLocation(n);
  const unwrap = n => n && (ts.isAwaitExpression(n) || ts.isParenthesizedExpression(n)) ? unwrap(n.expression) : n;
  const emptyReturn = n => ts.isReturnStatement(n) && n.expression && ts.isArrayLiteralExpression(n.expression) && n.expression.elements.length === 0;
  function mentionsReason(node, result) {
    if (isFunction(node)) return false;
    if (ts.isPropertyAccessExpression(node) && node.name.text === 'reason' && symbol(node.expression) === result) return true;
    return !!ts.forEachChild(node, child => mentionsReason(child, result));
  }
  function statusCondition(node, result) {
    node = unwrap(node);
    if (!node || !ts.isBinaryExpression(node)) return;
    const left = node.left, right = node.right;
    const access = ts.isPropertyAccessExpression(left) ? left : ts.isPropertyAccessExpression(right) ? right : undefined;
    const literal = access === left ? right : left;
    if (!access || access.name.text !== 'status' || symbol(access.expression) !== result || !ts.isStringLiteral(literal) || !['fulfilled','rejected'].includes(literal.text)) return;
    const equal = [ts.SyntaxKind.EqualsEqualsEqualsToken, ts.SyntaxKind.EqualsEqualsToken].includes(node.operatorToken.kind);
    const unequal = [ts.SyntaxKind.ExclamationEqualsEqualsToken, ts.SyntaxKind.ExclamationEqualsToken].includes(node.operatorToken.kind);
    if (!equal && !unequal) return;
    const positive = equal ? literal.text : literal.text === 'fulfilled' ? 'rejected' : 'fulfilled';
    return [positive, positive === 'fulfilled' ? 'rejected' : 'fulfilled'];
  }
  function settledOrigin(node, seen = new Set()) {
    node = unwrap(node);
    if (!node) return false;
    if (ts.isCallExpression(node) && ts.isPropertyAccessExpression(node.expression) && node.expression.name.text === 'allSettled') {
      const decl = checker.getResolvedSignature(node)?.declaration;
      return !!decl?.getSourceFile().isDeclarationFile && /(?:^|[\\/])lib\..*\.d\.ts$/.test(decl.getSourceFile().fileName);
    }
    if (ts.isIdentifier(node)) {
      const sym = symbol(node);
      if (!sym || seen.has(sym)) return false;
      seen.add(sym);
      const decl = sym.valueDeclaration;
      return !!decl && ts.isVariableDeclaration(decl) && !!(decl.parent.flags & ts.NodeFlags.Const) && !!decl.initializer && settledOrigin(decl.initializer, seen);
    }
    return false;
  }
  function callbackPaths(node, result, state = 'unknown', recorded = false) {
    if (!node || isFunction(node)) return { exits: false, recorded };
    if (ts.isBlock(node)) {
      for (const statement of node.statements) {
        const outcome = callbackPaths(statement, result, state, recorded);
        if (outcome.exits) return outcome;
        state = outcome.state || state;
        recorded = outcome.recorded;
      }
      return { exits: false, recorded, state };
    }
    if (ts.isIfStatement(node)) {
      const status = statusCondition(node.expression, result);
      const inspected = recorded || mentionsReason(node.expression, result);
      const yes = callbackPaths(node.thenStatement, result, status?.[0] || state, inspected);
      const no = node.elseStatement ? callbackPaths(node.elseStatement, result, status?.[1] || state, inspected) : { exits: false, recorded: inspected };
      return { exits: yes.exits && no.exits,
        recorded: yes.exits ? no.recorded : no.exits ? yes.recorded : yes.recorded && no.recorded,
        state: status && yes.exits ? status[1] : status && no.exits ? status[0] : state };
    }
    if (emptyReturn(node) && state === 'rejected' && !recorded) add('DISCARDED_SETTLED_REJECTION', node,
      'A flatMap path narrowed to a rejected native Promise.allSettled result returns an empty array without referencing its reason on that path. Reachability and intentional recovery require review.');
    return { exits: ts.isReturnStatement(node) || ts.isThrowStatement(node), recorded: recorded || mentionsReason(node, result), state };
  }
  function visit(node) {
    if (ts.isCallExpression(node) && ts.isPropertyAccessExpression(node.expression)) {
      const method = node.expression.name.text;
      const callback = node.arguments[0];
      if (callback && (ts.isArrowFunction(callback) || ts.isFunctionExpression(callback))) {
        if (method === 'flatMap' && settledOrigin(node.expression.expression) && callback.parameters.length && ts.isIdentifier(callback.parameters[0].name) && ts.isBlock(callback.body)) {
          callbackPaths(callback.body, symbol(callback.parameters[0].name));
        }
        const receiverType = method === 'catch' ? checker.getTypeAtLocation(node.expression.expression) : undefined;
        const awaitedType = receiverType && checker.getAwaitedType(receiverType);
        const catchDeclaration = method === 'catch' ? checker.getResolvedSignature(node)?.declaration : undefined;
        const nativeCatch = catchDeclaration?.getSourceFile().isDeclarationFile && /(?:^|[\\/])lib\..*\.d\.ts$/.test(catchDeclaration.getSourceFile().fileName);
        if (nativeCatch && awaitedType && awaitedType !== receiverType) {
          const expression = ts.isBlock(callback.body) && callback.body.statements.length === 1 && ts.isReturnStatement(callback.body.statements[0]) ? callback.body.statements[0].expression : ts.isBlock(callback.body) ? undefined : callback.body;
          if (expression && (ts.isLiteralExpression(expression) || [ts.SyntaxKind.NullKeyword,ts.SyntaxKind.TrueKeyword,ts.SyntaxKind.FalseKeyword].includes(expression.kind) || ts.isArrayLiteralExpression(expression) && !expression.elements.length)) add('SILENT_PROMISE_REJECTION', callback, 'A native promise rejection callback returns only a literal or empty array without recording or propagating the failure.');
        }
      }
    }
    ts.forEachChild(node, visit);
  }
  for (const source of sources) visit(source);
  return findings;
}
