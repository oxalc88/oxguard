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

// Optional Python maintainability evidence; legacy fixture() has no such declarations.
function pythonMaintainability(trace) {
	const { PYTHON_SCENARIOS, SCENARIO_FILES } = require("./agent-actions.cjs");
	for (const [name, scenario] of Object.entries(PYTHON_SCENARIOS)) {
		trace.sessions[name] = {
			scenario,
			skill_files: skillFileHashes(fixture().skill, SCENARIO_FILES.flat()),
		};
	}
	const baseline = "a".repeat(40);
	const interpretation = (semantic_status, assessment, exit_code) => ({
		semantic_status, assessment, exit_code, simplification_proven: false,
		explanation: "Review end-to-end behavior; smaller files alone do not establish simplification.",
		limitations: ["Scoped static evidence; duplication is deferred."],
	});
	trace.structure_review = {
		events: [{ tool: "pyguard", argv: ["structure", "--dirs", "src", "--output", "json"], exit_code: 0,
			result: { command: "structure", status: "pass", assessment: "complete", exit_code: 0,
				measurements: ["module.count", "module.edges", "function.fan_in", "function.fan_out", "function.call_depth"].map((metric) => ({ metric, value: 1 })) } }],
		response: { ...interpretation("pass", "complete", 0), blocks_shipping: false, maintainability_proven: false },
	};
	trace.change_events = [
		{ tool: "pyguard", argv: ["change", "--dirs", "src", "--output", "json"], exit_code: 1,
			result: { command: "change", status: "advisory", assessment: "incomplete", exit_code: 1,
				findings: [{ rule: "pyguard.change.BASELINE_UNAVAILABLE" }] } },
		{ tool: "pyguard", argv: ["change", "--dirs", "src", "--baseline", baseline, "--output", "json"], exit_code: 0,
			result: { command: "change", status: "advisory", assessment: "complete", exit_code: 0,
				artifacts: [{ kind: "maintainability_change", path: ".pyguard-cache/maintainability-change.json" }],
				measurements: ["baseline", "candidate"].flatMap((side) =>
					["total_branches", "total_cyclomatic", "function.cyclomatic", "function.cognitive"].map((m) => ({ metric: `change.${side}.${m}`, value: 4 }))) } },
	];
	const profile = () => ({ cognitive_complete: true, files: [], functions: [{ cyclomatic: 2, cognitive: 1 }] });
	trace.change_review = {
		baseline, artifact_path: ".pyguard-cache/maintainability-change.json", artifact_read: true,
		artifact: { schema_version: "1", baseline_sha: baseline, baseline_structure: {}, candidate_structure: {},
			file_matches: [], function_matches: [], native_metrics: { baseline: profile(), candidate: profile(),
				policy: { candidate_config_applies_to_both: true } } },
		response: { baseline, change: interpretation("advisory", "complete", 0), incomplete: interpretation("advisory", "incomplete", 1),
			duplication: { capability: "change.native_duplication", outcome: "deferred" } },
	};
	return trace;
}
test("new Python scenarios pass supported evidence and report duplication as deferred", () => {
	const { trace, skill } = fixture();
	const result = score(pythonMaintainability(trace), skill);
	assert.equal(result.ready, true, JSON.stringify(result.checks));
	assert.equal(result.current, true);
	assert.deepEqual(result.checks.find((c) => c.id === "change.native_duplication"),
		{ id: "change.native_duplication", outcome: "deferred" });
	// Missing duplication metrics are intentional; neither a success nor a failure.
	trace.change_events[1].argv.splice(3, 2, `--baseline=${trace.change_review.baseline}`);
	assert.equal(score(trace, skill).ready, true);
});
test("existing Python traces do not opt into new structure/change requirements", () => {
	const { trace, skill } = fixture();
	trace.change_events = [];
	trace.change_review = { response: { change: { simplification_proven: true } } };
	const result = score(trace, skill);
	assert.equal(result.ready, true);
	assert.equal(result.checks.some((c) => c.id.startsWith("agent.python_")), false);
});
test("Python structure rejects missing graph evidence and unsupported maintainability claims", () => {
	for (const mutate of [
		(t) => delete t.structure_review,
		(t) => t.structure_review.events[0].result.measurements.pop(),
		(t) => t.structure_review.response.maintainability_proven = true,
		(t) => t.structure_review.response.blocks_shipping = true,
		(t) => t.structure_review.response.assessment = "incomplete",
		(t) => t.structure_review.events[0].result.status = "unexpected",
		(t) => delete t.sessions.structure.skill_files["workflows/review.md"],
	]) {
		const { trace, skill } = fixture();
		pythonMaintainability(trace); mutate(trace);
		assert.equal(score(trace, skill).ready, false, mutate.toString());
	}
});
test("Python change rejects implicit baselines, missing artifact evidence and incomplete-success claims", () => {
	for (const mutate of [
		(t) => t.change_events[1].argv.splice(3, 2),
		(t) => t.change_events[1].argv.splice(4, 1),
		(t) => t.change_review.response.baseline = "HEAD",
		(t) => t.change_review.artifact.baseline_sha = "b".repeat(40),
		(t) => delete t.change_review.artifact,
		(t) => t.change_review.artifact_read = false,
		(t) => t.change_review.artifact_path = "other.json",
		(t) => t.change_events[1].result.artifacts = [],
		(t) => t.change_review.artifact.native_metrics.candidate.functions[0].cognitive = null,
		(t) => t.change_review.artifact.native_metrics.baseline.cognitive_complete = false,
		(t) => t.change_events[1].result.measurements.pop(),
		(t) => t.change_events[1].result.assessment = "incomplete",
		(t) => t.change_review.response.change.simplification_proven = true,
		(t) => t.change_review.response.incomplete.assessment = "complete",
		(t) => t.change_review.response.incomplete.simplification_proven = true,
		(t) => t.change_events.shift(),
		(t) => t.change_review.response.duplication.outcome = "pass",
		(t) => t.change_review.response.duplication.outcome = "fail",
	]) {
		const { trace, skill } = fixture();
		pythonMaintainability(trace); mutate(trace);
		assert.equal(score(trace, skill).ready, false, mutate.toString());
	}
});

test("capture assembly retains optional Python scenarios, artifacts and actual recording provenance", () => {
	const { fromCaptures } = require("./agent-capture/assemble.cjs");
	const { trace, skill } = fixture();
	pythonMaintainability(trace);
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "oxguard-capture-"));
	const dirs = {};
	try {
		for (const name of ["quality", "repair", "review", "focused", "structure", "change"]) {
			const dir = dirs[name] = path.join(root, name);
			fs.mkdirSync(path.join(dir, "project/.agents/skills"), { recursive: true });
			fs.cpSync(skill, path.join(dir, "project/.agents/skills/pyguard"), { recursive: true });
			const artifactPath = ".pyguard-cache/maintainability-change.json";
			const command = name === "change" ? `cat ${artifactPath}` : "cat .agents/skills/pyguard/SKILL.md";
			fs.writeFileSync(path.join(dir, "agent-stream.jsonl"), JSON.stringify({ type: "item.completed",
				item: { type: "command_execution", command, exit_code: 0 } }) + "\n");
			fs.writeFileSync(path.join(dir, "agent-exit.txt"), "exit 0 effort medium model recorded-model\n");
			fs.writeFileSync(path.join(dir, "agent-version.txt"), `codex-cli 9.8.${name.length}\n`);
			const response = {
				quality: {}, repair: {}, review: trace.interpretation.response,
				focused: trace.focused_review.response,
				structure: trace.structure_review.response, change: trace.change_review.response,
			}[name];
			const events = {
				quality: trace.omitted_events, repair: trace.repair_events,
				review: [{ result: trace.interpretation.advisory }], focused: [{ result: trace.focused_review.input }],
				structure: trace.structure_review.events, change: trace.change_events,
			}[name];
			fs.writeFileSync(path.join(dir, "agent-last-message.txt"), JSON.stringify(response));
			fs.writeFileSync(path.join(dir, "events.jsonl"), events.map((e) => JSON.stringify(e)).join("\n") + "\n");
		}
		fs.writeFileSync(path.join(dirs.change, "baseline.txt"), trace.change_review.baseline + "\n");
		fs.mkdirSync(path.join(dirs.change, "project/.pyguard-cache"));
		fs.writeFileSync(path.join(dirs.change, "project", trace.change_review.artifact_path), JSON.stringify(trace.change_review.artifact));
		const result = fromCaptures("python", dirs);
		assert.equal(result.sessions.structure.scenario, "python.structure.v1");
		assert.equal(result.sessions.change.scenario, "python.change.baseline.v1");
		assert.equal(result.sessions.change.agent, "codex-cli 9.8.6 exec");
		assert.equal(result.sessions.change.provider, "openai");
		assert.equal(result.sessions.change.model, "recorded-model");
		assert.equal(result.sessions.change.effort, "medium");
		assert.match(result.provenance, /codex-cli 9\.8\.6 exec/);
		assert.doesNotMatch(result.provenance, /0\.164\.0/);
		assert.equal(result.change_review.artifact_read, true);
		assert.deepEqual(result.change_review.artifact, trace.change_review.artifact);
		assert.deepEqual(result.change_events, trace.change_events);
		assert.deepEqual(result.structure_review.events, trace.structure_review.events);
		// No optional recordings: assembling old scenarios does not invent declarations.
		delete dirs.change; delete dirs.structure;
		const legacy = fromCaptures("python", dirs);
		assert.equal(legacy.sessions.change, undefined);
		assert.equal(legacy.sessions.structure, undefined);
	} finally {
		fs.rmSync(root, { recursive: true, force: true });
	}
});
test("assembly fails clearly when recorded CLI provenance is missing or invalid", () => {
	const { agentOf } = require("./agent-capture/assemble.cjs");
	const dir = fs.mkdtempSync(path.join(os.tmpdir(), "oxguard-version-"));
	try {
		fs.writeFileSync(path.join(dir, "agent-exit.txt"), "exit 0 effort high model actual-model\n");
		assert.throws(() => agentOf(dir), /missing recorded CLI provenance.*re-record/);
		for (const invalid of ["", "codex-cli", "made-up 1.2.3", "codex-cli 1.2.3\nextra"]) {
			fs.writeFileSync(path.join(dir, "agent-version.txt"), invalid);
			assert.throws(() => agentOf(dir), /invalid recorded CLI version/);
		}
		fs.writeFileSync(path.join(dir, "agent-version.txt"), "codex-cli 1.2.3\n");
		assert.deepEqual(agentOf(dir), { agent: "codex-cli 1.2.3 exec", provider: "openai", model: "actual-model", effort: "high" });
		fs.writeFileSync(path.join(dir, "agent-exit.txt"), "exit 1 effort high model actual-model\n");
		assert.throws(() => agentOf(dir), /unsuccessful recording/);
	} finally {
		fs.rmSync(dir, { recursive: true, force: true });
	}
});
test("artifact read evidence requires an observed successful command", () => {
	const { observedArtifactRead } = require("./agent-capture/assemble.cjs");
	const dir = fs.mkdtempSync(path.join(os.tmpdir(), "oxguard-read-"));
	const file = path.join(dir, "stream.jsonl"), artifact = ".pyguard-cache/maintainability-change.json";
	try {
		for (const [type, exit_code, command, expected] of [
			["item.completed", 0, `cat ${artifact}`, true],
			["item.started", 0, `cat ${artifact}`, false],
			["item.completed", 1, `cat ${artifact}`, false],
			["item.completed", 0, `echo ${artifact}`, false],
			["item.completed", 0, "cat other.json", false],
		]) {
			fs.writeFileSync(file, JSON.stringify({ type, item: { type: "command_execution", exit_code, command } }) + "\n");
			assert.equal(observedArtifactRead(file, artifact), expected);
		}
	} finally {
		fs.rmSync(dir, { recursive: true, force: true });
	}
});

test("capture setup creates a real Python baseline and branch-preserving split", {
	skip: process.platform === "win32" ? "Capture scripts require a POSIX environment (Linux or macOS)" : false,
}, () => {
	const { spawnSync } = require("node:child_process");
	const root = fs.mkdtempSync(path.join(os.tmpdir(), "oxguard-setup-"));
	const run = (command, args, options = {}) => {
		const result = spawnSync(command, args, { encoding: "utf8", ...options });
		assert.ifError(result.error);
		assert.equal(result.status, 0, result.stderr || result.stdout);
		return result.stdout.trim();
	};
	try {
		// Tool installation is irrelevant to fixture generation; keep it offline.
		fs.mkdirSync(path.join(root, "ts-toolchain/node_modules"), { recursive: true });
		fs.writeFileSync(path.join(root, "ts-toolchain/package-lock.json"), "{}\n");
		fs.writeFileSync(path.join(root, "fixture.tgz"), "");
		fs.mkdirSync(path.join(root, "bin"));
		for (const name of ["npm", "uv"]) {
			fs.writeFileSync(path.join(root, "bin", name), "#!/bin/sh\nexit 0\n", { mode: 0o755 });
		}
		run("bash", [path.join(__dirname, "agent-capture/setup.sh")], {
			env: { ...process.env, OXGUARD_CAPTURE_DIR: root, PATH: path.join(root, "bin") + path.delimiter + process.env.PATH },
		});
		const dir = path.join(root, "replay/python/change"), project = path.join(dir, "project");
		const git = (...args) => run("git", ["-C", project, ...args]);
		const branches = (s) => (s.match(/if x >/g) || []).length;
		const baseline = fs.readFileSync(path.join(dir, "baseline.txt"), "utf8").trim();
		assert.equal(git("rev-parse", "HEAD~1"), baseline);
		assert.equal(git("log", "-1", "--format=%s"), "candidate");
		const before = git("show", `${baseline}:src/main.py`);
		assert.equal(branches(before), 4);
		assert.equal(git("status", "--porcelain"), "");
		const a = fs.readFileSync(path.join(project, "src/a.py"), "utf8");
		const b = fs.readFileSync(path.join(project, "src/b.py"), "utf8");
		assert.equal(branches(a + b), 4);
		assert.match(fs.readFileSync(path.join(project, "src/main.py"), "utf8"), /return a\(x\) \+ b\(x\)/);
		const tsProject = path.join(root, "replay/typescript/change/project");
		assert.equal(fs.readFileSync(path.join(root, "replay/typescript/change/baseline.txt"), "utf8").trim(), run("git", ["-C", tsProject, "rev-parse", "HEAD~1"]));
		fs.mkdirSync(path.join(project, "src/__pycache__"), { recursive: true });
		fs.writeFileSync(path.join(project, "src/__pycache__/x.pyc"), "");
		assert.equal(git("status", "--porcelain"), "");
		fs.mkdirSync(path.join(project, ".pyguard-cache"));
		fs.writeFileSync(path.join(project, ".pyguard-cache/maintainability-change.json"), "{}");
		assert.equal(git("status", "--porcelain"), "");
	} finally {
		fs.rmSync(root, { recursive: true, force: true });
	}
});
