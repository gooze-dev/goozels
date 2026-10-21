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
	os.Exit(run(os.Args[1:]))
}

// run dispatches to a subcommand, or serves the language server.
func run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "check":
			return runCheck(args[1:], os.Stdout, os.Stderr)
		case "hook":
			return runHook(args[1:], os.Stdin, os.Stdout, os.Stderr, hookstate.Default())
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
	gopls, gooze  string
	noUpdateCheck bool
	goplsArgs     []string
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
	fs.BoolVar(&f.noUpdateCheck, "no-update-check", false, "do not check for a newer goozels release (also "+noUpdateCheckEnv+")")

	if err := fs.Parse(args); err != nil {
		return f, err
	}

	// Equivalent: len(rest) > 0 has already held, so len(rest) > 1 and len(rest) != 1 agree.
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
		Releases:  releaseChecker(f.noUpdateCheck, version()),
		Version:   version(),
	})
	_ = srv.Run(os.Stdin, proc.Stdout)

	// Editor disconnected: tear down gopls.
	_ = proc.Kill()
	_ = proc.Wait()

	return 0
}

// isTerminal reports whether f is an interactive terminal rather than the
// pipe an editor connects.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()

	// Equivalent: file mode bits are unsigned, so > 0 is != 0.
	return err == nil && info.Mode()&os.ModeCharDevice != 0 //gooze:ignore comparison
}

// goplsVersion runs `gopls version` with args, using the gopls goozels would
// proxy.
func goplsVersion(stdout, stderr io.Writer, args ...string) error {
	return gopls.Run(context.Background(), envOr("GOOZELS_GOPLS", "gopls"), stdout, stderr, append([]string{"version"}, args...)...)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
