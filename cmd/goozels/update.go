package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"gooze.dev/pkg/goozels/internal/adapter/releasecheck"
	"gooze.dev/pkg/goozels/internal/controller"
	"gooze.dev/pkg/goozels/internal/domain/update"
)

// noUpdateCheckEnv disables the newer-release check when set to any value, as
// the -no-update-check flag does.
const noUpdateCheckEnv = "GOOZELS_NO_UPDATE_CHECK"

// refreshTimeout bounds a background release lookup. Nothing waits for it: the
// command exits whenever it is done, abandoning a lookup still in flight.
const refreshTimeout = 10 * time.Second

// releaseChecker returns the latest-release lookup, or nil when the check is
// skipped: by flag or env, or for builds without a comparable version.
func releaseChecker(skip bool, version string) controller.ReleaseChecker {
	if skip || os.Getenv(noUpdateCheckEnv) != "" || !update.Released(version) {
		return nil
	}

	return releasecheck.New()
}

// cachedUpdateNotice returns the notice known from the cache, never touching
// the network on the caller's path. A stale cache is refreshed in a background
// goroutine for later runs; nothing waits for it, so an offline or slow network
// changes nothing the user sees. A nil checker yields no notice.
func cachedUpdateNotice(checker controller.ReleaseChecker, version string) (string, bool) {
	if checker == nil {
		return "", false
	}

	tag, fresh := checker.Cached()
	if !fresh {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), refreshTimeout)
			defer cancel()

			_, _ = checker.Refresh(ctx)
		}()
	}

	return update.Notice(version, tag)
}

// updateNoticePrinter looks up the cached update notice now (starting any
// background refresh early) and returns a func that prints it to w, or does
// nothing when there is none or the check is skipped.
func updateNoticePrinter(skip bool, w io.Writer) func() {
	notice, ok := cachedUpdateNotice(releaseChecker(skip, version()), version())

	return func() {
		if ok {
			_, _ = fmt.Fprintln(w, notice)
		}
	}
}
