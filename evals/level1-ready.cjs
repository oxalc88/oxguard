"use strict";
const fs = require("node:fs");
const { parity } = require("./parity.cjs");
const { evaluate } = require("./agent-actions.cjs");
// Agent traces are recorded evidence, not a deterministic gate: on pull requests
// stale or failing traces only warn; releases (strict) require ready and current.
function assess(ts, behavior, python, { strict = false } = {}) {
	const expectedTs = require("./cases.json")
		.cases.map((c) => c.id)
		.sort();
	const expectedPy = require("./python/cases.json").cases;
	const same = (report, ids) =>
		JSON.stringify(report.cases.map((c) => c.id).sort()) ===
		JSON.stringify(ids.sort());
	if (
		!same(ts, expectedTs) ||
		!same(
			behavior,
			expectedPy.filter((c) => c.lane === "behavior").map((c) => c.id),
		) ||
		!same(
			python,
			expectedPy.filter((c) => c.lane === "parity").map((c) => c.id),
		)
	)
		throw Error("Completion requires every case in the current corpus");
	const shared = parity(ts, behavior, python),
		agents = evaluate();
	const ready =
		shared.shared_capabilities_ready &&
		ts.cases.every((c) => c.outcome === "pass") &&
		behavior.cases.every((c) => c.outcome === "pass") &&
		python.cases.every((c) => c.outcome === "pass");
	const complete = ready && agents.ready && agents.current;
	return {
		schema_version: "1",
		level1_agent_contract_ready: complete,
		deterministic_ready: ready,
		agent_traces_ready: agents.ready,
		agent_traces_current: agents.current,
		strict,
		blocking_ready: strict ? complete : ready,
		shared_capabilities_ready: shared.shared_capabilities_ready,
		agent_actions: agents,
		level2_remaining: "Tsguard static module graph and advisory delegation are implemented; new PyGuard structural parity is not evaluated by this Level 1 report",
		level3: "Tsguard inert Git source comparisons are implemented; change-quality and new Python parity require the separate maintainability eval report",
		scope:
			"Current default Level 1 analyzers and documented adapters; unsupported backends remain explicitly partial. Readiness does not claim complete upstream report reproduction or production precision.",
	};
}
if (require.main === module) {
	if (process.argv.length !== 6)
		throw Error(
			"Required: TS report, Python behavior report, Python parity report, output path",
		);
	const result = assess(
		...process.argv
			.slice(2, 5)
			.map((file) => JSON.parse(fs.readFileSync(file))),
		{ strict: process.env.OXGUARD_REQUIRE_CURRENT_TRACES === "1" },
	);
	if (!result.agent_traces_ready || !result.agent_traces_current)
		console.log(
			`::warning::agent traces ready=${result.agent_traces_ready} current=${result.agent_traces_current}; releases require both (re-record stale sessions)`,
		);
	fs.writeFileSync(process.argv[5], JSON.stringify(result, null, 2) + "\n");
	console.log(JSON.stringify(result));
	process.exitCode = result.blocking_ready ? 0 : 1;
}
module.exports = { assess };
