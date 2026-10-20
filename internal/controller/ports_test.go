package controller

import (
	"testing"

	"gooze.dev/pkg/goozels/internal/adapter/docstore"
	"gooze.dev/pkg/goozels/internal/adapter/goozecli"
	"gooze.dev/pkg/goozels/internal/adapter/reportstore"
)

// Compile-time assertions that the production adapters satisfy the controller's
// ports. These fail to build (not just fail at runtime) if a port drifts.
var (
	_ GoozeRunner = goozecli.Runner{}
	_ Documents   = (*docstore.Store)(nil)
	_ Reports     = reportstore.Workspace{}
)

func TestPortsSatisfied(t *testing.T) {
	// The var blocks above are the real assertions; this test documents intent
	// and keeps the file a valid test target.
	if (GoozeRunner)(goozecli.New("gooze")) == nil {
		t.Fatal("goozecli.Runner should satisfy GoozeRunner")
	}
	if (Documents)(docstore.New()) == nil {
		t.Fatal("docstore.Store should satisfy Documents")
	}
	if (Reports)(reportstore.Workspace{}) == nil {
		t.Fatal("reportstore.Workspace should satisfy Reports")
	}
}
