// Command goozels is a Language Server that proxies gopls and layers gooze
// mutation-testing features (gooze:ignore directives and the gooze config file)
// on top. Editors connect to it over stdio exactly as they would to gopls.
//
// `goozels check file...` prints the same gooze diagnostics without an LSP
// session, and `goozels hook claude|codex` serves them to coding agents after edits.
package main

import (
	"flag"
	"fmt"
	"os"

	"gooze.dev/pkg/goozels/internal/adapter/docstore"
	"gooze.dev/pkg/goozels/internal/adapter/goozecli"
	"gooze.dev/pkg/goozels/internal/adapter/gopls"
	"gooze.dev/pkg/goozels/internal/adapter/reportstore"
	"gooze.dev/pkg/goozels/internal/controller"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "check":
			os.Exit(runCheck(os.Args[2:], os.Stdout, os.Stderr))
		case "hook":
			os.Exit(runHook(os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
		}
	}

	os.Exit(serve())
}

// serve runs the language server, proxying gopls over stdio, and returns the
// process exit code.
func serve() int {
	goplsPath := flag.String("gopls", envOr("GOOZELS_GOPLS", "gopls"), "path to the gopls binary to proxy")
	goozePath := flag.String("gooze", envOr("GOOZELS_GOOZE", "gooze"), "path to the gooze binary for execute commands")

	flag.Parse()

	proc, err := gopls.Start(*goplsPath, flag.Args()...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "goozels: failed to start gopls (%s): %v\n", *goplsPath, err)

		return 1
	}

	srv := controller.NewServer(os.Stdout, proc.Stdin, controller.Deps{
		Gooze:     goozecli.New(*goozePath),
		Documents: docstore.New(),
		Reports:   reportstore.NewWorkspace(),
	})
	_ = srv.Run(os.Stdin, proc.Stdout)

	// Editor disconnected: tear down gopls.
	_ = proc.Kill()
	_ = proc.Wait()

	return 0
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
