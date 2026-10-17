package contract

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func blockedResult(t *testing.T) *RunResult {
	t.Helper()
	r := New("test", "types")
	r.Plan("types")
	r.AddFinding(Finding{Gate: "types", Rule: "type.error", Status: StatusBlocking})
	r.RecordGate("types", false, true)
	if err := r.Finish(1); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestNumericStatesRetainSchemaOneStrings(t *testing.T) {
	if reflect.TypeOf(StatusPass).Kind() != reflect.Uint8 || StatusUnknown != 0 || CompletenessUnknown != 0 {
		t.Fatal("states must be numeric and zero unknown")
	}
	r := blockedResult(t)
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`"status":"fail"`, `"status":"blocking"`, `"status":"failed"`, `"assessment":"complete"`, `"normalization":"complete"`, `"schema_version":"1"`} {
		if !bytes.Contains(data, []byte(value)) {
			t.Fatalf("wire compatibility lost: %s", data)
		}
	}
	var decoded RunResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != StatusFail || decoded.Findings[0].Status != StatusBlocking {
		t.Fatal(decoded)
	}
}
func TestInvalidWireStatesAreRejectedWithoutPartialAssignment(t *testing.T) {
	base, err := json.Marshal(blockedResult(t))
	if err != nil {
		t.Fatal(err)
	}
	changes := []struct{ from, to string }{
		{`"status":"fail"`, `"status":"future"`},
		{`"status":"fail"`, `"status":0`},
		{`"status":"fail"`, `"status":null`},
		{`"status":"fail"`, `"status":"advisory"`},
		{`"status":"blocking"`, `"status":"pass"`},
		{`"status":"failed"`, `"status":"passed"`},
		{`"normalization":"complete"`, `"normalization":"not_run"`},
		{`"assessment":"complete"`, `"assessment":"partial"`},
	}
	for _, change := range changes {
		t.Run(change.to, func(t *testing.T) {
			value := New("sentinel", "unchanged")
			data := strings.Replace(string(base), change.from, change.to, 1)
			if err := json.Unmarshal([]byte(data), value); err == nil {
				t.Fatalf("invalid state accepted: %s", data)
			}
			if value.Command != "unchanged" {
				t.Fatal("invalid decode partially assigned result")
			}
		})
	}
}
func TestUnknownInternalStatesCannotBeReportedAsAdvisory(t *testing.T) {
	for _, mutate := range []func(*RunResult){
		func(r *RunResult) { r.Status = StatusUnknown },
		func(r *RunResult) { r.Status = Status(255) },
		func(r *RunResult) { r.Findings[0].Status = StatusUnknown },
		func(r *RunResult) { r.Findings[0].Status = StatusPass },
		func(r *RunResult) { r.Gates[0].Status = StatusBlocking },
		func(r *RunResult) { r.Gates[0].Normalization = CompletenessUnknown },
		func(r *RunResult) { r.Assessment = Complete; r.Gates[0].Normalization = Partial },
		func(r *RunResult) { r.Gates = append(r.Gates, r.Gates[0]) },
	} {
		r := blockedResult(t)
		mutate(r)
		if r.Validate() == nil {
			t.Fatal("invalid result passed validation")
		}
		for _, mode := range []string{"agent", "json"} {
			var out bytes.Buffer
			if Report(&out, mode, r) == nil || out.Len() != 0 {
				t.Fatal("invalid contract was emitted")
			}
		}
		if _, err := json.Marshal(r); err == nil {
			t.Fatal("invalid contract marshaled directly")
		}
	}
}
func TestFinishDerivesGateFailuresAndPreservesLegacyErrorExit(t *testing.T) {
	r := New("test", "types")
	r.Plan("types")
	r.RecordGate("types", false, true)
	if err := r.Finish(1); err != nil || r.Status != StatusFail {
		t.Fatal("failed gate without findings became pass", err)
	}
	r.Execution("types", "tool_missing", "missing")
	if err := r.Finish(0); err != nil || r.Status != StatusError || r.ExitCode != 0 || r.Assessment != Incomplete {
		t.Fatal("legacy error semantics changed", err)
	}
	r.Findings[0].Status = Status(255)
	if err := r.Finish(0); err == nil {
		t.Fatal("unknown state finalized")
	}
}
