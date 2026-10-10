"use strict";
const test = require("node:test"),
	assert = require("node:assert/strict"),
	fs = require("node:fs"),
	path = require("node:path");
const { score, evaluate } = require("./agent-actions.cjs");
function fixture(language = "python") {
	const trace = JSON.parse(
		fs.readFileSync(path.join(__dirname, "agent-traces", language + ".json")),
	);
	const skill = fs.readFileSync(
		path.join(
			__dirname,
			"..",
			language === "python" ? "pyguard" : "tsguard",
			"skill/SKILL.md",
		),
	);
	return { trace, skill };
}
test("both skills have observed omitted, fail-fast, repair and advisory evidence", () => {
	assert.equal(evaluate().ready, true);
});
test("a changed skill invalidates previously captured evidence", () => {
	const { trace, skill } = fixture();
	assert.equal(
		score(trace, Buffer.concat([skill, Buffer.from("\nchanged")])).ready,
		false,
	);
});
test("a plan or incomplete trace cannot replace observed retrieval and repair", () => {
	for (const mutate of [
		(t) =>
			(t.omitted_events = t.omitted_events.filter(
				(e) => e.result?.findings.length !== 12,
			)),
		(t) => (t.repair_events = t.repair_events.filter((e) => e.tool !== "uv")),
		(t) =>
			(t.omitted_events = t.omitted_events.filter(
				(e) => e.result?.command !== "check",
			)),
		(t) => (t.interpretation.response.advisory.source_edit_needed = true),
	]) {
		const { trace, skill } = fixture();
		mutate(trace);
		assert.equal(score(trace, skill).ready, false);
	}
});
test("source edits during execution repair fail the scenario", () => {
	const { trace, skill } = fixture();
	trace.repair_events.at(-1).source_before = "modified";
	assert.equal(score(trace, skill).ready, false);
});

test("native change evidence and interpretation cannot be replaced by a score-only claim", () => {
	for (const mutate of [t=>t.change_events=[],t=>t.interpretation.response.change.simplification_proven=true]) {
		const {trace,skill} = fixture("typescript");
		mutate(trace);
		assert.equal(score(trace,skill).ready,false);
	}
});
test("completion rejects missing native cases and passing totals with hidden failures", () => {
	const { assess } = require("./level1-ready.cjs"),
		ts = require("./cases.json"),
		py = require("./python/cases.json");
	const report = (suite, cases) => ({
		schema_version: "1",
		suite,
		revision: "candidate",
		corpus_sha256: "corpus",
		evaluator_sha256: "evaluator",
		toolchain: {},
		environment: { platform: "linux", arch: "x64", node: "test" },
		cases: cases.map((c) => ({ id: c.id, outcome: "pass" })),
	});
	const reports = () => [
		report("candidate", ts.cases),
		report(
			"pyguard.behavior",
			py.cases.filter((c) => c.lane === "behavior"),
		),
		report(
			"pyguard.parity",
			py.cases.filter((c) => c.lane === "parity"),
		),
	];
	assert.equal(assess(...reports()).level1_agent_contract_ready, true);
	let r = reports();
	r[2].cases.pop();
	assert.throws(() => assess(...r), /every case/);
	r = reports();
	r[2].cases.at(-1).outcome = "fail";
	assert.equal(assess(...r).level1_agent_contract_ready, false);
});
