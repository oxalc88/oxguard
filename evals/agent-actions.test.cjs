"use strict";
// Deterministic scorer tests. Whether the recorded traces are current and
// passing is reported by the non-blocking agent-evals job (node evals/agent-actions.cjs).
const test = require("node:test"),
	assert = require("node:assert/strict"),
	fs = require("node:fs"),
	os = require("node:os"),
	path = require("node:path");
const {
	score,
	skillInventory,
	skillFileHashes,
	REQUIRED_SESSIONS,
	compareSkills,
} = require("./agent-actions.cjs");
const guard = (language) => (language === "python" ? "pyguard" : "tsguard");
function fixture(language = "python") {
	const trace = JSON.parse(
		fs.readFileSync(path.join(__dirname, "agent-traces", language + ".json")),
	);
	return { trace, skill: path.join(__dirname, "..", guard(language), "skill") };
}
// A synthetic skill and a trace whose manifests match it, so freshness tests do
// not depend on the recorded traces being current.
function synthetic(language = "python") {
	const { trace, skill } = fixture(language);
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "oxguard-skill-"));
	fs.cpSync(skill, root, { recursive: true });
	const reads = {
		quality: ["SKILL.md", "workflows/fix.md"],
		repair: ["SKILL.md", "workflows/fix.md"],
		review: REQUIRED_SESSIONS.review.map((group) => group[0]),
		focused: ["SKILL.md", "workflows/check.md"],
	};
	trace.skill_inventory = skillInventory(root);
	trace.sessions = Object.fromEntries(
		Object.entries(reads).map(([session, files]) => [
			session,
			{ read_source: "synthetic", skill_files: skillFileHashes(root, files) },
		]),
	);
	return { trace, root, done: () => fs.rmSync(root, { recursive: true, force: true }) };
}
const staleSessions = (result) => result.stale.map((s) => s.session).sort();
test("a synthetic current trace is ready and current", () => {
	for (const language of ["python", "typescript"]) {
		const { trace, root, done } = synthetic(language);
		try {
			const result = score(trace, root);
			assert.equal(result.ready, true, JSON.stringify(result.checks));
			assert.equal(result.current, true);
		} finally {
			done();
		}
	}
});
test("a check.md change makes only the sessions that read it stale", () => {
	const { trace, root, done } = synthetic();
	try {
		fs.appendFileSync(path.join(root, "workflows/check.md"), "\nchanged");
		const result = score(trace, root);
		assert.deepEqual(staleSessions(result), ["focused", "review"]);
		assert.equal(result.ready, true);
		assert.equal(result.current, false);
	} finally {
		done();
	}
});
test("a SKILL.md or skill inventory change makes every session stale", () => {
	for (const change of [
		(root) => fs.appendFileSync(path.join(root, "SKILL.md"), "\nchanged"),
		(root) => fs.writeFileSync(path.join(root, "workflows/new.md"), "new\n"),
	]) {
		const { trace, root, done } = synthetic();
		try {
			change(root);
			assert.deepEqual(staleSessions(score(trace, root)), [
				"focused",
				"quality",
				"repair",
				"review",
			]);
		} finally {
			done();
		}
	}
});
test("a current trace with wrong behavior fails", () => {
	const { trace, root, done } = synthetic();
	try {
		trace.interpretation.response.advisory.source_edit_needed = true;
		const result = score(trace, root);
		assert.equal(result.current, true);
		assert.equal(result.ready, false);
	} finally {
		done();
	}
});
test("v1 traces and manifests missing declared files are rejected", () => {
	const { trace, root, done } = synthetic();
	try {
		const v1 = { ...trace, schema_version: "1", skill_sha256: "x" };
		delete v1.sessions;
		assert.equal(score(v1, root).ready, false);
		delete trace.sessions.review.skill_files["workflows/security.md"];
		assert.equal(score(trace, root).ready, false);
	} finally {
		done();
	}
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
		(t) => (t.repair_events.at(-1).source_before = "modified"),
	]) {
		const { trace, skill } = fixture();
		mutate(trace);
		assert.equal(score(trace, skill).ready, false);
	}
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
test("router evidence must include the selected task guides", () => {
	for (const task of ["quality", "repair", "security", "review"]) {
		const { trace, skill } = fixture();
		trace.interpretation.response.routes[task] = "workflows/wrong.md";
		assert.equal(score(trace, skill).ready, false);
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
test("skill word limits: total <= 700, entry and check below the old single-file entry", () => {
	for (const result of compareSkills().results) {
		assert.ok(result.text_size.current_total <= 700, JSON.stringify(result.text_size));
		assert.ok(result.text_size.current_entry <= 150);
		assert.ok(result.text_size.current_entry < result.text_size.baseline_entry);
		assert.ok(result.text_size.current_check < result.text_size.baseline_entry);
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
		report("pyguard.behavior", py.cases.filter((c) => c.lane === "behavior")),
		report("pyguard.parity", py.cases.filter((c) => c.lane === "parity")),
	];
	const ok = assess(...reports());
	assert.equal(ok.deterministic_ready, true);
	assert.equal(ok.blocking_ready, true);
	assert.equal(
		assess(...reports(), { strict: true }).blocking_ready,
		ok.agent_traces_ready && ok.agent_traces_current,
	);
	let r = reports();
	r[2].cases.pop();
	assert.throws(() => assess(...r), /every case/);
	r = reports();
	r[2].cases.at(-1).outcome = "fail";
	assert.equal(assess(...r).blocking_ready, false);
	assert.equal(assess(...r).level1_agent_contract_ready, false);
});
