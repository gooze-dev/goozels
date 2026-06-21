package controller

import (
	"testing"

	"gooze.dev/goozels/internal/adapter/goozecli"
	"gooze.dev/goozels/internal/adapter/lsp"
)

// Compile-time assertions that the production adapters satisfy the controller's
// ports. These fail to build (not just fail at runtime) if a port drifts.
var (
	_ GoozeRunner = goozecli.Runner{}
	_ Documents   = (*lsp.Store)(nil)
)

func TestPortsSatisfied(t *testing.T) {
	// The var blocks above are the real assertions; this test documents intent
	// and keeps the file a valid test target.
	if (GoozeRunner)(goozecli.New("gooze")) == nil {
		t.Fatal("goozecli.Runner should satisfy GoozeRunner")
	}
	if (Documents)(lsp.NewStore()) == nil {
		t.Fatal("lsp.Store should satisfy Documents")
	}
}
