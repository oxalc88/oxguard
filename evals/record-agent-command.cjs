"use strict";

// Fixture-only command observer. A fresh agent chooses commands through wrappers;
// this file records actual execution, never generates agent actions or answers.
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { spawnSync } = require("node:child_process");

function fingerprint(root) {
	const hash = crypto.createHash("sha256");
	function visit(directory) {
		for (const name of fs.readdirSync(directory).sort()) {
			const file = path.join(directory, name);
			const stat = fs.lstatSync(file);
			if (stat.isSymbolicLink()) throw Error(`Source symlink: ${file}`);
			if (stat.isDirectory()) visit(file);
			else if (stat.isFile()) {
				hash.update(path.relative(root, file).replaceAll("\\", "/"));
				hash.update("\0");
				hash.update(fs.readFileSync(file));
				hash.update("\0");
			}
		}
	}
	visit(path.join(root, "src"));
	return hash.digest("hex");
}

function record(config, tool, argv, cwd = process.cwd()) {
	const project = config.projects.find((p) =>
		cwd === p.root || cwd.startsWith(p.root + path.sep),
	);
	if (!project) throw Error("Command must run inside a capture fixture");
	if (!config.tools[tool]) throw Error(`Unknown observed tool: ${tool}`);
	const before = fingerprint(project.root);
	const result = spawnSync(config.tools[tool], argv, {
		cwd,
		env: { ...process.env, ...project.env },
		encoding: "utf8",
		timeout: config.timeout_ms || 60000,
		maxBuffer: 32 * 1024 * 1024,
	});
	const event = {
		tool, argv, exit_code: result.status,
		source_before: before, source_after: fingerprint(project.root),
	};
	try {
		const parsed = JSON.parse(result.stdout);
		if (parsed.schema_version === "1" && typeof parsed.status === "string" &&
			Array.isArray(parsed.findings) && Array.isArray(parsed.gates) &&
			typeof parsed.exit_code === "number")
			event.result = parsed;
		else event.tool_output = result.stdout;
	} catch {
		if (/^(PASS|FAIL|ERROR|ADVISORY|SKIPPED)\ncommand:/.test(result.stdout))
			event.summary = result.stdout;
		else event.tool_output = result.stdout;
	}
	if (result.stderr) event.stderr = result.stderr;
	if (result.error || result.signal)
		event.execution_error = result.error?.message || result.signal;
	fs.appendFileSync(project.events, JSON.stringify(event) + "\n");
	return result;
}

if (require.main === module) {
	const [configPath, tool, ...argv] = process.argv.slice(2);
	if (!configPath || !tool)
		throw Error("Usage: node record-agent-command.cjs <config.json> <tool> [args...]");
	const result = record(JSON.parse(fs.readFileSync(configPath)), tool, argv);
	if (result.stdout) process.stdout.write(result.stdout);
	if (result.stderr) process.stderr.write(result.stderr);
	if (result.error) process.stderr.write(result.error.message + "\n");
	process.exitCode = result.status ?? 1;
}

module.exports = { fingerprint, record };
