// Command goozels is a Language Server that proxies gopls and layers gooze
// mutation-testing features (gooze:ignore directives and the gooze config file)
// on top. Editors connect to it over stdio exactly as they would to gopls.
package main

import (
	"flag"
	"fmt"
	"os"

	"gooze.dev/goozels/internal/adapter/gopls"
	"gooze.dev/goozels/internal/controller"
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

	srv := controller.NewServer(os.Stdout, proc.Stdin, *goozePath)
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
