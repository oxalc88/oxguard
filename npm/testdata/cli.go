// A native subprocess fixture for testing launcher arguments, streams and signals.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

func main() {
	switch os.Args[1] {
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
