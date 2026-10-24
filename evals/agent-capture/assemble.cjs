"use strict";
// Builds schema-2 agent traces from saved capture folders, or migrates a v1 trace
// whose bundle hash still matches the skill it was recorded with. It never invents
// actions: events, responses and skill reads come from the recorded files.
const fs = require("node:fs"),
	path = require("node:path"),
	crypto = require("node:crypto");
const {
	readSkillBundle,
	skillInventory,
	skillFileHashes,
	REQUIRED_SESSIONS,
} = require("../agent-actions.cjs");
const GUARD = { typescript: "tsguard", python: "pyguard" };
const sha = (file) =>
	crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
const lines = (file) =>
	fs.readFileSync(file, "utf8").split("\n").filter(Boolean).map((l) => JSON.parse(l));
const canonical = (file) =>
	file.match(/(?:^|\/)(SKILL\.md|(?:workflows|references)\/[\w-]+\.md)$/)?.[1];
// Declared minimum: the first file of each required group not already observed.
function declared(session, observed) {
	return REQUIRED_SESSIONS[session]
		.filter((group) => !group.some((f) => observed.includes(f)))
		.map((group) => group[0]);
}
// Skill files read by successful shell reads in a Codex or pi JSON stream.
function observedReads(stream, guard) {
	const reads = [],
		unknown = [];
	const pattern = new RegExp(
		`\\.agents/skills/${guard}/((?:SKILL\\.md|(?:workflows|references)/[\\w-]+\\.md))`,
		"g",
	);
	for (const event of lines(stream)) {
		const item = event.item || {};
		if (event.type === "item.completed" && item.type === "command_execution") {
			if (item.exit_code !== 0 || !/\b(cat|sed|head|tail|nl|less|awk)\b/.test(item.command)) continue;
			for (const [, file] of item.command.matchAll(pattern))
				if (!reads.includes(file)) reads.push(file);
		} else if (
			event.type === "item.completed" &&
			["mcp_tool_call", "file_read"].includes(item.type) &&
			JSON.stringify(item).includes(`skills/${guard}/`)
		)
			unknown.push(item.type);
	}
	if (unknown.length)
		throw Error(`${stream}: unclassified skill access via ${unknown.join(", ")}`);
	return reads;
}
function lastJson(file) {
	const text = fs.readFileSync(file, "utf8").trim();
	return JSON.parse(text.slice(text.indexOf("{"), text.lastIndexOf("}") + 1));
}
// "exit 0 effort medium model gpt-6.1-sol" written by run-codex.sh
function agentOf(dir) {
	const [, , , effort, , model] = fs.readFileSync(path.join(dir, "agent-exit.txt"), "utf8").split(/\s+/);
	return { agent: "codex-cli 0.164.0 exec", model, effort };
}
function session(language, name, dir) {
	const guard = GUARD[language],
		skill = path.join(dir, "project/.agents/skills", guard);
	const stream = path.join(dir, "agent-stream.jsonl");
	const observed = observedReads(stream, guard);
	const files = [...observed, ...declared(name, observed)];
	return {
		record: {
			...agentOf(dir),
			read_source: "observed+declared",
			observed_reads: observed,
			skill_files: skillFileHashes(skill, files),
			stream_sha256: sha(stream),
			events_sha256: sha(path.join(dir, "events.jsonl")),
		},
		inventory: skillInventory(skill),
		reads: observed,
		response: lastJson(path.join(dir, "agent-last-message.txt")),
	};
}
function fromCaptures(language, dirs, note = "") {
	const guard = GUARD[language];
	const s = Object.fromEntries(
		["quality", "repair", "review", "focused"].map((n) => [n, session(language, n, dirs[n])]),
	);
	const inventories = new Set(Object.values(s).map((x) => JSON.stringify(x.inventory)));
	if (inventories.size !== 1) throw Error("Sessions were recorded with different skill inventories");
	const q = lines(path.join(dirs.quality, "events.jsonl")),
		repair = lines(path.join(dirs.repair, "events.jsonl"));
	const scored = (e) => [guard, "npx"].includes(e.tool) && e.argv.includes("src");
	const route = (v) => v?.match(/workflows\/[\w-]+\.md/)?.[0] || v;
	const raw = {
		...s.review.response,
		quality_summary: s.quality.response.quality_summary ?? s.quality.response,
		repair_summary: s.repair.response.repair_summary ?? s.repair.response,
	};
	const response = structuredClone(raw);
	response.routes = Object.fromEntries(Object.entries(response.routes).map(([k, v]) => [k, route(v)]));
	response.skill_reads = s.review.reads;
	const efforts = [...new Set(Object.values(s).map((x) => `${x.record.model} effort ${x.record.effort}`))];
	const trace = {
		schema_version: "2",
		language,
		skill_inventory: [...s.quality.inventory],
		sessions: Object.fromEntries(Object.entries(s).map(([k, v]) => [k, v.record])),
		capture_date: new Date().toISOString().slice(0, 10),
		provenance: `Fresh history-free Codex agents (codex-cli 0.164.0 exec; ${efforts.join("; ")}; --ephemeral --ignore-user-config --ignore-rules, memories disabled, no login shell, sandbox danger-full-access with the fixture project as working directory), one agent per session, using the router skill installed in disposable projects outside the repository. Per-session skill_files hash the installed skill files each agent read (successful shell reads in its stream) plus the declared minimum. Setup supplied the actual bounded quality, missing-tool, advisory${language === "typescript" ? " and change" : ""} results; subsequent retrieval, edits, offline installation and verification were agent choices recorded by evals/record-agent-command.cjs. Scored quality events keep the original --dirs src scope; wider exploratory invocations are recorded separately.`,
		additional_verification: note,
		agent_tasks: Object.keys(s).map((k) => `codex_${language}_${k}`),
		omitted_events: q.filter(scored),
		additional_quality_events: q.filter((e) => !scored(e)),
		repair_events: repair,
		interpretation: {
			advisory: lines(path.join(dirs.review, "events.jsonl"))[0].result,
			execution: repair[0].result,
			response,
			raw_response: raw,
		},
	};
	if (language === "typescript") trace.change_events = lines(path.join(dirs.change, "events.jsonl"));
	trace.skill_layout = "router";
	trace.focused_review = {
		input: lines(path.join(dirs.focused, "events.jsonl"))[0].result,
		response: { ...s.focused.response, skill_reads: s.focused.reads },
		raw_response: s.focused.response,
	};
	trace.raw_capture_paths = Object.fromEntries(
		Object.entries(dirs).map(([k, d]) => [k, d]),
	);
	return trace;
}
// v1 -> v2 without new agent runs: valid only while the v1 bundle hash still matches.
function fromV1(trace, skillDir) {
	const bundle = crypto.createHash("sha256")
		.update(readSkillBundle(skillDir).toString().replace(/\r\n/g, "\n")).digest("hex");
	if (String(trace.schema_version) !== "1" || trace.skill_sha256 !== bundle)
		throw Error("v1 trace does not match this skill; re-record instead of migrating");
	const inventory = skillInventory(skillDir);
	const reads = {
		review: trace.interpretation?.response?.skill_reads,
		focused: trace.focused_review?.response?.skill_reads,
	};
	const sessions = {};
	for (const name of Object.keys(REQUIRED_SESSIONS)) {
		const observed = (reads[name] || []).map(canonical).filter(Boolean);
		const files = reads[name] ? [...new Set([...observed, ...declared(name, observed)])] : inventory;
		sessions[name] = {
			read_source: reads[name] ? "v1 recorded skill_reads+declared" : "v1 bundle (reads not recorded)",
			skill_files: skillFileHashes(skillDir, files),
		};
	}
	const { skill_sha256, schema_version, language, ...rest } = trace;
	return { schema_version: "2", language, skill_inventory: inventory, sessions, migrated_from_v1_skill_sha256: skill_sha256, ...rest };
}
if (require.main === module) {
	const [mode, language, ...args] = process.argv.slice(2);
	let trace;
	if (mode === "from-v1") trace = fromV1(JSON.parse(fs.readFileSync(args[0])), args[1]);
	else if (mode === "captures") {
		// captures <language> <capture-root> [session=dir ...] [--note file]
		const [root, ...rest] = args, dirs = {};
		for (const n of ["quality", "repair", "review", "focused", "change"]) dirs[n] = path.join(root, language, n);
		let note = "";
		for (let i = 0; i < rest.length; i++) {
			if (rest[i] === "--note") note = fs.readFileSync(rest[++i], "utf8").trim();
			else { const [k, v] = rest[i].split("="); dirs[k] = path.resolve(v); }
		}
		if (language !== "typescript") delete dirs.change;
		trace = fromCaptures(language, dirs, note);
	} else throw Error("Usage: assemble.cjs from-v1 <language> <v1.json> <skill-dir> | captures <language> <root> [session=dir] [--note file]");
	process.stdout.write(JSON.stringify(trace, null, 2) + "\n");
}
module.exports = { fromCaptures, fromV1, observedReads };
