package main

import "github.com/oxalc88/oxguard/contract"

type RunResult = contract.RunResult
type Finding = contract.Finding
type Location = contract.Location
type Measurement = contract.Measurement
type Artifact = contract.Artifact
type Diagnostic = contract.Diagnostic

func newRunResult(command string) *RunResult { return contract.New("pyguard", command) }
func relativePath(root, path string) string  { return contract.RelativePath(root, path) }
