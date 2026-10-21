package controller

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gooze.dev/pkg/goozels/pkg/jsonrpc"
	"gooze.dev/pkg/goozels/pkg/lsp"
)

// stubReleases is a ReleaseChecker whose Refresh blocks until release is
// closed (or forever), standing in for a slow or missing network.
type stubReleases struct {
	cachedTag string
	fresh     bool
	latest    string
	err       error
	release   chan struct{}
	refreshes atomic.Int32
}

func newStubReleases(cachedTag string, fresh bool, latest string, err error) *stubReleases {
	return &stubReleases{cachedTag: cachedTag, fresh: fresh, latest: latest, err: err, release: make(chan struct{})}
}

func (s *stubReleases) Cached() (string, bool) { return s.cachedTag, s.fresh }

func (s *stubReleases) Refresh(ctx context.Context) (string, error) {
	s.refreshes.Add(1)
	select {
	case <-s.release:
		return s.latest, s.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func newUpdateHarness(t *testing.T, releases ReleaseChecker, version string) *harness {
	t.Helper()

	deps := productionDeps("gooze")
	deps.Releases = releases
	deps.Version = version

	return newHarnessWithDeps(t, goplsResponder, deps)
}

func showMessageText(t *testing.T, msg *jsonrpc.Message) lsp.ShowMessageParams {
	t.Helper()

	var p lsp.ShowMessageParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		t.Fatalf("unmarshal showMessage: %v", err)
	}

	return p
}

// expectNoShowMessage round-trips a request, then fails on any showMessage seen
// within a short grace period.
func expectNoShowMessage(t *testing.T, h *harness) {
	t.Helper()

	h.send(t, "initialize", "99", map[string]any{"capabilities": map[string]any{}})
	h.awaitResponse(t, "99")

	deadline := time.After(100 * time.Millisecond)
	for {
		select {
		case msg := <-h.inbox:
			if msg.Method == "window/showMessage" {
				t.Fatalf("unexpected showMessage: %s", msg.Params)
			}
		case <-deadline:
			return
		}
	}
}

func TestInitialized_AnnouncesCachedRelease(t *testing.T) {
	releases := newStubReleases("v0.3.0", true, "", nil)
	h := newUpdateHarness(t, releases, "v0.2.0")

	h.send(t, "initialized", "", map[string]any{})

	p := showMessageText(t, h.awaitNotification(t, "window/showMessage"))
	if p.Type != lsp.MessageWarn || !strings.Contains(p.Message, "goozels v0.3.0 is available") {
		t.Errorf("showMessage = %+v, want a warning about v0.3.0", p)
	}

	// initialized must still reach gopls.
	select {
	case fwd := <-h.goplsSeen:
		if fwd.Method != "initialized" {
			t.Errorf("gopls got %q, want initialized", fwd.Method)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("initialized was not forwarded to gopls")
	}

	if n := releases.refreshes.Load(); n != 0 {
		t.Errorf("refreshes = %d, want 0 with a fresh cache", n)
	}
}

func TestInitialized_DeadNetworkDoesNotBlock(t *testing.T) {
	// Stale cache, and a Refresh that never returns.
	releases := newStubReleases("", false, "v0.3.0", nil)
	h := newUpdateHarness(t, releases, "v0.2.0")

	h.send(t, "initialized", "", map[string]any{})
	<-h.goplsSeen

	// The session keeps serving requests while the lookup hangs.
	expectNoShowMessage(t, h)

	if n := releases.refreshes.Load(); n != 1 {
		t.Errorf("refreshes = %d, want 1", n)
	}
}

func TestInitialized_AnnouncesRefreshedRelease(t *testing.T) {
	releases := newStubReleases("v0.2.0", false, "v0.4.0", nil)
	h := newUpdateHarness(t, releases, "v0.2.0")

	h.send(t, "initialized", "", map[string]any{})
	<-h.goplsSeen
	close(releases.release)

	p := showMessageText(t, h.awaitNotification(t, "window/showMessage"))
	if !strings.Contains(p.Message, "goozels v0.4.0 is available") {
		t.Errorf("showMessage = %+v, want a warning about v0.4.0", p)
	}

	// A second initialized does not repeat the check.
	h.send(t, "initialized", "", map[string]any{})
	<-h.goplsSeen

	if n := releases.refreshes.Load(); n != 1 {
		t.Errorf("refreshes = %d, want 1 per session", n)
	}
}

func TestInitialized_RefreshMatchingCacheIsNotRepeated(t *testing.T) {
	releases := newStubReleases("v0.3.0", false, "v0.3.0", nil)
	h := newUpdateHarness(t, releases, "v0.2.0")
	close(releases.release)

	h.send(t, "initialized", "", map[string]any{})
	h.awaitNotification(t, "window/showMessage")
	<-h.goplsSeen

	expectNoShowMessage(t, h)
}

func TestInitialized_NoAnnouncement(t *testing.T) {
	tests := map[string]struct {
		releases ReleaseChecker
		version  string
	}{
		"up to date":    {newStubReleases("v0.2.0", true, "", nil), "v0.2.0"},
		"offline":       {newStubReleases("", false, "", errors.New("offline")), "v0.2.0"},
		"devel build":   {newStubReleases("v0.3.0", true, "", nil), "(devel)"},
		"check skipped": {nil, "v0.2.0"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if stub, ok := tt.releases.(*stubReleases); ok {
				close(stub.release)
			}

			h := newUpdateHarness(t, tt.releases, tt.version)
			h.send(t, "initialized", "", map[string]any{})
			<-h.goplsSeen

			expectNoShowMessage(t, h)
		})
	}
}
