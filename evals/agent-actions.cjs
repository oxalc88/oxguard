"use strict";
const fs = require("node:fs"),
	path = require("node:path"),
	crypto = require("node:crypto");
function readSkillBundle(directory) {
	const normalize = (content) =>
		Buffer.from(content.toString().replace(/\r\n/g, "\n").trimEnd() + "\n");
	const entry = normalize(fs.readFileSync(path.join(directory, "SKILL.md")));
	const parts = [entry];
	for (const folder of ["references", "workflows"]) {
		const dir = path.join(directory, folder);
		if (!fs.existsSync(dir)) continue;
		for (const file of fs
			.readdirSync(dir)
			.filter((f) => f.endsWith(".md"))
			.sort()) {
			parts.push(Buffer.from("\0" + folder + "/" + file + "\0"));
			parts.push(normalize(fs.readFileSync(path.join(dir, file))));
		}
	}
	return Buffer.concat(parts);
}
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
	if (skill.includes(Buffer.from("\0workflows/"))) {
		check("skill.router_layout", trace.skill_layout === "router");
		const response = trace.interpretation?.response;
		const readNames =
			response?.skill_reads?.map((file) => file.replace(/^skill\//, "")) || [];
		for (const [task, file] of [
			["quality", "check"],
			["repair", "fix"],
			["security", "security"],
			["review", "review"],
		]) {
			const workflow = "workflows/" + file + ".md";
			check(
				"agent.route_" + task,
				(response?.routes?.[task] === workflow ||
					(task === "quality" &&
						response?.routes?.[task] === "workflows/fix.md")) &&
					readNames.includes(workflow),
			);
		}
		check("agent.entry_read", readNames.includes("SKILL.md"));
		const focused = trace.focused_review;
		const reads =
			focused?.response?.skill_reads?.map((file) =>
				file.replace(/^skill\//, ""),
			) || [];
		check(
			"agent.focused_result",
			focused?.input?.status === "advisory" &&
				focused?.response?.semantic_status === "advisory" &&
				focused?.response?.exit_code === 0 &&
				focused?.response?.source_edit_needed === false &&
				reads.filter((file) => /^workflows\//.test(file)).length === 1 &&
				reads.some((file) =>
					["workflows/check.md", "workflows/review.md"].includes(file),
				),
		);
	}
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
	for (const [id, key, decision] of [
		["agent.dependency_review_boundary", "dependency_review", "may_waive"],
		[
			"agent.verification_reuse_boundary",
			"verification_reuse",
			"reuse_allowed",
		],
		["agent.unknown_status_rejected", "unknown_status", "valid_evidence"],
	]) {
		check(
			id,
			response?.[key]?.[decision] === false &&
				response[key].explanation?.length > 0,
		);
	}
	if (trace.language === "typescript") {
		const nativeChange = trace.change_events?.find(
			(e) => e.result?.command === "change",
		)?.result;
		const responseChange = response?.change;
		const nativeMetrics = nativeChange?.measurements || [];
		check(
			"agent.native_change_interpretation",
			nativeChange?.assessment === "complete" &&
				nativeChange.status === "advisory" &&
				nativeChange.exit_code === 0 &&
				[
					"change.baseline.fta.score",
					"change.candidate.fta.score",
					"change.baseline.duplicated_tokens",
					"change.candidate.function.cognitive_min",
				].every((metric) => nativeMetrics.some((m) => m.metric === metric)) &&
				responseChange?.semantic_status === nativeChange.status &&
				responseChange.exit_code === 0 &&
				responseChange.simplification_proven === false &&
				responseChange.explanation.length > 0 &&
				responseChange.limitations.length > 0,
		);
	}
	return {
		language: trace.language,
		ready: checks.every((c) => c.outcome === "pass"),
		checks,
	};
}
function compareSkills(root = path.join(__dirname, "..")) {
	const words = (value) => value.toString().trim().split(/\s+/).length;
	const results = [
		["typescript", "tsguard"],
		["python", "pyguard"],
	].map(([language, guard]) => {
		const baseline = JSON.parse(
			fs.readFileSync(
				path.join(
					root,
					"evals/agent-traces/router-baseline-" + language + ".json",
				),
			),
		);
		const current = JSON.parse(
			fs.readFileSync(
				path.join(root, "evals/agent-traces/" + language + ".json"),
			),
		);
		const directory = path.join(root, guard, "skill");
		const previous = score(baseline, Buffer.from(baseline.skill_snapshot));
		const candidate = score(current, readSkillBundle(directory));
		const selectedFiles = [
			...new Set(
				current.interpretation.response.skill_reads.map((file) =>
					file.replace(/^skill\//, ""),
				),
			),
		];
		return {
			language,
			baseline: previous,
			candidate,
			text_size: {
				unit: "whitespace-separated words; not model tokens",
				baseline_entry: words(baseline.skill_snapshot),
				current_entry: words(fs.readFileSync(path.join(directory, "SKILL.md"))),
				current_total:
					words(fs.readFileSync(path.join(directory, "SKILL.md"))) +
					fs
						.readdirSync(path.join(directory, "workflows"))
						.filter((file) => file.endsWith(".md"))
						.reduce(
							(n, file) =>
								n +
								words(fs.readFileSync(path.join(directory, "workflows", file))),
							0,
						),
				focused_review_words: [
					...new Set(
						current.focused_review.response.skill_reads.map((file) =>
							file.replace(/^skill\//, ""),
						),
					),
				].reduce(
					(n, file) => n + words(fs.readFileSync(path.join(directory, file))),
					0,
				),
				current_check: ["SKILL.md", "workflows/check.md"].reduce(
					(n, file) => n + words(fs.readFileSync(path.join(directory, file))),
					0,
				),
				combined_task_files: selectedFiles,
				combined_task_words: selectedFiles.reduce(
					(n, file) => n + words(fs.readFileSync(path.join(directory, file))),
					0,
				),
				read_inventory:
					"agent-reported; combined requests exercise all task guides",
			},
		};
	});
	return {
		ready: results.every(
			(r) =>
				r.baseline.ready &&
				r.candidate.ready &&
				r.text_size.current_total <= 700 &&
				r.text_size.current_entry <= 150,
		),
		results,
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
			readSkillBundle(path.join(root, guard, "skill")),
		),
	);
	const comparison = compareSkills(root);
	return {
		schema_version: "1",
		execution: "replay of recorded real agent actions; no live LLM calls in CI",
		ready: results.every((r) => r.ready) && comparison.ready,
		results,
		comparison,
	};
}
if (require.main === module) {
	const report = evaluate();
	console.log(JSON.stringify(report, null, 2));
	process.exitCode = report.ready ? 0 : 1;
}
module.exports = { score, evaluate, readSkillBundle, compareSkills };
