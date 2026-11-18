package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

type invocation struct {
	config string
	in     *os.File
	out    io.Writer
	errs   io.Writer
}

type subcommand struct {
	name    string
	summary string
	run     func(ctx context.Context, call invocation, args []string) error
}

var subcommands = []subcommand{
	{"init", "Create a local config from the example; never overwrite.", initConfig},
	{"set-secret", "Save a Beeper token through a hidden prompt.", setSecret},
	{"doctor", "Check local API availability; group reads require --chat.", doctor},
	{"accounts", "List connected Beeper accounts and connection status.", accounts},
	{"list-groups", "List recent group chats (metadata only, no message bodies).", listGroups},
	{"find-chat", "Search group metadata using your exact title words.", findChat},
	{"inspect-chat", "Inspect metadata for one explicitly selected chat ID.", inspectChat},
	{"arm", "Start a ballot database at the newest message; old commands are skipped.", arm},
	{"run", "Poll the group and reply. Without --live, replies are previews only.", runBot},
	{"status", "Show nominations and replies needing attention (read-only; safe while running).", status},
	{"feedback", "List !gtfeedback and !gtsuggest entries (read-only; safe while running).", feedback},
	{"resolve-send", "Record a checked outcome for an uncertain or failed reply.", resolveSend},
	{"busy", "Exit non-zero while a photo search is running (read-only; the deploy uses it).", busy},
	{"backup", "Copy each existing database into backups/ beside it (safe while running; the deploy uses it).", backup},
	{"generate-media", "Run the subscription-only multi-model photo strip builder.", generateMedia},
}

type usageError struct {
	error
}

func main() {
	syscall.Umask(0o077)
	os.Exit(execute(context.Background(), os.Args[1:], invocation{in: os.Stdin, out: os.Stdout, errs: os.Stderr}))
}

func execute(ctx context.Context, args []string, call invocation) int {
	err := dispatch(ctx, args, call)
	var usage usageError
	switch {
	case err == nil || errors.Is(err, flag.ErrHelp):
		return 0
	case errors.As(err, &usage):
		fmt.Fprintln(call.errs, "gentalent:", usage.error)
		return 2
	default:
		fmt.Fprintln(call.errs, "GenTalent:", err)
		return 1
	}
}

func dispatch(ctx context.Context, args []string, call invocation) error {
	global := flag.NewFlagSet("gentalent", flag.ContinueOnError)
	global.SetOutput(call.errs)
	global.StringVar(&call.config, "config", "config.json", "path to config.json")
	global.Usage = func() { printUsage(call.errs, global) }
	if err := global.Parse(args); err != nil {
		return usageFailure(err)
	}
	if global.NArg() == 0 {
		printUsage(call.errs, global)
		return usageError{errors.New("a subcommand is required")}
	}
	name := global.Arg(0)
	for _, command := range subcommands {
		if command.name == name {
			return command.run(ctx, call, global.Args()[1:])
		}
	}
	printUsage(call.errs, global)
	return usageError{fmt.Errorf("unknown subcommand %q", name)}
}

func printUsage(w io.Writer, global *flag.FlagSet) {
	fmt.Fprintln(w, "usage: gentalent [--config PATH] <subcommand> [flags]")
	global.PrintDefaults()
	for _, command := range subcommands {
		fmt.Fprintf(w, "  %-15s %s\n", command.name, command.summary)
	}
}

func usageFailure(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return err
	}
	return usageError{err}
}

func newFlags(call invocation, name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(call.errs)
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string, names ...string) ([]string, error) {
	var positional []string
	for {
		if err := flags.Parse(args); err != nil {
			return nil, usageFailure(err)
		}
		rest := flags.Args()
		if len(rest) == 0 {
			break
		}
		if consumed := len(args) - len(rest); consumed > 0 && args[consumed-1] == "--" {
			positional = append(positional, rest...)
			break
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
	if len(positional) != len(names) {
		return nil, usageError{fmt.Errorf("%s needs %d argument(s) (%s), got %d", flags.Name(), len(names), strings.Join(names, ", "), len(positional))}
	}
	return positional, nil
}

func printJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
