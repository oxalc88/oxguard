package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// lambdaInvoke runs aws lambda invoke, writing the response to a temp file.
// Returns the temp file path, a cleanup function, and an exit code (0 = success).
func lambdaInvoke(fnName, payload string) (string, func(), int) {
	tmp, err := os.CreateTemp("", "pyguard-lambda-*.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not create temp file: %v\n", err)
		return "", func() {}, 1
	}
	tmp.Close()
	cleanup := func() { os.Remove(tmp.Name()) } //nolint:errcheck

	cmd := exec.Command("aws", "lambda", "invoke",
		"--function-name", fnName,
		"--payload", payload,
		"--cli-binary-format", "raw-in-base64-out",
		tmp.Name(),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: aws lambda invoke failed: %v\n", err)
		return tmp.Name(), cleanup, 1
	}
	return tmp.Name(), cleanup, 0
}

// runInvoke wraps `aws lambda invoke` and pretty-prints the JSON response.
// Replaces: aws lambda invoke ... out.json && cat out.json | jq '.body | fromjson'
//
// Usage: pyguard invoke <function-name> '<json-payload>'
func runInvoke(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pyguard invoke <function-name> '<json-payload>'")
		fmt.Fprintln(os.Stderr, "example: pyguard invoke my-fn-dev '{\"client_id\":\"sergio\",\"message\":\"Hola\"}'")
		return 3
	}

	tmpPath, cleanup, code := lambdaInvoke(args[0], args[1])
	defer cleanup()
	if code != 0 {
		return code
	}
	return parseAndPrintResponse(tmpPath, false)
}

// runTest invokes the testing harness Lambda and prints the summary.
// Usage: pyguard test <function-name> <client-id>
func runTest(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: pyguard test <function-name> <client-id>")
		fmt.Fprintln(os.Stderr, "example: pyguard test my-fn-testing sergio")
		return 3
	}

	payload := fmt.Sprintf(`{"client_id":%q}`, args[1])
	tmpPath, cleanup, code := lambdaInvoke(args[0], payload)
	defer cleanup()
	if code != 0 {
		return code
	}
	return parseAndPrintResponse(tmpPath, true)
}

// parseAndPrintResponse reads the Lambda response file, parses .body (JSON string),
// and pretty-prints it. If summaryOnly, prints just the .summary field.
func parseAndPrintResponse(path string, summaryOnly bool) int {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: could not read response: %v\n", err)
		return 1
	}

	// Parse outer response
	var outer map[string]interface{}
	if err := json.Unmarshal(data, &outer); err != nil {
		// Not JSON — print raw
		fmt.Println(strings.TrimSpace(string(data)))
		return 0
	}

	// Lambda responses from this project wrap the actual body as a JSON string in .body
	body, hasBody := outer["body"]
	if !hasBody {
		// No .body field — print the whole response pretty
		return prettyPrint(outer)
	}

	bodyStr, isString := body.(string)
	if !isString {
		// .body is already an object — print it directly
		return prettyPrint(body)
	}

	// Parse the JSON string
	var parsed interface{}
	if err := json.Unmarshal([]byte(bodyStr), &parsed); err != nil {
		// Can't parse body — print raw
		fmt.Println(bodyStr)
		return 0
	}

	if summaryOnly {
		if m, ok := parsed.(map[string]interface{}); ok {
			if summary, ok := m["summary"]; ok {
				return prettyPrint(summary)
			}
		}
	}

	return prettyPrint(parsed)
}

func prettyPrint(v interface{}) int {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: json marshal: %v\n", err)
		return 1
	}
	fmt.Println(string(out))
	return 0
}
