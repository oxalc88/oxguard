// A native subprocess fixture for testing launcher arguments, streams and signals.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	switch os.Args[1] {
	case "context":
		cwd, _ := os.Getwd()
		json.NewEncoder(os.Stdout).Encode(map[string]any{
			"cwd": cwd, "argv": os.Args[2:], "value": os.Getenv("TSGUARD_FIXTURE_VALUE"),
			"runtime": os.Getenv("TSGUARD_RUNTIME"), "node": os.Getenv("TSGUARD_NODE"), "opengrep": os.Getenv("TSGUARD_OPENGREP"),
		})
	case "exit":
		fmt.Println(os.Args[3])
		fmt.Fprintln(os.Stderr, "fixture stderr")
		code, _ := strconv.Atoi(os.Args[2])
		os.Exit(code)
	case "stdin":
		var input string
		fmt.Scanln(&input)
		fmt.Println(input)
	case "wait":
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
		fmt.Println("ready")
		fmt.Println(<-ch)
		os.Exit(42)
	case "signal":
		p, _ := os.FindProcess(os.Getpid())
		_ = p.Signal(syscall.SIGTERM)
		select {}
	}
}
