# Native coverage contract fixture

Five equal functions each have three source lines, one statement and no conditional branches. Cases call three, four or five functions, giving 60%, 80% or 100% coverage for lines/statements/functions and 100% branch coverage. The failed-test case calls all five before its last assertion fails. These expectations are hand-counted controls, not copied OxGuard output. The contract must preserve numeric measurements, existing global thresholds and test failures independently.
