"use strict";
const test = require("node:test"),
	assert = require("node:assert/strict"),
	fs = require("node:fs"),
	path = require("node:path");
const {
	score,
	evaluate,
	readSkillBundle,
	compareSkills,
} = require("./agent-actions.cjs");
function fixture(language = "python") {
	const trace = JSON.parse(
		fs.readFileSync(path.join(__dirname, "agent-traces", language + ".json")),
	);
	const skill = readSkillBundle(
		path.join(
			__dirname,
			"..",
			language === "python" ? "pyguard" : "tsguard",
			"skill",
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

test("skills cannot waive unreviewed advisories, reuse changed locks or accept unknown statuses", () => {
	for (const language of ["python", "typescript"]) {
		for (const [key, decision] of [
			["dependency_review", "may_waive"],
			["verification_reuse", "reuse_allowed"],
			["unknown_status", "valid_evidence"],
		]) {
			const { trace, skill } = fixture(language);
			trace.interpretation.response[key][decision] = true;
			assert.equal(score(trace, skill).ready, false);
		}
	}
});

test("native change evidence and interpretation cannot be replaced by a score-only claim", () => {
	for (const mutate of [
		(t) => (t.change_events = []),
		(t) => (t.interpretation.response.change.simplification_proven = true),
	]) {
		const { trace, skill } = fixture("typescript");
		mutate(trace);
		assert.equal(score(trace, skill).ready, false);
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

test("router evidence must include the selected task guides", () => {
	for (const task of ["quality", "repair", "security", "review"]) {
		const { trace, skill } = fixture();
		trace.interpretation.response.routes[task] = "workflows/wrong.md";
		assert.equal(score(trace, skill).ready, false);
	}
});

test("a changed task guide invalidates the whole bundle capture", () => {
	const os = require("node:os");
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "oxguard-skill-hash-"));
	try {
		fs.cpSync(path.join(__dirname, "../pyguard/skill"), root, {
			recursive: true,
		});
		const { trace } = fixture();
		for (const relative of ["workflows/check.md", "workflows/fix.md"]) {
			const target = path.join(root, relative),
				original = fs.readFileSync(target);
			fs.appendFileSync(target, "\nchanged");
			assert.equal(score(trace, readSkillBundle(root)).ready, false);
			fs.writeFileSync(target, original);
		}
	} finally {
		fs.rmSync(root, { recursive: true, force: true });
	}
});

test("old and split skills preserve action correctness; focused tasks load less text", () => {
	const comparison = compareSkills();
	assert.equal(comparison.ready, true);
	for (const result of comparison.results) {
		assert.ok(result.text_size.current_entry < result.text_size.baseline_entry);
		assert.ok(result.text_size.current_total <= 700);
		assert.ok(
			result.text_size.focused_review_words < result.text_size.baseline_entry,
		);
		assert.ok(result.text_size.current_check < result.text_size.baseline_entry);
	}
});

test("focused review cannot claim success after loading unrelated workflows", () => {
	const { trace, skill } = fixture();
	trace.focused_review.response.skill_reads.push("workflows/security.md");
	assert.equal(score(trace, skill).ready, false);
});

test("router metadata cannot disable routing checks", () => {
	const { trace, skill } = fixture();
	delete trace.skill_layout;
	assert.equal(score(trace, skill).ready, false);
});
