package main

import "github.com/oxalc88/oxguard/contract"

type RunResult = contract.RunResult
type Finding = contract.Finding
type Location = contract.Location
type Measurement = contract.Measurement
type Artifact = contract.Artifact
type Diagnostic = contract.Diagnostic

func newRunResult(command string) *RunResult { return contract.New("tsguard", command) }
func relativePath(root, path string) string  { return contract.RelativePath(root, path) }

// Shared numeric states preserve the schema-1 wire strings.
type Status = contract.Status

const (
	StatusUnknown        = contract.StatusUnknown
	StatusPass           = contract.StatusPass
	StatusFail           = contract.StatusFail
	StatusError          = contract.StatusError
	StatusAdvisory       = contract.StatusAdvisory
	StatusSkipped        = contract.StatusSkipped
	StatusBlocking       = contract.StatusBlocking
	StatusExecutionError = contract.StatusExecutionError
	StatusNotRun         = contract.StatusNotRun
	StatusPassed         = contract.StatusPassed
	StatusFailed         = contract.StatusFailed
	Complete             = contract.Complete
	Incomplete           = contract.Incomplete
	Partial              = contract.Partial
	NotRun               = contract.NotRun
)
