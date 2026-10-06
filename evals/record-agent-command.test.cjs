"use strict";
const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const os = require("node:os");
const { fingerprint, record } = require("./record-agent-command.cjs");

function fixture(t) {
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "oxguard-observer-"));
	t.after(() => fs.rmSync(root, { recursive: true, force: true }));
	fs.mkdirSync(path.join(root, "src"));
	fs.writeFileSync(path.join(root, "src/main.ts"), "export const value = 1;\n");
	const events = path.join(root, "events.jsonl");
	return {
		root, events,
		config: { tools: { node: process.execPath }, projects: [{ root, events }] },
		read: () => fs.readFileSync(events, "utf8").trim().split("\n").map(JSON.parse),
	};
}

test("observer records real process results and detects source mutations", (t) => {
	const f = fixture(t);
	const before = fingerprint(f.root);
	const data = { schema_version: "1", status: "fail", exit_code: 7, gates: [], findings: [] };
	const argv = ["-e", `console.log(${JSON.stringify(JSON.stringify(data))}); process.exitCode=7;`];
	const result = record(f.config, "node", argv, f.root);
	assert.equal(result.status, 7);
	const first = f.read()[0];
	assert.deepEqual(first.argv, argv);
	assert.equal(first.exit_code, 7);
	assert.deepEqual(first.result, data);
	assert.equal(first.source_before, before);
	assert.equal(first.source_after, before);
	record(f.config, "node", ["-e", "require('fs').writeFileSync('src/main.ts', 'changed');"], f.root);
	const second = f.read()[1];
	assert.equal(second.source_before, before);
	assert.notEqual(second.source_after, before);
	assert.equal(second.source_after, fingerprint(f.root));
});

test("observer preserves bounded output without inventing normalized records", (t) => {
	const f = fixture(t);
	const summary = "FAIL\ncommand: types\nfindings: 12\nomitted: 2 (use --output json)\n";
	record(f.config, "node", ["-e", `process.stdout.write(${JSON.stringify(summary)}); process.stderr.write('diagnostic');`], f.root);
	const e = f.read()[0];
	assert.equal(e.summary, summary);
	assert.equal(e.stderr, "diagnostic");
	assert.equal(e.result, undefined);
});

test("observer rejects outside projects, unknown tools and source symlinks", (t) => {
	const f = fixture(t);
	assert.throws(() => record(f.config, "node", [], path.dirname(f.root)), /inside a capture fixture/);
	assert.throws(() => record(f.config, "unknown", [], f.root), /Unknown observed tool/);
	try {
		fs.symlinkSync(path.join(f.root, "src/main.ts"), path.join(f.root, "src/link.ts"));
	} catch (error) {
		if (error.code === "EPERM" || error.code === "EACCES") {
			t.skip("Environment cannot create source symlinks");
			return;
		}
		throw error;
	}
	assert.throws(() => fingerprint(f.root), /Source symlink/);
	assert.equal(fs.existsSync(f.events), false);
});
