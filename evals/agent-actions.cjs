"use strict";
const fs = require("node:fs"),
	path = require("node:path"),
	crypto = require("node:crypto");
const normalize = (content) =>
	Buffer.from(content.toString().replace(/\r\n/g, "\n").trimEnd() + "\n");
const sha256 = (content) =>
	crypto.createHash("sha256").update(content).digest("hex");
// Skill files an agent can be routed to: the entry plus every reference and workflow guide.
function skillInventory(directory) {
	const files = ["SKILL.md"];
	for (const folder of ["references", "workflows"]) {
		const dir = path.join(directory, folder);
		if (!fs.existsSync(dir)) continue;
		for (const file of fs.readdirSync(dir).filter((f) => f.endsWith(".md")))
			files.push(folder + "/" + file);
	}
	return files.sort();
}
function skillFileHashes(directory, files = skillInventory(directory)) {
	return Object.fromEntries(
		files.map((file) => {
			const target = path.join(directory, file);
			return [
				file,
				fs.existsSync(target) ? sha256(normalize(fs.readFileSync(target))) : null,
			];
		}),
	);
}
// Legacy v1 bundle: only historical snapshot baselines are still scored this way.
function readSkillBundle(directory) {
	const parts = [normalize(fs.readFileSync(path.join(directory, "SKILL.md")))];
	for (const file of skillInventory(directory).filter((f) => f !== "SKILL.md")) {
		parts.push(Buffer.from("\0" + file + "\0"));
		parts.push(normalize(fs.readFileSync(path.join(directory, file))));
	}
	return Buffer.concat(parts);
}
// Minimum skill files each recorded session must declare, whatever else it read.
const REQUIRED_SESSIONS = {
	quality: [["SKILL.md"], ["workflows/check.md", "workflows/fix.md"]],
	repair: [["SKILL.md"], ["workflows/fix.md"]],
	review: [
		["SKILL.md"],
		["workflows/check.md"],
		["workflows/fix.md"],
		["workflows/security.md"],
		["workflows/review.md"],
	],
	focused: [["SKILL.md"], ["workflows/check.md", "workflows/review.md"]],
};
// A session is stale when the inventory or any skill file it recorded has changed.
function freshness(trace, directory) {
	const inventory = skillInventory(directory);
	const inventoryChanged =
		JSON.stringify(trace.skill_inventory) !== JSON.stringify(inventory);
	const stale = [];
	for (const [session, record] of Object.entries(trace.sessions || {})) {
		const recorded = record.skill_files || {};
		const current = skillFileHashes(directory, Object.keys(recorded));
		const files = Object.keys(recorded).filter(
			(file) => recorded[file] !== current[file],
		);
		if (inventoryChanged || files.length)
			stale.push({
				session,
				reason: inventoryChanged ? "skill inventory changed" : "skill files changed",
				files,
			});
	}
	return stale;
}
function score(trace, skill) {
	const checks = [];
	const check = (id, ok) => checks.push({ id, outcome: ok ? "pass" : "fail" });
	let stale = [],
		router;
	if (Buffer.isBuffer(skill)) {
		check(
			"skill.capture_current",
			String(trace.schema_version) === "1" &&
				typeof trace.skill_snapshot === "string" &&
				trace.skill_sha256 === sha256(skill.toString().replace(/\r\n/g, "\n")),
		);
		router = skill.includes(Buffer.from("\0workflows/"));
	} else {
		check("trace.schema_v2", trace.schema_version === "2");
		check(
			"skill.manifest_declared",
			Object.entries(REQUIRED_SESSIONS).every(([session, groups]) => {
				const files = Object.keys(trace.sessions?.[session]?.skill_files || {});
				return groups.every((group) => group.some((f) => files.includes(f)));
			}),
		);
		stale = trace.schema_version === "2" ? freshness(trace, skill) : [];
		router = skillInventory(skill).some((file) => file.startsWith("workflows/"));
	}
	if (router) {
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
		current: stale.length === 0,
		stale,
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
		const candidate = score(current, directory);
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
	const results = [
		["typescript", "tsguard"],
		["python", "pyguard"],
	].map(([name, guard]) =>
		score(
			JSON.parse(
				fs.readFileSync(path.join(root, "evals/agent-traces", name + ".json")),
			),
			path.join(root, guard, "skill"),
		),
	);
	const comparison = compareSkills(root);
	return {
		schema_version: "2",
		execution: "replay of recorded real agent actions; no live LLM calls in CI",
		ready: results.every((r) => r.ready) && comparison.ready,
		current: results.every((r) => r.current),
		stale: results.flatMap((r) =>
			r.stale.map((s) => ({ language: r.language, ...s })),
		),
		results,
		comparison,
	};
}
if (require.main === module) {
	const report = evaluate();
	const requireCurrent = process.argv.includes("--require-current");
	console.log(JSON.stringify(report, null, 2));
	for (const s of report.stale)
		console.log(
			`::warning::stale agent trace ${s.language}.${s.session} (${s.reason}: ${s.files.join(", ") || "inventory"}); re-record before release`,
		);
	for (const r of report.results)
		for (const c of r.checks.filter((c) => c.outcome === "fail"))
			console.log(`::error::agent trace ${r.language} failed ${c.id}`);
	console.error(
		`agent-evals: ${report.results.flatMap((r) => r.checks).filter((c) => c.outcome === "fail").length} fail, ${report.stale.length} stale`,
	);
	process.exitCode = report.ready && (report.current || !requireCurrent) ? 0 : 1;
}
module.exports = {
	score,
	evaluate,
	freshness,
	readSkillBundle,
	skillInventory,
	skillFileHashes,
	compareSkills,
	REQUIRED_SESSIONS,
};
