// Command goozels is a Language Server that proxies gopls and layers gooze
// mutation-testing features (gooze:ignore directives and the gooze config file)
// on top. Editors connect to it over stdio exactly as they would to gopls.
package main

import (
	"flag"
	"fmt"
	"os"

	"gooze.dev/pkg/gooze/pkg/config"
	"gooze.dev/pkg/gooze/pkg/report"

	"gooze.dev/pkg/goozels/internal/adapter/docstore"
	"gooze.dev/pkg/goozels/internal/adapter/goozecli"
	"gooze.dev/pkg/goozels/internal/adapter/gopls"
	"gooze.dev/pkg/goozels/internal/controller"
)

func main() {
	goplsPath := flag.String("gopls", envOr("GOOZELS_GOPLS", "gopls"), "path to the gopls binary to proxy")
	goozePath := flag.String("gooze", envOr("GOOZELS_GOOZE", "gooze"), "path to the gooze binary for execute commands")

	flag.Parse()

	proc, err := gopls.Start(*goplsPath, flag.Args()...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "goozels: failed to start gopls (%s): %v\n", *goplsPath, err)
		os.Exit(1)
	}

	srv := controller.NewServer(os.Stdout, proc.Stdin, controller.Deps{
		Gooze:     goozecli.New(*goozePath),
		Documents: docstore.New(),
		Reports:   report.NewDirReader(),
		Config:    config.NewRegistry(),
	})
	_ = srv.Run(os.Stdin, proc.Stdout)

	// Editor disconnected: tear down gopls.
	_ = proc.Kill()
	_ = proc.Wait()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}
