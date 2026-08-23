package main

import (
	"bytes"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: trimdoc FILE")
		os.Exit(2)
	}
	contents, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	contents = append(bytes.TrimRight(contents, "\r\n"), '\n')
	if err := os.WriteFile(os.Args[1], contents, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
