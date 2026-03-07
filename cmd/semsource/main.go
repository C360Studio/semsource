// Command semsource is the SemSource graph ingestion service.
//
// It reads a JSON config file, ingests configured sources through registered
// handlers, normalizes the results into a knowledge graph, and continuously
// emits graph events (SEED, DELTA, RETRACT, HEARTBEAT) to downstream consumers
// via the configured output (WebSocket broadcast on :7890 by default).
//
// Usage:
//
//	semsource init        Interactive setup wizard
//	semsource run         Start the ingestion engine
//	semsource add         Add a source to existing config
//	semsource sources     List configured sources
//	semsource validate    Check config without starting
//	semsource version     Print version
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/c360studio/semsource/cli"
)

const version = "0.1.0"

func main() {
	if err := dispatch(); err != nil {
		fmt.Fprintf(os.Stderr, "semsource: %v\n", err)
		os.Exit(1)
	}
}

func dispatch() error {
	if len(os.Args) < 2 {
		// No subcommand: run if config exists, otherwise show usage.
		if _, err := os.Stat("semsource.json"); err == nil {
			return runCmd(nil)
		}
		usage()
		return nil
	}

	switch os.Args[1] {
	case "init":
		return initCmd(os.Args[2:])
	case "run":
		return runCmd(os.Args[2:])
	case "add":
		return addCmd(os.Args[2:])
	case "sources":
		return sourcesCmd(os.Args[2:])
	case "validate":
		return validateCmd(os.Args[2:])
	case "version":
		fmt.Printf("semsource %s\n", version)
		return nil
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		fmt.Fprintf(os.Stderr, "semsource: unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
		return nil
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: semsource <command> [options]

Commands:
  init        Interactive setup wizard — creates semsource.json
  run         Start the ingestion engine
  add         Add a source to an existing config
  sources     List configured sources
  validate    Check config without starting
  version     Print version

Run 'semsource <command> -h' for command-specific options.
`)
}

// initCmd runs the interactive setup wizard.
func initCmd(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	configPath := fs.String("config", "semsource.json", "path to write config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	term := cli.NewTerm(os.Stdin, os.Stdout)
	return cli.Init(term, *configPath)
}

// addCmd adds a source to an existing config.
func addCmd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	configPath := fs.String("config", "semsource.json", "path to semsource JSON config file")
	// Parse only the --config flag; remaining args are passed to Add.
	// We need to find --config before the source type argument.
	remaining := parseGlobalFlag(args, fs)
	term := cli.NewTerm(os.Stdin, os.Stdout)
	return cli.Add(term, *configPath, remaining)
}

// sourcesCmd lists configured sources.
func sourcesCmd(args []string) error {
	fs := flag.NewFlagSet("sources", flag.ContinueOnError)
	configPath := fs.String("config", "semsource.json", "path to semsource JSON config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	term := cli.NewTerm(os.Stdin, os.Stdout)
	return cli.Sources(term, *configPath)
}

// validateCmd checks the config without starting the engine.
func validateCmd(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	configPath := fs.String("config", "semsource.json", "path to semsource JSON config file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	term := cli.NewTerm(os.Stdin, os.Stdout)
	return cli.Validate(term, *configPath)
}

// buildLogger constructs an slog.Logger at the given level writing JSON to stdout.
func buildLogger(level string) *slog.Logger {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

// parseGlobalFlag parses flags from args that are defined in fs, returning
// any positional args and non-flag args that appear before the first flag.
// This allows: semsource add --config foo.json ast --path ./src
func parseGlobalFlag(args []string, fs *flag.FlagSet) []string {
	// Collect flags for fs, stop at first non-flag arg.
	var remaining []string
	i := 0
	for i < len(args) {
		arg := args[i]
		if len(arg) > 0 && arg[0] == '-' {
			// It's a flag — feed it to fs.
			if err := fs.Parse(args[i:]); err == nil {
				remaining = append(remaining, fs.Args()...)
				return remaining
			}
			i++
		} else {
			// Non-flag (e.g., source type like "ast") — collect as remaining.
			remaining = append(remaining, arg)
			i++
		}
	}
	return remaining
}
