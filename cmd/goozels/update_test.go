package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// stubReleases is a ReleaseChecker whose Refresh never returns, standing in for
// a dead network.
type stubReleases struct {
	tag       string
	fresh     bool
	refreshed chan struct{}
}

func newStubReleases(tag string, fresh bool) *stubReleases {
	return &stubReleases{tag: tag, fresh: fresh, refreshed: make(chan struct{}, 1)}
}

func (s *stubReleases) Cached() (string, bool) { return s.tag, s.fresh }

func (s *stubReleases) Refresh(ctx context.Context) (string, error) {
	s.refreshed <- struct{}{}
	<-ctx.Done()

	return "", ctx.Err()
}

func TestReleaseChecker(t *testing.T) {
	if releaseChecker(false, "v0.2.0") == nil {
		t.Error("releaseChecker for a release build should check by default")
	}

	if releaseChecker(true, "v0.2.0") != nil {
		t.Error("releaseChecker with -no-update-check should skip")
	}

	if releaseChecker(false, "(devel)") != nil {
		t.Error("releaseChecker for a devel build should skip")
	}

	t.Setenv(noUpdateCheckEnv, "1")

	if releaseChecker(false, "v0.2.0") != nil {
		t.Errorf("releaseChecker with %s set should skip", noUpdateCheckEnv)
	}
}

func TestCachedUpdateNotice(t *testing.T) {
	want := "goozels v0.3.0 is available (you have v0.2.0). Update: go install gooze.dev/pkg/goozels/cmd/goozels@latest"

	if got, ok := cachedUpdateNotice(newStubReleases("v0.3.0", true), "v0.2.0"); !ok || got != want {
		t.Errorf("cached newer release = %q, %v; want %q", got, ok, want)
	}

	if _, ok := cachedUpdateNotice(newStubReleases("v0.2.0", true), "v0.2.0"); ok {
		t.Error("up-to-date cache should give no notice")
	}

	if _, ok := cachedUpdateNotice(nil, "v0.2.0"); ok {
		t.Error("nil checker should give no notice")
	}
}

func TestCachedUpdateNotice_DeadNetworkDoesNotBlock(t *testing.T) {
	checker := newStubReleases("", false)

	start := time.Now()
	if _, ok := cachedUpdateNotice(checker, "v0.2.0"); ok {
		t.Error("empty cache should give no notice")
	}

	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("cachedUpdateNotice took %v; it must not wait on the network", elapsed)
	}

	select {
	case <-checker.refreshed:
	case <-time.After(2 * time.Second):
		t.Fatal("stale cache should start a background refresh")
	}
}

func TestUpdateNoticePrinter_SkippedPrintsNothing(t *testing.T) {
	var buf strings.Builder

	updateNoticePrinter(true, &buf)()

	if buf.Len() != 0 {
		t.Errorf("skipped check printed %q", buf.String())
	}
}
