package main

import (
	"fmt"
	"io"
	"runtime/debug"
)

// usage is the text of `goozels help`.
const usage = `goozels: gooze mutation-testing support for editors and coding agents.

Usage:
  goozels [-gopls PATH] [-gooze PATH] [-no-update-check] [serve] [-- GOPLS-ARGS...]
      Run the language server on stdin/stdout. Editors start goozels this way,
      in place of gopls; it runs gopls behind the scenes and adds gooze
      features. Arguments after -- are passed to gopls.
  goozels check [-workdir DIR] [-max N] [-no-update-check] FILE...
      Print gooze diagnostics for files as "file:line:col: severity: message".
      The workspace root (where .gooze-reports lives) is found by walking up
      from each file. Exit status: 0 no errors, 1 an error diagnostic was
      printed or a file could not be read, 2 usage error.
  goozels hook claude|codex
      Post-edit hook for Claude Code or Codex: reads the PostToolUse payload on
      stdin and prints {"hookSpecificOutput": {...}} with the edited files'
      gooze results.
  goozels version
      Print "goozels <version>". (version -json prints gopls's version, for
      editors that query the gopls they start.)
  goozels help
      Show this help.

Language server flags:
  -gopls PATH   gopls binary to proxy (default "gopls" on PATH)
  -gooze PATH   gooze binary run by the code lenses (default "gooze" on PATH)
  -no-update-check
                do not check for a newer goozels release (the check reads a
                daily-refreshed cache and never waits on the network)

Environment:
  GOOZELS_GOPLS   default for -gopls
  GOOZELS_GOOZE   default for -gooze
  GOOZELS_NO_UPDATE_CHECK
                  any value: same as -no-update-check
`

// terminalHint is printed when the language server's stdin is a terminal,
// which usually means someone ran goozels by hand.
const terminalHint = "goozels: running as a language server: it reads LSP messages on stdin and is " +
	"normally started by an editor. See goozels help for the check, hook and version commands; " +
	"press Ctrl+C to quit.\n"

// runVersion implements `goozels version`: one line, "goozels <version>",
// which scripts and plugins rely on to tell goozels from gopls. With -json it
// prints gopls's own version as JSON instead, as `gopls version -json` does,
// since editors such as VS Code's Go extension run that on the binary they
// start as gopls.
func runVersion(args []string, stdout, stderr io.Writer, gopls func(stdout, stderr io.Writer, args ...string) error) int {
	switch {
	case len(args) == 0:
		_, _ = fmt.Fprintln(stdout, "goozels "+version())

		return 0
	// Equivalent: the case above took len 0, so <= 1 is == 1.
	case len(args) == 1 && args[0] == "-json": //gooze:ignore comparison
		if err := gopls(stdout, stderr, args...); err != nil {
			_, _ = fmt.Fprintf(stderr, "goozels: gopls version: %v\n", err)

			return 1
		}

		return 0
	default:
		_, _ = fmt.Fprintln(stderr, "usage: goozels version [-json]")

		return 2
	}
}

// version is the goozels module version from the build info: a release tag
// for `go install ...@vX.Y.Z` builds, "(devel)" for local builds.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}

	return "(devel)"
}
