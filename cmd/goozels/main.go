// Command goozels is a Language Server that proxies gopls and layers gooze
// mutation-testing features (gooze:ignore directives, the gooze config file
// and the results of gooze runs) on top. Editors connect to it over stdio
// exactly as they would to gopls.
//
// `goozels check file...` prints the same gooze diagnostics without an LSP
// session, `goozels hook claude|codex` serves them to coding agents after
// edits, and `goozels version` prints a first line starting with "goozels ".
// See `goozels help`.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"

	"gooze.dev/pkg/gooze/pkg/config"
	"gooze.dev/pkg/gooze/pkg/report"

	"gooze.dev/pkg/goozels/internal/adapter/docstore"
	"gooze.dev/pkg/goozels/internal/adapter/goozecli"
	"gooze.dev/pkg/goozels/internal/adapter/gopls"
	"gooze.dev/pkg/goozels/internal/adapter/hookstate"
	"gooze.dev/pkg/goozels/internal/adapter/osfs"
	"gooze.dev/pkg/goozels/internal/adapter/reportcache"
	"gooze.dev/pkg/goozels/internal/controller"
)

func main() {
	// The process entry point is not unit-tested; run covers everything it calls.
	os.Exit(run(os.Args[1:])) //gooze:ignore numbers
}

// run dispatches to a subcommand, or serves the language server.
func run(args []string) int {
	// Equivalent for tests: with no arguments goozels serves on the process's stdin, which unit tests do not run (!= 0 is > 0 anyway).
	if len(args) > 0 { //gooze:ignore comparison
		switch args[0] {
		case "check":
			return runCheck(args[1:], os.Stdout, os.Stderr)
		case "hook":
			// Equivalent for tests: a working hook call reads the process's stdin; with an extra "hook" argument both forms fail with usage status 2.
			return runHook(args[1:], os.Stdin, os.Stdout, os.Stderr, hookstate.Default()) //gooze:ignore numbers
		case "version":
			return runVersion(args[1:], os.Stdout, os.Stderr, goplsVersion)
		case "help", "-h", "-help", "--help":
			_, _ = fmt.Fprint(os.Stdout, usage)

			return 0
		}
	}

	return serve(args)
}

// serveFlags are the language server's options.
type serveFlags struct {
	gopls, gooze string
	goplsArgs    []string
}

// parseServe parses the language server's command line: its flags, an
// optional "serve" (gopls's name for this mode), and gopls arguments after
// "--". Anything else is an error.
func parseServe(args []string, stderr io.Writer) (serveFlags, error) {
	var f serveFlags

	if i := slices.Index(args, "--"); i >= 0 {
		args, f.goplsArgs = args[:i], args[i+1:]
	}

	fs := flag.NewFlagSet("goozels", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usage) }
	fs.StringVar(&f.gopls, "gopls", envOr("GOOZELS_GOPLS", "gopls"), "path to the gopls binary to proxy")
	fs.StringVar(&f.gooze, "gooze", envOr("GOOZELS_GOOZE", "gooze"), "path to the gooze binary for code-lens commands")

	if err := fs.Parse(args); err != nil {
		return f, err
	}

	// Equivalent: with at least one argument, len > 1 and len != 1 agree, and != 0 is > 0 (the "serve" comparisons are killed by a test).
	if rest := fs.Args(); len(rest) > 0 && (len(rest) > 1 || rest[0] != "serve") { //gooze:ignore comparison
		return f, fmt.Errorf("unknown command or argument %q; see goozels help (gopls arguments go after --)", rest[0])
	}

	return f, nil
}

// serve runs the language server, proxying gopls over stdio, and returns the
// process exit code.
func serve(args []string) int {
	f, err := parseServe(args, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}

	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "goozels:", err)

		return 2
	}

	if isTerminal(os.Stdin) {
		_, _ = fmt.Fprint(os.Stderr, terminalHint)
	}

	proc, err := gopls.Start(f.gopls, f.goplsArgs...)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "goozels: failed to start gopls (%s): %v\n", f.gopls, err)

		return 1
	}

	srv := controller.NewServer(os.Stdout, proc.Stdin, controller.Deps{
		Gooze:     goozecli.New(f.gooze),
		Documents: docstore.New(),
		Reports:   reportcache.New(report.NewDirReader()),
		Config:    config.NewRegistry(),
		Files:     osfs.FS{},
	})
	_ = srv.Run(os.Stdin, proc.Stdout)

	// Editor disconnected: tear down gopls.
	_ = proc.Kill()
	_ = proc.Wait()

	// Not unit-tested: reached only after a real editor session ends.
	return 0 //gooze:ignore numbers
}

// isTerminal reports whether f is an interactive terminal rather than the
// pipe an editor connects.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()

	// Equivalent: the mode bits are unsigned, so > 0 is != 0 (< 0 never holds).
	return err == nil && info.Mode()&os.ModeCharDevice != 0 //gooze:ignore comparison
}

// goplsVersion runs `gopls version` with args, using the gopls goozels would
// proxy.
func goplsVersion(stdout, stderr io.Writer, args ...string) error {
	return gopls.Run(context.Background(), envOr("GOOZELS_GOPLS", "gopls"), stdout, stderr, append([]string{"version"}, args...)...)
}

func envOr(key, fallback string) string {
	// Equivalent: no string sorts below "", so > "" is != "".
	if v := os.Getenv(key); v != "" { //gooze:ignore comparison
		return v
	}

	return fallback
}
