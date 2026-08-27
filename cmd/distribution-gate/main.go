// Command distribution-gate validates immutable self-hosted distribution
// evidence. It never creates a release file, package, SBOM, license, notice,
// or OCI image.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/araihu/manja/internal/distribution"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	var err error
	switch args[0] {
	case "canonical":
		err = runCanonical(args[1:], stdout, stderr)
	case "check":
		return runCheck(args[1:], stdout, stderr)
	default:
		usage(stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	return 0
}

func runCanonical(args []string, stdout, stderr io.Writer) error {
	input, closeInput, err := openInput(args, stderr)
	if err != nil {
		return err
	}
	defer closeInput()
	evidence, err := distribution.DecodeStrict(input)
	if err != nil {
		return fmt.Errorf("decode evidence: %w", err)
	}
	canonical, err := distribution.MarshalCanonical(evidence)
	if err != nil {
		return err
	}
	_, err = stdout.Write(canonical)
	return err
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	input, closeInput, err := openInput(args, stderr)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	defer closeInput()
	evidence, err := distribution.DecodeStrict(input)
	if err != nil {
		fmt.Fprintf(stderr, "decode evidence: %v\n", err)
		return 2
	}
	result := distribution.Evaluate(evidence, distribution.DefaultPolicy())
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "encode result: %v\n", err)
		return 2
	}
	encoded = append(encoded, '\n')
	if _, err := stdout.Write(encoded); err != nil {
		fmt.Fprintf(stderr, "write result: %v\n", err)
		return 2
	}
	if result.Status != distribution.StatusPass {
		return 1
	}
	return 0
}

func openInput(args []string, stderr io.Writer) (io.Reader, func(), error) {
	flags := flag.NewFlagSet("distribution-gate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	inputPath := flags.String("input", "-", "canonical evidence JSON path, or - for stdin")
	if err := flags.Parse(args); err != nil {
		return nil, func() {}, err
	}
	if flags.NArg() != 0 {
		return nil, func() {}, errors.New("unexpected positional argument")
	}
	if *inputPath == "-" {
		return os.Stdin, func() {}, nil
	}
	file, err := os.Open(*inputPath)
	if err != nil {
		return nil, func() {}, fmt.Errorf("open evidence: %w", err)
	}
	return file, func() { _ = file.Close() }, nil
}

func usage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: distribution-gate <canonical|check> [-input evidence.json]")
}
