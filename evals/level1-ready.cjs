"use strict";
const fs = require("node:fs");
const { parity } = require("./parity.cjs");
const { evaluate } = require("./agent-actions.cjs");
function assess(ts, behavior, python) {
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
		python.cases.every((c) => c.outcome === "pass") &&
		agents.ready;
	return {
		schema_version: "1",
		level1_agent_contract_ready: ready,
		shared_capabilities_ready: shared.shared_capabilities_ready,
		agent_actions: agents,
		level2_remaining: "not implemented",
		level3: "not implemented",
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
	);
	fs.writeFileSync(process.argv[5], JSON.stringify(result, null, 2) + "\n");
	console.log(JSON.stringify(result));
	process.exitCode = result.level1_agent_contract_ready ? 0 : 1;
}
module.exports = { assess };
