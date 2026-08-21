package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/d31ma/sluice/internal/transport/yon"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Sluice Yon handler rejected the request")
		os.Exit(1)
	}
}

func run(arguments []string, input io.Reader, output io.Writer) error {
	sessionFile, err := parseSessionFile(arguments)
	if err != nil {
		return err
	}
	return serveSession(input, output, sessionFile)
}

func parseSessionFile(arguments []string) (string, error) {
	flags := flag.NewFlagSet("sluice-yon", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sessionFile := flags.String("session-file", "", "absolute path to the live agent session descriptor")
	if err := flags.Parse(arguments); err != nil || flags.NArg() > 1 {
		return "", fmt.Errorf("invalid Yon handler options")
	}
	return *sessionFile, nil
}

func serveSession(input io.Reader, output io.Writer, sessionFile string) error {
	if sessionFile == "" {
		return yon.Serve(input, output)
	}
	return yon.ServeWithSessionFile(input, output, sessionFile)
}
