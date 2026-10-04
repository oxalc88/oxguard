"use strict";
const fs = require("node:fs"),
	path = require("node:path"),
	crypto = require("node:crypto");
function score(trace, skill) {
	const checks = [];
	const check = (id, ok) => checks.push({ id, outcome: ok ? "pass" : "fail" });
	check(
		"skill.capture_current",
		trace.skill_sha256 ===
			crypto
				.createHash("sha256")
				.update(skill.toString().replace(/\r\n/g, "\n"))
				.digest("hex"),
	);
	const events = trace.omitted_events || [],
		first = events.findIndex((e) => e.summary?.includes("omitted:"));
	const initial = events[first];
	const full = events
		.slice(first + 1)
		.find((e) => e.result?.findings.length === 12);
	check(
		"agent.omitted_results",
		first >= 0 &&
			initial.summary.includes("findings: 12") &&
			full &&
			full.result.status === "fail" &&
			full.source_before === initial.source_before &&
			full.source_after === initial.source_before &&
			full.argv.includes("src") &&
			initial.argv.includes("src") &&
			full.result.findings.every((f) => f.category === "quality"),
	);
	const clean = events.find(
		(e) =>
			e.source_before !== initial?.source_before &&
			((e.result?.status === "pass" &&
				e.result.assessment === "complete" &&
				e.result.findings.length === 0) ||
				e.summary?.startsWith("PASS\n")),
	);
	const checkRun = events.findLast(
		(e) =>
			e.result?.command === "check" &&
			e.source_before !== initial?.source_before,
	);
	check(
		"agent.fail_fast",
		!!clean &&
			!!checkRun &&
			checkRun.argv.includes("src") &&
			checkRun.result.assessment === "incomplete" &&
			checkRun.result.gates.some((g) => g.status === "not_run") &&
			checkRun.result.status === "error" &&
			checkRun.exit_code === checkRun.result.exit_code,
	);
	const repair = trace.repair_events || [],
		failed = repair.find((e) =>
			e.result?.findings.some(
				(f) => f.category === "tool_missing" && f.status === "execution_error",
			),
		);
	const install = repair.find(
		(e) =>
			["npm", "uv"].includes(e.tool) &&
			e.argv.includes("install") &&
			e.argv.includes("--offline") &&
			e.exit_code === 0,
	);
	const repaired = repair.find(
		(e) =>
			e.result?.status === "pass" &&
			e.result.assessment === "complete" &&
			e.result.findings.length === 0,
	);
	check(
		"agent.execution_failure",
		!!failed &&
			!!install &&
			!!repaired &&
			repair.indexOf(failed) < repair.indexOf(install) &&
			repair.indexOf(install) < repair.indexOf(repaired) &&
			repair.every(
				(e) =>
					e.source_before === failed.source_before &&
					e.source_after === failed.source_before,
			),
	);
	const native = trace.interpretation?.advisory,
		response = trace.interpretation?.response;
	check(
		"agent.advisory_criticality",
		native?.status === "advisory" &&
			native.exit_code === 0 &&
			response?.advisory.semantic_status === native.status &&
			response.advisory.exit_code === 0 &&
			response.advisory.source_edit_needed === false &&
			response.advisory.limitations.length > 0 &&
			response.advisory.explanation.length > 0,
	);
	check(
		"agent.execution_interpretation",
		trace.interpretation?.execution.status === "error" &&
			response?.execution.semantic_status === "error" &&
			response.execution.source_edit_needed === false &&
			response.execution.explanation.includes("tool_missing"),
	);
	return {
		language: trace.language,
		ready: checks.every((c) => c.outcome === "pass"),
		checks,
	};
}
function evaluate(root = path.join(__dirname, "..")) {
	const languages = [
		["typescript", "tsguard"],
		["python", "pyguard"],
	];
	const results = languages.map(([name, guard]) =>
		score(
			JSON.parse(
				fs.readFileSync(path.join(root, "evals/agent-traces", name + ".json")),
			),
			fs.readFileSync(path.join(root, guard, "skill/SKILL.md")),
		),
	);
	return {
		schema_version: "1",
		execution: "replay of recorded real agent actions; no live LLM calls in CI",
		ready: results.every((r) => r.ready),
		results,
	};
}
if (require.main === module) {
	const report = evaluate();
	console.log(JSON.stringify(report, null, 2));
	process.exitCode = report.ready ? 0 : 1;
}
module.exports = { score, evaluate };
